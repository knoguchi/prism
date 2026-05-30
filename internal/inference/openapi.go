package inference

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// OpenAPITag represents a tag for grouping operations
type OpenAPITag struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// OpenAPI represents an OpenAPI 3.0 specification
type OpenAPI struct {
	OpenAPI    string                 `yaml:"openapi" json:"openapi"`
	Info       OpenAPIInfo            `yaml:"info" json:"info"`
	Servers    []OpenAPIServer        `yaml:"servers,omitempty" json:"servers,omitempty"`
	Tags       []OpenAPITag           `yaml:"tags,omitempty" json:"tags,omitempty"`
	Paths      map[string]PathItem    `yaml:"paths" json:"paths"`
	Components *OpenAPIComponents     `yaml:"components,omitempty" json:"components,omitempty"`
}

type OpenAPIInfo struct {
	Title       string          `yaml:"title" json:"title"`
	Description string          `yaml:"description" json:"description"`
	Version     string          `yaml:"version" json:"version"`
	Contact     *OpenAPIContact `yaml:"contact,omitempty" json:"contact,omitempty"`
	License     *OpenAPILicense `yaml:"license,omitempty" json:"license,omitempty"`
}

type OpenAPIContact struct {
	Name  string `yaml:"name,omitempty" json:"name,omitempty"`
	URL   string `yaml:"url,omitempty" json:"url,omitempty"`
	Email string `yaml:"email,omitempty" json:"email,omitempty"`
}

type OpenAPILicense struct {
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url,omitempty" json:"url,omitempty"`
}

type OpenAPIServer struct {
	URL         string `yaml:"url" json:"url"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

type PathItem struct {
	Get     *Operation `yaml:"get,omitempty" json:"get,omitempty"`
	Post    *Operation `yaml:"post,omitempty" json:"post,omitempty"`
	Put     *Operation `yaml:"put,omitempty" json:"put,omitempty"`
	Patch   *Operation `yaml:"patch,omitempty" json:"patch,omitempty"`
	Delete  *Operation `yaml:"delete,omitempty" json:"delete,omitempty"`
	Head    *Operation `yaml:"head,omitempty" json:"head,omitempty"`
	Options *Operation `yaml:"options,omitempty" json:"options,omitempty"`
}

type Operation struct {
	Summary     string               `yaml:"summary" json:"summary"`
	Description string               `yaml:"description" json:"description"`
	OperationID string               `yaml:"operationId" json:"operationId"`
	Tags        []string             `yaml:"tags,omitempty" json:"tags,omitempty"`
	Parameters  []Parameter          `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	RequestBody *RequestBody         `yaml:"requestBody,omitempty" json:"requestBody,omitempty"`
	Responses   map[string]*Response `yaml:"responses" json:"responses"`
}

type Parameter struct {
	Name        string  `yaml:"name" json:"name"`
	In          string  `yaml:"in" json:"in"` // path, query, header, cookie
	Description string  `yaml:"description" json:"description"`
	Required    bool    `yaml:"required" json:"required"`
	Schema      *Schema `yaml:"schema,omitempty" json:"schema,omitempty"`
}

type RequestBody struct {
	Description string               `yaml:"description,omitempty" json:"description,omitempty"`
	Required    bool                 `yaml:"required,omitempty" json:"required,omitempty"`
	Content     map[string]MediaType `yaml:"content" json:"content"`
}

type Response struct {
	Description string               `yaml:"description" json:"description"`
	Content     map[string]MediaType `yaml:"content,omitempty" json:"content,omitempty"`
}

type MediaType struct {
	Schema  *Schema     `yaml:"schema,omitempty" json:"schema,omitempty"`
	Example interface{} `yaml:"example,omitempty" json:"example,omitempty"`
}

