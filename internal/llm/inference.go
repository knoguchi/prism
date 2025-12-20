package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ai-proxy/pkg/models"
)

// InferenceService handles AI-powered traffic analysis
type InferenceService struct {
	provider Provider
}

// NewInferenceService creates a new inference service
func NewInferenceService(provider Provider) *InferenceService {
	return &InferenceService{provider: provider}
}

// EndpointSample represents a sample for inference
type EndpointSample struct {
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	QueryString string            `json:"query_string,omitempty"`
	StatusCode  int               `json:"status_code"`
	RequestBody string            `json:"request_body,omitempty"`
	ResponseBody string           `json:"response_body,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// InferredPattern represents an inferred URL pattern
type InferredPattern struct {
	Pattern     string            `json:"pattern"`      // e.g., "/users/{id}"
	Method      string            `json:"method"`
	Description string            `json:"description"`
	PathParams  map[string]string `json:"path_params"`  // param name -> type
	QueryParams map[string]string `json:"query_params"` // param name -> type/description
}

// rawInferredPattern is used for flexible JSON parsing where method can be string or array
type rawInferredPattern struct {
	Pattern     string            `json:"pattern"`
	Method      json.RawMessage   `json:"method"` // can be string or []string
	Description string            `json:"description"`
	PathParams  map[string]string `json:"path_params"`
	QueryParams map[string]string `json:"query_params"`
}

// parseMethod extracts method(s) from raw JSON - handles both string and array
func parseMethod(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}

	// Try as string first
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}
	}

	// Try as array
	var multiple []string
	if err := json.Unmarshal(raw, &multiple); err == nil {
		return multiple
	}

	return nil
}

// InferredSchema represents an inferred data schema
type InferredSchema struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Fields      map[string]FieldSchema `json:"fields"`
}

// FieldSchema represents a field in a schema
type FieldSchema struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Example     any    `json:"example,omitempty"`
}

// InferPatterns analyzes URL paths and infers patterns
func (s *InferenceService) InferPatterns(ctx context.Context, samples []EndpointSample) ([]InferredPattern, error) {
	if len(samples) == 0 {
		return nil, nil
	}

	// Extract unique method+path combinations (no bodies - keep it lightweight)
	seen := make(map[string]bool)
	var pathList []string
	for _, sample := range samples {
		key := sample.Method + " " + sample.Path
		if !seen[key] {
			seen[key] = true
			pathList = append(pathList, key)
		}
	}

	// Send just the path list - much more efficient
	pathsText := strings.Join(pathList, "\n")

	systemPrompt := `You are an API route analyzer. Given a list of HTTP method + path combinations, identify URL patterns with path parameters.

Your goal is to BUILD A ROUTER that can match all these requests.

Rules for detecting path parameters:
- Path segments that vary while structure stays same → parameter
  Example: GET /users/123, GET /users/456 → GET /users/{id}
- UUIDs, numeric IDs, usernames, slugs are typically parameters
- Version prefixes (v1, v2, v3) are LITERAL, not parameters
- Resource names (users, posts, videos) are LITERAL
- Nested resources: /users/{user_id}/posts/{post_id}

Infer parameter types:
- All digits → integer
- UUID format → uuid
- Alphanumeric with dashes → slug or username
- Other → string

Output valid JSON only, no markdown.`

	userPrompt := fmt.Sprintf(`Analyze these API paths and infer URL patterns that would match them all:

%s

Return JSON array - each pattern should match one or more paths above:
[
  {
    "pattern": "/v3/profiles/{username}",
    "method": "GET",
    "description": "Get user profile by username",
    "path_params": {"username": "string"},
    "matched_paths": ["/v3/profiles/john", "/v3/profiles/jane"]
  }
]

IMPORTANT: Every path in the input should be matched by exactly one pattern.`, pathsText)

	resp, err := s.provider.Complete(ctx, &CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.1,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM completion failed: %w", err)
	}

	// Parse response with flexible method handling (can be string or array)
	content := strings.TrimSpace(resp.Content)
	// Strip markdown code blocks if present
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var rawPatterns []rawInferredPattern
	if err := json.Unmarshal([]byte(content), &rawPatterns); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w (response: %s)", err, content)
	}

	// Expand patterns: if method is an array, create one pattern per method
	var patterns []InferredPattern
	for _, raw := range rawPatterns {
		methods := parseMethod(raw.Method)
		if len(methods) == 0 {
			// Default to GET if no method specified
			methods = []string{"GET"}
		}

		for _, method := range methods {
			patterns = append(patterns, InferredPattern{
				Pattern:     raw.Pattern,
				Method:      method,
				Description: raw.Description,
				PathParams:  raw.PathParams,
				QueryParams: raw.QueryParams,
			})
		}
	}

	return patterns, nil
}

// InferEndpointSchema infers request and response schemas for a specific endpoint pattern
func (s *InferenceService) InferEndpointSchema(ctx context.Context, pattern, method string, samples []EndpointSample) (string, error) {
	if len(samples) == 0 {
		return "", nil
	}

	// Format samples for the prompt
	var sampleTexts []string
	for i, sample := range samples {
		text := fmt.Sprintf("Sample %d:\n  Path: %s\n  Status: %d", i+1, sample.Path, sample.StatusCode)
		if sample.RequestBody != "" {
			text += fmt.Sprintf("\n  Request Body: %s", sample.RequestBody)
		}
		if sample.ResponseBody != "" {
			text += fmt.Sprintf("\n  Response Body: %s", sample.ResponseBody)
		}
		sampleTexts = append(sampleTexts, text)
	}

	systemPrompt := `You are an API schema analyzer. Given sample requests/responses for an endpoint, infer the typed schemas.

Your goal is to define types for a TYPED ROUTER:
- Path parameters (extracted from URL pattern)
- Query parameters (if visible)
- Request body schema (for POST/PUT/PATCH)
- Response body schema

Rules:
- Infer field types: string, integer, number, boolean, array, object
- Detect nullable fields (missing in some samples)
- Identify enums if values are from a limited set
- Use descriptive field names
- Note required vs optional fields

Output valid JSON only, no markdown.`

	userPrompt := fmt.Sprintf(`Analyze this endpoint and infer schemas:

Endpoint: %s %s

%s

Return JSON with request and response schemas:
{
  "path_params": {
    "id": {"type": "integer", "description": "Resource ID"}
  },
  "query_params": {
    "include": {"type": "string", "required": false, "description": "Relations to include"}
  },
  "request_body": {
    "type": "object",
    "properties": {
      "name": {"type": "string", "required": true}
    }
  },
  "response_body": {
    "type": "object",
    "properties": {
      "id": {"type": "integer"},
      "name": {"type": "string"},
      "created_at": {"type": "string", "format": "date-time"}
    }
  }
}

Only include sections that apply (e.g., GET usually has no request_body).`, method, pattern, strings.Join(sampleTexts, "\n\n"))

	resp, err := s.provider.Complete(ctx, &CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.1,
	})
	if err != nil {
		return "", fmt.Errorf("LLM completion failed: %w", err)
	}

	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	return content, nil
}

// InferSchema analyzes request/response bodies and infers schema
func (s *InferenceService) InferSchema(ctx context.Context, samples []EndpointSample) (*InferredSchema, error) {
	if len(samples) == 0 {
		return nil, nil
	}

	// Prepare sample bodies for analysis
	var bodies []string
	for _, sample := range samples {
		if sample.ResponseBody != "" {
			bodies = append(bodies, sample.ResponseBody)
		}
	}

	if len(bodies) == 0 {
		return nil, nil
	}

	// Limit samples to avoid token limits
	if len(bodies) > 5 {
		bodies = bodies[:5]
	}

	systemPrompt := `You are a JSON schema analyzer. Given sample JSON responses, infer the data schema.

Rules:
- Identify field types (string, integer, number, boolean, array, object)
- Detect nullable fields (fields missing in some samples)
- Identify enums (fields with limited set of values)
- Detect date/time formats
- Note required vs optional fields

Output valid JSON only, no markdown.`

	userPrompt := fmt.Sprintf(`Analyze these JSON response samples and infer the schema:

%s

Return JSON:
{
  "name": "User",
  "description": "User account information",
  "fields": {
    "id": {"type": "integer", "required": true, "description": "Unique identifier"},
    "email": {"type": "string", "required": true, "description": "Email address"},
    "role": {"type": "string", "required": false, "description": "User role", "example": "admin"}
  }
}`, strings.Join(bodies, "\n\n---\n\n"))

	resp, err := s.provider.Complete(ctx, &CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.1,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM completion failed: %w", err)
	}

	// Parse response
	var schema InferredSchema
	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	if err := json.Unmarshal([]byte(content), &schema); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return &schema, nil
}

// Correlation represents a detected relationship between requests
type Correlation struct {
	FromRequest   string `json:"from_request"`   // Request ID that produced the value
	ToRequest     string `json:"to_request"`     // Request ID that consumed the value
	ValuePath     string `json:"value_path"`     // JSON path in source response (e.g., "$.id")
	UsedIn        string `json:"used_in"`        // Where value was used: "path", "query", "header", "body"
	UsedAs        string `json:"used_as"`        // What it represents (e.g., "user_id", "auth_token")
	Description   string `json:"description"`    // Human-readable description
}

// FlowStep represents a step in an API workflow
type FlowStep struct {
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	Description string   `json:"description"`
	Consumes    []string `json:"consumes,omitempty"` // Values consumed from previous steps
	Produces    []string `json:"produces,omitempty"` // Values produced for later steps
}

// APIFlow represents a detected workflow pattern
type APIFlow struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Steps       []FlowStep `json:"steps"`
}

// CorrelateRequests analyzes a sequence of requests to find data dependencies
func (s *InferenceService) CorrelateRequests(ctx context.Context, samples []EndpointSample) ([]Correlation, []APIFlow, error) {
	if len(samples) < 2 {
		return nil, nil, nil
	}

	// Prepare samples with request/response data
	sampleJSON, _ := json.MarshalIndent(samples, "", "  ")

	systemPrompt := `You are an API flow analyzer. Given a sequence of HTTP requests with their responses, identify data dependencies between them.

Look for:
1. Values from a response that appear in subsequent requests (IDs, tokens, cursors)
2. Authentication flows (login returns token, later requests use it)
3. CRUD patterns (create returns ID, later operations use that ID)
4. Pagination (response has next_page, next request uses it)
5. Resource relationships (user_id in /users response used in /users/{id}/posts)

Output valid JSON only, no markdown.`

	userPrompt := fmt.Sprintf(`Analyze this sequence of HTTP requests/responses and identify data flow correlations:

%s

Return JSON with two arrays:
{
  "correlations": [
    {
      "from_request": "POST /users",
      "to_request": "GET /users/123",
      "value_path": "$.id",
      "used_in": "path",
      "used_as": "user_id",
      "description": "User ID from creation used to fetch user details"
    }
  ],
  "flows": [
    {
      "name": "User Creation Flow",
      "description": "Create user and fetch details",
      "steps": [
        {"method": "POST", "path": "/users", "description": "Create user", "produces": ["user_id"]},
        {"method": "GET", "path": "/users/{id}", "description": "Get user details", "consumes": ["user_id"]}
      ]
    }
  ]
}`, string(sampleJSON))

	resp, err := s.provider.Complete(ctx, &CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.1,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("LLM completion failed: %w", err)
	}

	// Parse response
	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var result struct {
		Correlations []Correlation `json:"correlations"`
		Flows        []APIFlow     `json:"flows"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return result.Correlations, result.Flows, nil
}

