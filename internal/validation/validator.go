package validation

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"go.uber.org/zap"

	"ai-proxy/internal/storage"
	"ai-proxy/pkg/models"
)

// ValidationResult represents the result of validating a request
type ValidationResult struct {
	RequestID   string             `json:"request_id"`
	Host        string             `json:"host"`
	Method      string             `json:"method"`
	Path        string             `json:"path"`
	Valid       bool               `json:"valid"`
	Errors      []ValidationError  `json:"errors,omitempty"`
	MatchedPath string             `json:"matched_path,omitempty"`
}

// ValidationError represents a single validation error
type ValidationError struct {
	Location string `json:"location"` // "path", "query", "header", "body"
	Field    string `json:"field"`
	Message  string `json:"message"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
}

// Validator validates requests against OpenAPI specs
type Validator struct {
	db      *storage.DB
	logger  *zap.Logger
	mu      sync.RWMutex
	routers map[string]routers.Router // host -> router
	specs   map[string]*openapi3.T    // host -> spec
}

// NewValidator creates a new validator
func NewValidator(db *storage.DB, logger *zap.Logger) *Validator {
	return &Validator{
		db:      db,
		logger:  logger,
		routers: make(map[string]routers.Router),
		specs:   make(map[string]*openapi3.T),
	}
}

// LoadSpec loads the OpenAPI spec for a host from the database
func (v *Validator) LoadSpec(ctx context.Context, host string) error {
	schemas, err := v.db.GetSchemasByFormat(ctx, host, "openapi")
	if err != nil {
		return fmt.Errorf("failed to get OpenAPI schema: %w", err)
	}
	if len(schemas) == 0 {
		return fmt.Errorf("no OpenAPI schema found for host %s", host)
	}

	// Parse the OpenAPI spec
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromData([]byte(schemas[0].Content))
	if err != nil {
		return fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}

	// Fix missing descriptions in responses (required by OpenAPI 3.0)
	fixMissingDescriptions(spec)

	// Validate the spec itself
	if err := spec.Validate(ctx); err != nil {
		v.logger.Warn("OpenAPI spec has validation issues", zap.String("host", host), zap.Error(err))
		// Continue anyway - the spec may still be usable
	}

	// Create router from spec using legacy router (doesn't depend on gorilla/mux)
	router, err := legacy.NewRouter(spec)
	if err != nil {
		return fmt.Errorf("failed to create router from spec: %w", err)
	}

	v.mu.Lock()
	v.routers[host] = router
	v.specs[host] = spec
	v.mu.Unlock()

	v.logger.Info("Loaded OpenAPI spec for validation", zap.String("host", host))
	return nil
}

// fixMissingDescriptions adds default descriptions to responses and operations
// that are missing them (required by OpenAPI 3.0)
func fixMissingDescriptions(spec *openapi3.T) {
	if spec.Paths == nil {
		return
	}

	for _, pathItem := range spec.Paths.Map() {
		for _, op := range []*openapi3.Operation{
			pathItem.Get, pathItem.Post, pathItem.Put, pathItem.Delete,
			pathItem.Patch, pathItem.Head, pathItem.Options,
		} {
			if op == nil {
				continue
			}

			// Fix missing operation description
			if op.Description == "" {
				if op.Summary != "" {
					op.Description = op.Summary
				} else {
					op.Description = "No description available"
				}
			}

			// Fix missing response descriptions
			if op.Responses != nil {
				for code, resp := range op.Responses.Map() {
					if resp.Value != nil && resp.Value.Description == nil {
						desc := getDefaultResponseDescription(code)
						resp.Value.Description = &desc
					}
				}
			}
		}
	}
}

// getDefaultResponseDescription returns a default description for HTTP status codes
func getDefaultResponseDescription(code string) string {
	switch code {
	case "200":
		return "Successful response"
	case "201":
		return "Resource created successfully"
	case "204":
		return "No content"
	case "400":
		return "Bad request"
	case "401":
		return "Unauthorized"
	case "403":
		return "Forbidden"
	case "404":
		return "Not found"
	case "500":
		return "Internal server error"
	default:
		return "Response"
	}
}

// HasSpec checks if a spec is loaded for a host
func (v *Validator) HasSpec(host string) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	_, ok := v.specs[host]
	return ok
}

// ValidateRequest validates a captured request against its OpenAPI spec
func (v *Validator) ValidateRequest(ctx context.Context, capture *models.Capture) (*ValidationResult, error) {
	host := capture.Request.Host
	requestID := capture.Request.UUID

	// Skip OPTIONS requests (CORS preflight) - they're not part of the API contract
	if capture.Request.Method == "OPTIONS" {
		return &ValidationResult{
			RequestID: requestID,
			Host:      host,
			Method:    capture.Request.Method,
			Path:      capture.Request.Path,
			Valid:     true, // Mark as valid since we're skipping
		}, nil
	}

	// Try to load spec if not already loaded
	if !v.HasSpec(host) {
		if err := v.LoadSpec(ctx, host); err != nil {
			return &ValidationResult{
				RequestID: requestID,
				Host:      host,
				Method:    capture.Request.Method,
				Path:      capture.Request.Path,
				Valid:     false,
				Errors: []ValidationError{{
					Location: "spec",
					Message:  fmt.Sprintf("No OpenAPI spec available: %v", err),
				}},
			}, nil
		}
	}

	v.mu.RLock()
	router := v.routers[host]
	v.mu.RUnlock()

	if router == nil {
		return &ValidationResult{
			RequestID: requestID,
			Host:      host,
			Method:    capture.Request.Method,
			Path:      capture.Request.Path,
			Valid:     false,
			Errors: []ValidationError{{
				Location: "spec",
				Message:  "Router not initialized",
			}},
		}, nil
	}

	result := &ValidationResult{
		RequestID: requestID,
		Host:      host,
		Method:    capture.Request.Method,
		Path:      capture.Request.Path,
		Valid:     true,
	}

	// Build HTTP request from capture
	reqURL := capture.Request.URL
	if reqURL == "" {
		scheme := "https"
		if !capture.Request.IsHTTPS {
			scheme = "http"
		}
		reqURL = fmt.Sprintf("%s://%s%s", scheme, host, capture.Request.Path)
		if capture.Request.QueryString != "" {
			reqURL += "?" + capture.Request.QueryString
		}
	}

	parsedURL, err := url.Parse(reqURL)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Location: "url",
			Message:  fmt.Sprintf("Invalid URL: %v", err),
		})
		return result, nil
	}

	// Create HTTP request for validation
	httpReq := &http.Request{
		Method: capture.Request.Method,
		URL:    parsedURL,
		Header: make(http.Header),
	}

	// Copy headers (map[string]string -> http.Header)
	for key, val := range capture.Request.Headers {
		httpReq.Header.Set(key, val)
	}

	// Set body if present
	if len(capture.Request.Body) > 0 {
		httpReq.Body = http.NoBody // We'll validate body separately
	}

	// Find route
	route, pathParams, err := router.FindRoute(httpReq)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Location: "path",
			Message:  fmt.Sprintf("No matching route found: %v", err),
		})
		return result, nil
	}

	result.MatchedPath = route.Path

	// Validate request
	requestInput := &openapi3filter.RequestValidationInput{
		Request:    httpReq,
		PathParams: pathParams,
		Route:      route,
		Options: &openapi3filter.Options{
			IncludeResponseStatus: false,
			MultiError:            true,
		},
	}

	// Set body for validation if JSON
	if len(capture.Request.Body) > 0 && strings.Contains(capture.Request.ContentType, "json") {
		httpReq.Body = newReadCloser(capture.Request.Body)
		httpReq.ContentLength = int64(len(capture.Request.Body))
	}

	if err := openapi3filter.ValidateRequest(ctx, requestInput); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, parseValidationErrors(err)...)
	}

	// Validate response if present
	if capture.Response != nil && capture.Response.StatusCode > 0 {
		responseInput := &openapi3filter.ResponseValidationInput{
			RequestValidationInput: requestInput,
			Status:                 capture.Response.StatusCode,
			Header:                 make(http.Header),
			Options: &openapi3filter.Options{
				IncludeResponseStatus: true,
				MultiError:            true,
			},
		}

		// Copy response headers (map[string]string -> http.Header)
		for key, val := range capture.Response.Headers {
			responseInput.Header.Set(key, val)
		}

		// Set response body if JSON
		if len(capture.Response.Body) > 0 && strings.Contains(capture.Response.ContentType, "json") {
			responseInput.SetBodyBytes(capture.Response.Body)
		}

		if err := openapi3filter.ValidateResponse(ctx, responseInput); err != nil {
			result.Valid = false
			for _, e := range parseValidationErrors(err) {
				e.Location = "response." + e.Location
				result.Errors = append(result.Errors, e)
			}
		}
	}

	return result, nil
}

// ValidateHost validates all captured requests for a host
func (v *Validator) ValidateHost(ctx context.Context, host string, limit int) ([]*ValidationResult, error) {
	// Ensure spec is loaded
	if !v.HasSpec(host) {
		if err := v.LoadSpec(ctx, host); err != nil {
			return nil, err
		}
	}

	// Get captures for this host
	filter := &storage.RequestFilter{
		Host:  host,
		Limit: limit,
	}

	requests, _, err := v.db.ListRequests(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list requests: %w", err)
	}

	var results []*ValidationResult
	for _, req := range requests {
		capture, err := v.db.GetCapture(ctx, req.ID)
		if err != nil || capture == nil {
			continue
		}

		result, err := v.ValidateRequest(ctx, capture)
		if err != nil {
			v.logger.Error("Validation failed", zap.String("request_id", req.ID), zap.Error(err))
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

// GetValidationSummary returns a summary of validation results for a host
func (v *Validator) GetValidationSummary(ctx context.Context, host string) (*ValidationSummary, error) {
	results, err := v.ValidateHost(ctx, host, 500)
	if err != nil {
		return nil, err
	}

	summary := &ValidationSummary{
		Host:          host,
		TotalRequests: len(results),
		ValidCount:    0,
		InvalidCount:  0,
		ErrorsByPath:  make(map[string]int),
		ErrorsByType:  make(map[string]int),
	}

	for _, r := range results {
		if r.Valid {
			summary.ValidCount++
		} else {
			summary.InvalidCount++
			if r.MatchedPath != "" {
				summary.ErrorsByPath[r.MatchedPath]++
			}
			for _, e := range r.Errors {
				summary.ErrorsByType[e.Location]++
			}
		}
	}

	return summary, nil
}

// ValidationSummary provides aggregate validation statistics
type ValidationSummary struct {
	Host          string         `json:"host"`
	TotalRequests int            `json:"total_requests"`
	ValidCount    int            `json:"valid_count"`
	InvalidCount  int            `json:"invalid_count"`
	ErrorsByPath  map[string]int `json:"errors_by_path"`
	ErrorsByType  map[string]int `json:"errors_by_type"`
}

// parseValidationErrors converts kin-openapi errors to ValidationErrors
func parseValidationErrors(err error) []ValidationError {
	var errors []ValidationError

	if multiErr, ok := err.(openapi3.MultiError); ok {
		for _, e := range multiErr {
			errors = append(errors, parseValidationError(e))
		}
	} else {
		errors = append(errors, parseValidationError(err))
	}

	return errors
}

func parseValidationError(err error) ValidationError {
	ve := ValidationError{
		Message: err.Error(),
	}

	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "header"):
		ve.Location = "header"
	case strings.Contains(errStr, "query"):
		ve.Location = "query"
	case strings.Contains(errStr, "path"):
		ve.Location = "path"
	case strings.Contains(errStr, "body"):
		ve.Location = "body"
	default:
		ve.Location = "unknown"
	}

	return ve
}

// readCloser wraps a byte slice as io.ReadCloser
type readCloser struct {
	data   []byte
	offset int
}

func newReadCloser(data []byte) *readCloser {
	return &readCloser{data: data}
}

func (r *readCloser) Read(p []byte) (n int, err error) {
	if r.offset >= len(r.data) {
		return 0, fmt.Errorf("EOF")
	}
	n = copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func (r *readCloser) Close() error {
	return nil
}