type Schema struct {
	Type        string             `yaml:"type,omitempty" json:"type,omitempty"`
	Format      string             `yaml:"format,omitempty" json:"format,omitempty"`
	Description string             `yaml:"description,omitempty" json:"description,omitempty"`
	Properties  map[string]*Schema `yaml:"properties,omitempty" json:"properties,omitempty"`
	Items       *Schema            `yaml:"items,omitempty" json:"items,omitempty"`
	Required    []string           `yaml:"required,omitempty" json:"required,omitempty"`
	Enum        []interface{}      `yaml:"enum,omitempty" json:"enum,omitempty"`
	Example     interface{}        `yaml:"example,omitempty" json:"example,omitempty"`
	Nullable    bool               `yaml:"nullable,omitempty" json:"nullable,omitempty"`
	Minimum     *int64             `yaml:"minimum,omitempty" json:"minimum,omitempty"`
	Maximum     *int64             `yaml:"maximum,omitempty" json:"maximum,omitempty"`
}

type OpenAPIComponents struct {
	Schemas map[string]*Schema `yaml:"schemas,omitempty" json:"schemas,omitempty"`
}

// OpenAPIGenerator generates OpenAPI 3.0 specs
type OpenAPIGenerator struct {
	engine *Engine
}

// NewOpenAPIGenerator creates a new OpenAPI generator
func NewOpenAPIGenerator(engine *Engine) *OpenAPIGenerator {
	return &OpenAPIGenerator{engine: engine}
}

// Generate creates an OpenAPI spec for a host
func (g *OpenAPIGenerator) Generate(host string) (*OpenAPI, error) {
	hostSchema := g.engine.GetHostSchema(host)
	if hostSchema == nil {
		return nil, fmt.Errorf("no schema found for host: %s", host)
	}

	spec := &OpenAPI{
		OpenAPI: "3.0.3",
		Info: OpenAPIInfo{
			Title:       fmt.Sprintf("%s API", host),
			Description: fmt.Sprintf("API specification for %s, auto-generated from captured traffic by Prism.", host),
			Version:     "1.0.0",
			Contact: &OpenAPIContact{
				Name: "Prism",
			},
			License: &OpenAPILicense{
				Name: "MIT",
			},
		},
		Servers: []OpenAPIServer{
			{URL: fmt.Sprintf("https://%s", host), Description: "Production server"},
		},
		Paths: make(map[string]PathItem),
	}

	// Collect unique tags
	tagSet := make(map[string]bool)

	// Group endpoints by path
	pathGroups := make(map[string]map[string]*EndpointSchema)
	for _, endpoint := range hostSchema.Endpoints {
		if pathGroups[endpoint.PathPattern] == nil {
			pathGroups[endpoint.PathPattern] = make(map[string]*EndpointSchema)
		}
		pathGroups[endpoint.PathPattern][endpoint.Method] = endpoint

		// Collect tags
		tags := g.extractTags(endpoint.PathPattern)
		for _, tag := range tags {
			tagSet[tag] = true
		}
	}

	// Build tags array
	for tag := range tagSet {
		spec.Tags = append(spec.Tags, OpenAPITag{
			Name:        tag,
			Description: fmt.Sprintf("Operations related to %s", tag),
		})
	}
	// Sort tags for consistent output
	sort.Slice(spec.Tags, func(i, j int) bool {
		return spec.Tags[i].Name < spec.Tags[j].Name
	})

	// Convert to OpenAPI paths
	for pathPattern, methods := range pathGroups {
		pathItem := PathItem{}

		for method, endpoint := range methods {
			op := g.generateOperation(pathPattern, method, endpoint)

			switch strings.ToUpper(method) {
			case "GET":
				pathItem.Get = op
			case "POST":
				pathItem.Post = op
			case "PUT":
				pathItem.Put = op
			case "PATCH":
				pathItem.Patch = op
			case "DELETE":
				pathItem.Delete = op
			case "HEAD":
				pathItem.Head = op
			case "OPTIONS":
				pathItem.Options = op
			}
		}

		spec.Paths[pathPattern] = pathItem
	}

	return spec, nil
}