// GenerateCode generates code in the specified format from an OpenAPI spec
func (s *InferenceService) GenerateCode(ctx context.Context, openAPISpec, format, host string) (string, error) {
	if openAPISpec == "" {
		return "", nil
	}

	var formatInstructions string
	switch format {
	case "protobuf":
		formatInstructions = `Generate Protocol Buffers (proto3) definitions.
Rules:
- Use proto3 syntax
- Create message types for each schema
- Use appropriate protobuf types (int64 for IDs, string for text, etc.)
- Add package name based on the host
- Use nested messages for embedded objects
- Add service definition with rpc methods for each endpoint`

	case "typescript":
		formatInstructions = `Generate TypeScript interfaces and types.
Rules:
- Create interface for each schema
- Use TypeScript types (number, string, boolean, etc.)
- Use optional (?) for nullable/optional fields
- Export all interfaces
- Add type for API client with methods for each endpoint`

	case "go":
		formatInstructions = `Generate Go struct definitions.
Rules:
- Create struct for each schema
- Add json tags for field names
- Use appropriate Go types (int64, string, bool, etc.)
- Use pointers for optional fields
- Use time.Time for datetime fields
- Add omitempty for optional fields`

	default:
		return "", fmt.Errorf("unsupported format: %s", format)
	}

	systemPrompt := fmt.Sprintf(`You are a code generator. Convert OpenAPI/JSON schema to %s code.

%s

Output ONLY the code, no explanations or markdown fences.`, format, formatInstructions)

	userPrompt := fmt.Sprintf(`Generate %s code from this API schema for host %s:

%s

Generate clean, production-ready code with proper types for all schemas.`, format, host, openAPISpec)

	resp, err := s.provider.Complete(ctx, &CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.1,
	})
	if err != nil {
		return "", fmt.Errorf("LLM completion failed: %w", err)
	}

	content := strings.TrimSpace(resp.Content)
	// Strip markdown code fences if present
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) > 2 {
			// Remove first and last lines (fences)
			content = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	return content, nil
}

