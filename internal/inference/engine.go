package inference

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"ai-proxy/pkg/models"
)

// Engine coordinates schema inference from captured traffic
type Engine struct {
	logger      *zap.Logger
	minSamples  int
	mu          sync.RWMutex
	hostSchemas map[string]*HostSchema
}

// HostSchema holds inferred schemas for a single host
type HostSchema struct {
	Host      string
	Endpoints map[string]*EndpointSchema // key: "METHOD /path/pattern"
	UpdatedAt time.Time
}

// EndpointSchema holds the inferred schema for a single endpoint
type EndpointSchema struct {
	Method      string
	PathPattern string
	Samples     int
	Request     *InferredType
	Response    *InferredType
	StatusCodes map[int]int // count per status code
	UpdatedAt   time.Time
}

// InferredType represents an inferred JSON type
type InferredType struct {
	Type       string                   // "object", "array", "string", "integer", "number", "boolean", "null"
	Format     string                   // "date-time", "email", "uuid", "uri", etc.
	Nullable   bool                     // seen null values
	Enum       []interface{}            // if limited set of values
	Properties map[string]*InferredType // for objects
	Items      *InferredType            // for arrays
	Required   []string                 // required properties
	MinInt     *int64                   // for integer range detection
	MaxInt     *int64                   // for integer range detection
	Examples   []interface{}            // sample values
	SeenCount  int                      // how many times seen
}

// NewEngine creates a new inference engine
func NewEngine(logger *zap.Logger, minSamples int) *Engine {
	if minSamples < 1 {
		minSamples = 3
	}
	return &Engine{
		logger:      logger,
		minSamples:  minSamples,
		hostSchemas: make(map[string]*HostSchema),
	}
}

// LearnFromRequest processes a captured request/response pair
func (e *Engine) LearnFromRequest(ctx context.Context, req *models.HTTPRequest, resp *models.HTTPResponse) error {
	if req == nil {
		return nil
	}

	// Skip non-JSON content for now
	if !isJSONContent(req.ContentType) && (resp == nil || !isJSONContent(resp.ContentType)) {
		return nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Get or create host schema
	hostSchema, ok := e.hostSchemas[req.Host]
	if !ok {
		hostSchema = &HostSchema{
			Host:      req.Host,
			Endpoints: make(map[string]*EndpointSchema),
		}
		e.hostSchemas[req.Host] = hostSchema
	}

	// Detect path pattern
	pathPattern := DetectPathPattern(req.Path)
	key := req.Method + " " + pathPattern

	// Get or create endpoint schema
	endpoint, ok := hostSchema.Endpoints[key]
	if !ok {
		endpoint = &EndpointSchema{
			Method:      req.Method,
			PathPattern: pathPattern,
			StatusCodes: make(map[int]int),
		}
		hostSchema.Endpoints[key] = endpoint
	}

	// Learn from request body
	if len(req.Body) > 0 && isJSONContent(req.ContentType) {
		var data interface{}
		if err := json.Unmarshal(req.Body, &data); err == nil {
			if endpoint.Request == nil {
				endpoint.Request = &InferredType{Properties: make(map[string]*InferredType)}
			}
			e.inferType(endpoint.Request, data)
		}
	}

	// Learn from response body
	if resp != nil {
		endpoint.StatusCodes[resp.StatusCode]++

		if len(resp.Body) > 0 && isJSONContent(resp.ContentType) {
			var data interface{}
			if err := json.Unmarshal(resp.Body, &data); err == nil {
				if endpoint.Response == nil {
					endpoint.Response = &InferredType{Properties: make(map[string]*InferredType)}
				}
				e.inferType(endpoint.Response, data)
			}
		}
	}

	endpoint.Samples++
	endpoint.UpdatedAt = time.Now()
	hostSchema.UpdatedAt = time.Now()

	return nil
}

// inferType recursively infers and merges type information
func (e *Engine) inferType(schema *InferredType, value interface{}) {
	schema.SeenCount++

	switch v := value.(type) {
	case nil:
		schema.Nullable = true
		if schema.Type == "" {
			schema.Type = "null"
		}

	case bool:
		schema.Type = "boolean"
		e.addExample(schema, v)

	case float64:
		// JSON numbers are always float64
		if v == float64(int64(v)) {
			// It's an integer
			if schema.Type == "" || schema.Type == "integer" {
				schema.Type = "integer"
				intVal := int64(v)
				if schema.MinInt == nil || intVal < *schema.MinInt {
					schema.MinInt = &intVal
				}
				if schema.MaxInt == nil || intVal > *schema.MaxInt {
					schema.MaxInt = &intVal
				}
			} else if schema.Type == "number" {
				// Keep as number if we've seen non-integers
			}
		} else {
			schema.Type = "number"
		}
		e.addExample(schema, v)

	case string:
		schema.Type = "string"
		// Detect format
		if format := detectStringFormat(v); format != "" {
			if schema.Format == "" || schema.Format == format {
				schema.Format = format
			}
		}
		e.addExample(schema, v)
		e.trackEnum(schema, v)

	case []interface{}:
		schema.Type = "array"
		if schema.Items == nil {
			schema.Items = &InferredType{Properties: make(map[string]*InferredType)}
		}
		for _, item := range v {
			e.inferType(schema.Items, item)
		}

	case map[string]interface{}:
		schema.Type = "object"
		if schema.Properties == nil {
			schema.Properties = make(map[string]*InferredType)
		}
		for key, val := range v {
			prop, ok := schema.Properties[key]
			if !ok {
				prop = &InferredType{Properties: make(map[string]*InferredType)}
				schema.Properties[key] = prop
			}
			e.inferType(prop, val)
		}
		// Track which properties are required (seen every time)
		e.updateRequired(schema, v)
	}
}

// addExample adds an example value (limited to 5)
func (e *Engine) addExample(schema *InferredType, value interface{}) {
	if len(schema.Examples) < 5 {
		// Check for duplicates
		for _, ex := range schema.Examples {
			if ex == value {
				return
			}
		}
		schema.Examples = append(schema.Examples, value)
	}
}

// trackEnum tracks potential enum values
func (e *Engine) trackEnum(schema *InferredType, value string) {
	if schema.Enum == nil {
		schema.Enum = []interface{}{}
	}

	// Check if already tracked
	for _, v := range schema.Enum {
		if v == value {
			return
		}
	}

	// Only track if we have few unique values
	if len(schema.Enum) < 20 {
		schema.Enum = append(schema.Enum, value)
	} else {
		// Too many values - not an enum
		schema.Enum = nil
	}
}

// updateRequired updates the required field list based on seen properties
func (e *Engine) updateRequired(schema *InferredType, obj map[string]interface{}) {
	if schema.SeenCount == 1 {
		// First time - all properties are potentially required
		schema.Required = make([]string, 0, len(obj))
		for key := range obj {
			schema.Required = append(schema.Required, key)
		}
		sort.Strings(schema.Required)
	} else {
		// Remove properties from required if not present
		newRequired := make([]string, 0, len(schema.Required))
		for _, key := range schema.Required {
			if _, ok := obj[key]; ok {
				newRequired = append(newRequired, key)
			}
		}
		schema.Required = newRequired
	}
}

// GetHostSchema returns the inferred schema for a host
func (e *Engine) GetHostSchema(host string) *HostSchema {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.hostSchemas[host]
}

// GetEndpointSchema returns the inferred schema for an endpoint
func (e *Engine) GetEndpointSchema(host, method, pathPattern string) *EndpointSchema {
	e.mu.RLock()
	defer e.mu.RUnlock()

	hostSchema, ok := e.hostSchemas[host]
	if !ok {
		return nil
	}

	key := method + " " + pathPattern
	return hostSchema.Endpoints[key]
}

// ListHosts returns all hosts with inferred schemas
func (e *Engine) ListHosts() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	hosts := make([]string, 0, len(e.hostSchemas))
	for host := range e.hostSchemas {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	return hosts
}

// isJSONContent checks if content type is JSON
func isJSONContent(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "application/json") ||
		strings.Contains(ct, "+json")
}