// GenerateYAML returns the OpenAPI spec as YAML
func (g *OpenAPIGenerator) GenerateYAML(host string) (string, error) {
	spec, err := g.Generate(host)
	if err != nil {
		return "", err
	}

	data, err := yaml.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("failed to marshal OpenAPI spec: %w", err)
	}

	return string(data), nil
}

// generateOperation creates an OpenAPI operation from an endpoint schema
func (g *OpenAPIGenerator) generateOperation(pathPattern, method string, endpoint *EndpointSchema) *Operation {
	summary := fmt.Sprintf("%s %s", method, pathPattern)
	op := &Operation{
		Summary:     summary,
		Description: fmt.Sprintf("Performs %s operation on %s. Inferred from %d captured request(s).", method, pathPattern, endpoint.Samples),
		OperationID: g.generateOperationID(method, pathPattern),
		Tags:        g.extractTags(pathPattern),
		Parameters:  g.generateParameters(pathPattern),
		Responses:   make(map[string]*Response),
	}

	// Add request body for methods that typically have one
	if endpoint.Request != nil && (method == "POST" || method == "PUT" || method == "PATCH") {
		op.RequestBody = &RequestBody{
			Required: true,
			Content: map[string]MediaType{
				"application/json": {
					Schema: g.convertToSchema(endpoint.Request),
				},
			},
		}
	}

	// Add responses
	if endpoint.Response != nil {
		for statusCode := range endpoint.StatusCodes {
			statusStr := fmt.Sprintf("%d", statusCode)
			resp := &Response{
				Description: httpStatusText(statusCode),
			}

			// Add response body for success codes
			if statusCode >= 200 && statusCode < 300 {
				resp.Content = map[string]MediaType{
					"application/json": {
						Schema: g.convertToSchema(endpoint.Response),
					},
				}
			}

			op.Responses[statusStr] = resp
		}
	}

	// Ensure at least a default response
	if len(op.Responses) == 0 {
		op.Responses["200"] = &Response{
			Description: "Successful response",
		}
	}

	return op
}

// generateOperationID creates a unique operation ID
func (g *OpenAPIGenerator) generateOperationID(method, pathPattern string) string {
	// Convert path to camelCase identifier
	parts := strings.Split(strings.Trim(pathPattern, "/"), "/")
	var result strings.Builder

	result.WriteString(strings.ToLower(method))

	for _, part := range parts {
		if strings.HasPrefix(part, "{") {
			// Parameter - use "By" + param name
			paramName := strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
			result.WriteString("By")
			result.WriteString(capitalize(paramName))
		} else {
			result.WriteString(capitalize(part))
		}
	}

	return result.String()
}

// extractTags extracts tags from path (usually first segment)
func (g *OpenAPIGenerator) extractTags(pathPattern string) []string {
	parts := strings.Split(strings.Trim(pathPattern, "/"), "/")
	if len(parts) > 0 && !strings.HasPrefix(parts[0], "{") {
		// Skip version prefixes
		tag := parts[0]
		if versionRegex.MatchString(tag) && len(parts) > 1 {
			tag = parts[1]
		}
		return []string{tag}
	}
	return nil
}

// generateParameters extracts path parameters from pattern
func (g *OpenAPIGenerator) generateParameters(pathPattern string) []Parameter {
	var params []Parameter

	parts := strings.Split(strings.Trim(pathPattern, "/"), "/")
	for _, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			paramName := part[1 : len(part)-1]
			schemaType, format := GetPathParamType(paramName)

			param := Parameter{
				Name:        paramName,
				In:          "path",
				Description: fmt.Sprintf("The %s parameter", paramName),
				Required:    true,
				Schema: &Schema{
					Type:   schemaType,
					Format: format,
				},
			}
			params = append(params, param)
		}
	}

	return params
}