// NormalizeSchemas analyzes multiple endpoint schemas and factors out common types
func (s *InferenceService) NormalizeSchemas(ctx context.Context, schemaTexts []string) (string, error) {
	if len(schemaTexts) < 2 {
		return "", nil
	}

	allSchemas := strings.Join(schemaTexts, "\n\n---\n\n")

	systemPrompt := `You are an API schema architect. Given multiple endpoint schemas, identify common structures and create a normalized OpenAPI 3.0 specification with shared component definitions.

Goals:
1. Find duplicate/similar object structures across endpoints
2. Extract them as named schemas (e.g., User, Post, Comment)
3. Replace inline definitions with $ref references
4. Create a clean, DRY schema definition

CRITICAL - OpenAPI 3.0 Compliance Rules:
- EVERY operation MUST have "operationId" (unique, camelCase like "getUsers", "createPost")
- EVERY operation MUST have "summary" (short title) AND "description" (longer explanation)
- EVERY response (200, 201, etc.) MUST have "description" field
- EVERY parameter MUST have "description" field
- Path parameters MUST have "required": true and "in": "path"
- Query parameters MUST have "in": "query" (NOT "path")
- info object MUST have "title", "version", AND "description"
- Include "tags" array at root level for operation grouping

Schema Rules:
- If same structure appears with different names (author, owner, user) → create one User schema
- Nested objects that repeat → extract as separate schema
- Keep primitive types inline, extract complex objects
- Use OpenAPI 3.0 components/schemas format

Output valid JSON only, no markdown.`

	userPrompt := fmt.Sprintf(`Analyze these endpoint schemas and create a normalized API specification with shared types:

%s

Return OpenAPI 3.0 style JSON:
{
  "openapi": "3.0.0",
  "info": {
    "title": "API Title",
    "version": "1.0.0",
    "description": "API description"
  },
  "tags": [{"name": "posts", "description": "Post operations"}],
  "components": {
    "schemas": {
      "User": {
        "type": "object",
        "properties": {
          "id": {"type": "integer"},
          "name": {"type": "string"},
          "avatar_url": {"type": "string"}
        }
      }
    }
  },
  "paths": {
    "/posts/{id}": {
      "get": {
        "operationId": "getPostById",
        "summary": "Get a post by ID",
        "description": "Retrieves a specific post by its unique identifier",
        "tags": ["posts"],
        "parameters": [
          {
            "name": "id",
            "in": "path",
            "required": true,
            "description": "The post ID",
            "schema": {"type": "integer"}
          }
        ],
        "responses": {
          "200": {
            "description": "Successful response with post data",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "id": {"type": "integer"},
                    "title": {"type": "string"},
                    "author": {"$ref": "#/components/schemas/User"}
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`, allSchemas)

	resp, err := s.provider.Complete(ctx, &CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.1,
	})
	if err != nil {
		return "", fmt.Errorf("LLM completion failed: %w", err)
	}

	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	return content, nil
}