// detectStringFormat detects common string formats
func detectStringFormat(s string) string {
	// UUID detection (v1, v4, v5, v7)
	if uuidRegex.MatchString(s) {
		return "uuid"
	}

	// Date-time (ISO 8601)
	if dateTimeRegex.MatchString(s) {
		return "date-time"
	}

	// Date only
	if dateRegex.MatchString(s) {
		return "date"
	}

	// Email
	if emailRegex.MatchString(s) {
		return "email"
	}

	// URI/URL
	if uriRegex.MatchString(s) {
		return "uri"
	}

	// IPv4
	if ipv4Regex.MatchString(s) {
		return "ipv4"
	}

	// IPv6
	if ipv6Regex.MatchString(s) {
		return "ipv6"
	}

	return ""
}

// Common format regexes
var (
	uuidRegex     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	dateTimeRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)
	dateRegex     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	emailRegex    = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	uriRegex      = regexp.MustCompile(`^https?://`)
	ipv4Regex     = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`)
	ipv6Regex     = regexp.MustCompile(`^([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}$`)
)

// InferIntegerType determines int32 vs int64 based on observed range
func (e *Engine) InferIntegerType(schema *InferredType) string {
	if schema.MinInt == nil || schema.MaxInt == nil {
		return "int64" // Default to safe option
	}

	min, max := *schema.MinInt, *schema.MaxInt

	// Check if fits in int32
	const int32Min = -2147483648
	const int32Max = 2147483647

	if min >= int32Min && max <= int32Max {
		return "int32"
	}
	return "int64"
}

// IsEnumField determines if a string field should be an enum
func (e *Engine) IsEnumField(schema *InferredType) bool {
	if schema.Type != "string" || schema.Enum == nil {
		return false
	}

	// Must have seen enough samples
	if schema.SeenCount < e.minSamples {
		return false
	}

	// Max 10 unique values for enum
	if len(schema.Enum) > 10 {
		return false
	}

	// Values should repeat (seen count should be higher than enum count)
	return schema.SeenCount > len(schema.Enum)
}