// convertToSchema converts InferredType to OpenAPI Schema
func (g *OpenAPIGenerator) convertToSchema(inferred *InferredType) *Schema {
	if inferred == nil {
		return nil
	}

	schema := &Schema{
		Type:     inferred.Type,
		Format:   inferred.Format,
		Nullable: inferred.Nullable,
	}

	// Handle integer range for format
	if inferred.Type == "integer" && inferred.MinInt != nil && inferred.MaxInt != nil {
		intType := g.engine.InferIntegerType(inferred)
		schema.Format = intType
	}

	// Handle enums
	if g.engine.IsEnumField(inferred) {
		schema.Enum = inferred.Enum
	}

	// Handle properties for objects
	if inferred.Type == "object" && len(inferred.Properties) > 0 {
		schema.Properties = make(map[string]*Schema)
		for name, prop := range inferred.Properties {
			schema.Properties[name] = g.convertToSchema(prop)
		}
		if len(inferred.Required) > 0 {
			schema.Required = inferred.Required
		}
	}

	// Handle array items
	if inferred.Type == "array" && inferred.Items != nil {
		schema.Items = g.convertToSchema(inferred.Items)
	}

	// Add example if available
	if len(inferred.Examples) > 0 {
		schema.Example = inferred.Examples[0]
	}

	return schema
}

// capitalize capitalizes first letter
func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// httpStatusText returns description for HTTP status codes
func httpStatusText(code int) string {
	texts := map[int]string{
		200: "OK",
		201: "Created",
		202: "Accepted",
		204: "No Content",
		301: "Moved Permanently",
		302: "Found",
		304: "Not Modified",
		400: "Bad Request",
		401: "Unauthorized",
		403: "Forbidden",
		404: "Not Found",
		405: "Method Not Allowed",
		409: "Conflict",
		422: "Unprocessable Entity",
		429: "Too Many Requests",
		500: "Internal Server Error",
		502: "Bad Gateway",
		503: "Service Unavailable",
	}

	if text, ok := texts[code]; ok {
		return text
	}
	return "Response"
}

// GenerateForEndpoint generates OpenAPI for a specific endpoint
func (g *OpenAPIGenerator) GenerateForEndpoint(host, method, pathPattern string) (*PathItem, error) {
	endpoint := g.engine.GetEndpointSchema(host, method, pathPattern)
	if endpoint == nil {
		return nil, fmt.Errorf("endpoint not found: %s %s", method, pathPattern)
	}

	pathItem := &PathItem{}
	op := g.generateOperation(pathPattern, method, endpoint)

	switch strings.ToUpper(method) {
	case "GET":
		pathItem.Get = op
	case "POST":
		pathItem.Post = op
	case "PUT":
		pathItem.Put = op
	case "PATCH":
		pathItem.Patch = op
	case "DELETE":
		pathItem.Delete = op
	}

	return pathItem, nil
}

// ListEndpoints returns all endpoints for a host
func (g *OpenAPIGenerator) ListEndpoints(host string) []EndpointSummary {
	hostSchema := g.engine.GetHostSchema(host)
	if hostSchema == nil {
		return nil
	}

	var endpoints []EndpointSummary
	for _, ep := range hostSchema.Endpoints {
		endpoints = append(endpoints, EndpointSummary{
			Method:      ep.Method,
			PathPattern: ep.PathPattern,
			Samples:     ep.Samples,
		})
	}

	// Sort by path then method
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].PathPattern == endpoints[j].PathPattern {
			return endpoints[i].Method < endpoints[j].Method
		}
		return endpoints[i].PathPattern < endpoints[j].PathPattern
	})

	return endpoints
}

// EndpointSummary provides a brief overview of an endpoint
type EndpointSummary struct {
	Method      string `json:"method"`
	PathPattern string `json:"path_pattern"`
	Samples     int    `json:"samples"`
}