// NormalizeSchemasWithFeedback regenerates the schema with validation error feedback
func (s *InferenceService) NormalizeSchemasWithFeedback(ctx context.Context, schemaTexts []string, validationErrors []string) (string, error) {
	if len(schemaTexts) < 2 {
		return "", nil
	}

	allSchemas := strings.Join(schemaTexts, "\n\n---\n\n")
	errorFeedback := strings.Join(validationErrors, "\n")

	systemPrompt := `You are an API schema architect. Given multiple endpoint schemas, identify common structures and create a normalized OpenAPI 3.0 specification.

IMPORTANT: Your previous schema attempt failed validation. Review the validation errors carefully and fix the schema to match the actual API traffic patterns.

CRITICAL - OpenAPI 3.0 Compliance Rules (MUST follow):
- EVERY operation MUST have "operationId" (unique, camelCase like "getUsers", "createPost")
- EVERY operation MUST have "summary" (short title) AND "description" (longer explanation)
- EVERY response (200, 201, 400, etc.) MUST have "description" field (e.g., "description": "OK")
- EVERY parameter MUST have "description" field
- Path parameters: "in": "path", "required": true
- Query parameters: "in": "query" (NOT "path"!)
- info object MUST have "title", "version", AND "description"
- Include "tags" array at root level for operation grouping

Common issues to fix:
- Missing required fields that are present in actual traffic
- Wrong parameter types (e.g., path params should be strings unless proven numeric)
- Missing path patterns that exist in the traffic
- Incorrect $ref paths
- Schema structure not matching actual request/response bodies
- Query parameters incorrectly marked as path parameters

Goals:
1. Find duplicate/similar object structures across endpoints
2. Extract them as named schemas (e.g., User, Post, Comment)
3. Replace inline definitions with $ref references
4. Create a clean, DRY schema definition
5. FIX THE VALIDATION ERRORS listed below

Schema Rules:
- If same structure appears with different names (author, owner, user) → create one User schema
- Nested objects that repeat → extract as separate schema
- Keep primitive types inline, extract complex objects
- Use OpenAPI 3.0 components/schemas format

Output valid JSON only, no markdown.`

	userPrompt := fmt.Sprintf(`VALIDATION ERRORS FROM PREVIOUS ATTEMPT:
%s

---

ENDPOINT SCHEMAS TO NORMALIZE:
%s

---

Fix the validation errors and return a corrected OpenAPI 3.0 JSON specification. Make sure:
1. All paths from the traffic are included
2. Path parameters are correctly defined in the parameters array
3. Request/response schemas match the actual traffic structure
4. $ref paths are correct (e.g., "#/components/schemas/User")

Return valid OpenAPI 3.0 JSON:`, errorFeedback, allSchemas)

	resp, err := s.provider.Complete(ctx, &CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.1,
	})
	if err != nil {
		return "", fmt.Errorf("LLM completion failed: %w", err)
	}

	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	return content, nil
}

// GenerateSequenceDiagram creates a Mermaid sequence diagram from API flows
func (s *InferenceService) GenerateSequenceDiagram(ctx context.Context, samples []EndpointSample) (string, error) {
	if len(samples) < 2 {
		return "", nil
	}

	sampleJSON, _ := json.MarshalIndent(samples, "", "  ")

	systemPrompt := `You are an API flow analyzer. Given HTTP request/response sequences, generate a clean Mermaid sequence diagram showing the logical flow.

IMPORTANT - Intelligent Deduplication:
- SKIP CORS preflight (OPTIONS) requests entirely
- COLLAPSE truly redundant repeated requests (e.g., polling same endpoint) into a single call with a note like "repeated N times"
- KEEP meaningful sequences even if similar (e.g., upload progress showing state changes, pagination through results)
- INFER the logical workflow, don't just list every request

Rules:
- Use "Client" as the actor
- Group endpoints into logical services (e.g., "Auth", "API", "Media")
- Show request arrows with method and simplified path (use {id} for IDs)
- Show response arrows with status and key data returned
- Add notes for important data flow between requests (tokens, IDs used later)
- Keep diagram readable: aim for 5-15 meaningful steps, not raw request log
- If requests show a clear pattern (auth -> fetch data -> update), highlight that flow

Output ONLY the Mermaid diagram code starting with "sequenceDiagram", no markdown fences.`

	userPrompt := fmt.Sprintf(`Analyze this API traffic and generate a clean sequence diagram showing the logical flow:

%s

Generate a readable diagram that shows the WORKFLOW, not every raw request. Collapse noise, keep meaningful sequences.

Example output:
sequenceDiagram
    participant C as Client
    participant A as Auth
    participant API as API

    C->>A: POST /auth/login
    A-->>C: 200 {token}
    Note over C: Store JWT
    C->>API: GET /users/me
    API-->>C: 200 {user}
    C->>API: GET /videos
    API-->>C: 200 {list}`, string(sampleJSON))

	resp, err := s.provider.Complete(ctx, &CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.2,
	})
	if err != nil {
		return "", fmt.Errorf("LLM completion failed: %w", err)
	}

	content := strings.TrimSpace(resp.Content)
	// Clean up any markdown if present
	content = strings.TrimPrefix(content, "```mermaid")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	return content, nil
}

// GenerateSchema generates a schema in the specified format
func (s *InferenceService) GenerateSchema(ctx context.Context, schema *InferredSchema, format models.SchemaFormat) (string, error) {
	if schema == nil {
		return "", nil
	}

	schemaJSON, _ := json.MarshalIndent(schema, "", "  ")

	systemPrompt := fmt.Sprintf(`You are a code generator. Convert the given schema to %s format.

Output only the generated code, no explanations or markdown.`, format)

	userPrompt := fmt.Sprintf(`Convert this schema to %s:

%s`, format, string(schemaJSON))

	resp, err := s.provider.Complete(ctx, &CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.1,
	})
	if err != nil {
		return "", fmt.Errorf("LLM completion failed: %w", err)
	}

	content := strings.TrimSpace(resp.Content)
	// Strip markdown if present
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) > 2 {
			content = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	return content, nil
}
