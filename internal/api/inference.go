package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"prism/internal/codegen"
	"prism/internal/inference"
	"prism/internal/llm"
	"prism/internal/storage"
	"prism/pkg/models"
)

// InferenceManager handles AI inference operations
type InferenceManager struct {
	db       *storage.DB
	provider llm.Provider
	service  *llm.InferenceService
	logger   *zap.Logger
	mu       sync.Mutex
	running  map[string]bool // tracks hosts currently being analyzed
}

// NewInferenceManager creates a new inference manager
func NewInferenceManager(db *storage.DB, cfg *models.LLMConfig, logger *zap.Logger) (*InferenceManager, error) {
	provider, err := llm.NewProvider(&llm.Config{
		Provider:    cfg.Provider,
		APIKey:      cfg.APIKey,
		BaseURL:     cfg.BaseURL,
		Model:       cfg.Model,
		MaxTokens:   cfg.MaxTokens,
		Temperature: cfg.Temperature,
	})
	if err != nil {
		return nil, err
	}

	return &InferenceManager{
		db:       db,
		provider: provider,
		service:  llm.NewInferenceService(provider),
		logger:   logger,
		running:  make(map[string]bool),
	}, nil
}

// InferenceStatus represents the status of an inference job
type InferenceStatus struct {
	Host    string `json:"host"`
	Status  string `json:"status"` // "pending", "running", "completed", "failed"
	Message string `json:"message,omitempty"`
}

// handleAnalyzeHost triggers AI inference for a specific host
func (s *Server) handleAnalyzeHost(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	if host == "" {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Host is required")
		return
	}

	if s.inference == nil {
		s.respondError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE",
			"LLM provider not configured. Set ANTHROPIC_API_KEY or configure llm section in proxy.yaml")
		return
	}

	// Check if already running
	s.inference.mu.Lock()
	if s.inference.running[host] {
		s.inference.mu.Unlock()
		s.respondJSON(w, http.StatusAccepted, InferenceStatus{
			Host:   host,
			Status: "running",
		})
		return
	}
	s.inference.running[host] = true
	s.inference.mu.Unlock()

	// Run inference in background
	go func() {
		defer func() {
			s.inference.mu.Lock()
			delete(s.inference.running, host)
			s.inference.mu.Unlock()
		}()

		ctx := context.Background()
		if err := s.runInference(ctx, host); err != nil {
			s.logger.Error("Inference failed", zap.String("host", host), zap.Error(err))
		}
	}()

	s.respondJSON(w, http.StatusAccepted, InferenceStatus{
		Host:    host,
		Status:  "running",
		Message: "Analysis started. Check schemas endpoint for results.",
	})
}

// runInference performs the actual inference for a host in phases
func (s *Server) runInference(ctx context.Context, host string) error {
	s.logger.Info("Starting inference", zap.String("host", host))

	// Get ALL requests for this host to find all unique paths
	filter := &storage.RequestFilter{
		Host:  host,
		Limit: 500, // Get more to find all path variants
	}

	requests, total, err := s.db.ListRequests(ctx, filter)
	if err != nil {
		return err
	}

	if len(requests) == 0 {
		s.logger.Warn("No requests found for host", zap.String("host", host))
		return nil
	}

	s.logger.Info("Found requests", zap.String("host", host), zap.Int("fetched", len(requests)), zap.Int64("total", total))

	// Phase 1: Extract ALL unique method+path combinations
	// This is lightweight - just paths, no bodies
	seen := make(map[string]bool)
	var urlSamples []llm.EndpointSample
	for _, req := range requests {
		key := req.Method + " " + req.Path
		if !seen[key] {
			seen[key] = true
			urlSamples = append(urlSamples, llm.EndpointSample{
				Method:     req.Method,
				Path:       req.Path,
				StatusCode: req.StatusCode,
			})
		}
	}

	s.logger.Info("Phase 1: Inferring URL patterns", zap.String("host", host), zap.Int("unique_paths", len(urlSamples)))

	patterns, err := s.inference.service.InferPatterns(ctx, urlSamples)
	if err != nil {
		s.logger.Error("Pattern inference failed", zap.Error(err))
	} else {
		s.logger.Info("Inferred patterns from LLM", zap.Int("count", len(patterns)))
		// Note: We'll save patterns after refining them with response body analysis in Phase 2
	}

	// Phase 2: For each pattern, infer request/response schemas
	s.logger.Info("Phase 2: Inferring schemas per pattern", zap.String("host", host), zap.Int("patterns", len(patterns)))

	const maxBodySize = 2000
	const samplesPerPattern = 3

	for _, p := range patterns {
		s.logger.Info("Inferring schema for pattern", zap.String("pattern", p.Pattern), zap.String("method", p.Method))

		// Find requests matching this pattern (simple prefix match for now)
		var matchingSamples []llm.EndpointSample
		patternPrefix := extractPatternPrefix(p.Pattern)

		for _, req := range requests {
			if req.Method != p.Method {
				continue
			}
			if !strings.HasPrefix(req.Path, patternPrefix) {
				continue
			}
			if len(matchingSamples) >= samplesPerPattern {
				break
			}

			// Get full capture with body
			capture, err := s.db.GetCapture(ctx, req.ID)
			if err != nil || capture == nil {
				continue
			}

			sample := llm.EndpointSample{
				Method:     capture.Request.Method,
				Path:       capture.Request.Path,
				StatusCode: capture.Response.StatusCode,
			}

			// Include request body if JSON
			if strings.Contains(capture.Request.ContentType, "json") && len(capture.Request.Body) > 0 {
				body := string(capture.Request.Body)
				if len(body) > maxBodySize {
					body = body[:maxBodySize] + "..."
				}
				sample.RequestBody = body
			}

			// Include response body if JSON
			if strings.Contains(capture.Response.ContentType, "json") && len(capture.Response.Body) > 0 {
				body := string(capture.Response.Body)
				if len(body) > maxBodySize {
					body = body[:maxBodySize] + "..."
				}
				sample.ResponseBody = body
			}

			if sample.RequestBody != "" || sample.ResponseBody != "" {
				matchingSamples = append(matchingSamples, sample)
			}
		}

		// Refine pattern using response body analysis
		// This looks for path segment values that appear in response fields
		refinedPattern := p.Pattern
		if len(matchingSamples) > 0 {
			var allMatches [][]inference.ParamMatch
			for _, sample := range matchingSamples {
				if sample.ResponseBody != "" {
					matches := inference.InferParamNamesFromResponse(sample.Path, []byte(sample.ResponseBody))
					if len(matches) > 0 {
						allMatches = append(allMatches, matches)
					}
				}
			}

			if len(allMatches) > 0 {
				// Aggregate matches and get most common field names per segment
				aggregated := inference.AggregateParamMatches(allMatches)
				if len(aggregated) > 0 {
					// Convert to ParamMatch slice for RefinePathPattern
					var refinementMatches []inference.ParamMatch
					for segIdx, fieldName := range aggregated {
						refinementMatches = append(refinementMatches, inference.ParamMatch{
							SegmentIndex: segIdx,
							FieldName:    fieldName,
						})
					}
					refinedPattern = inference.RefinePathPattern(p.Pattern, refinementMatches)
					if refinedPattern != p.Pattern {
						s.logger.Info("Refined pattern using response body analysis",
							zap.String("original", p.Pattern),
							zap.String("refined", refinedPattern))
					}
				}
			}
		}

		// Save the refined pattern
		endpointPattern := &models.EndpointPattern{
			Host:        host,
			Method:      p.Method,
			PathPattern: refinedPattern,
			SampleCount: len(matchingSamples),
		}
		if err := s.db.SaveEndpointPattern(ctx, endpointPattern); err != nil {
			s.logger.Error("Failed to save pattern", zap.Error(err))
		}

		if len(matchingSamples) > 0 {
			s.logger.Info("Calling AI for schema inference",
				zap.String("pattern", refinedPattern),
				zap.Int("samples", len(matchingSamples)))
			schema, err := s.inference.service.InferEndpointSchema(ctx, refinedPattern, p.Method, matchingSamples)
			if err != nil {
				s.logger.Error("Schema inference failed", zap.String("pattern", refinedPattern), zap.Error(err))
				continue
			}
			if schema == "" {
				s.logger.Warn("Empty schema returned", zap.String("pattern", refinedPattern))
				continue
			}
			s.logger.Info("Schema inferred successfully",
				zap.String("pattern", refinedPattern),
				zap.Int("schema_length", len(schema)))
			if err := s.db.SaveInferredSchema(ctx, host, p.Method, refinedPattern, "json-schema", schema, len(matchingSamples)); err != nil {
				s.logger.Error("Failed to save schema", zap.Error(err))
			}
		} else {
			s.logger.Warn("No matching samples with JSON body",
				zap.String("pattern", refinedPattern),
				zap.String("prefix", patternPrefix))
		}
	}

	// Phase 3: Normalize schemas - factor out common types with validation retry
	s.logger.Info("Phase 3: Normalizing schemas", zap.String("host", host))

	// Get all schemas we just saved
	allSchemas, err := s.db.GetSchemasByFormat(ctx, host, "json-schema")
	s.logger.Info("Found json-schemas for normalization", zap.Int("count", len(allSchemas)))
	if err != nil {
		s.logger.Error("Failed to get json-schemas", zap.Error(err))
	}
	if len(allSchemas) <= 1 {
		s.logger.Warn("Not enough schemas for normalization (need at least 2)", zap.Int("count", len(allSchemas)))
	}
	if err == nil && len(allSchemas) > 1 {
		// Collect all schema contents
		var schemaTexts []string
		for _, schema := range allSchemas {
			schemaTexts = append(schemaTexts, fmt.Sprintf("Endpoint: %s %s\nSchema: %s",
				schema.Method, schema.PathPattern, schema.Content))
		}

		const maxRetries = 3
		var lastErrors []string
		var validationSuccess bool

		for attempt := 1; attempt <= maxRetries; attempt++ {
			s.logger.Info("Schema normalization attempt",
				zap.Int("attempt", attempt),
				zap.Int("max_retries", maxRetries),
				zap.String("host", host))

			// Generate normalized OpenAPI spec (with error feedback on retries)
			var normalized string
			if attempt == 1 {
				normalized, err = s.inference.service.NormalizeSchemas(ctx, schemaTexts)
			} else {
				// Retry with error feedback
				normalized, err = s.inference.service.NormalizeSchemasWithFeedback(ctx, schemaTexts, lastErrors)
			}

			if err != nil {
				s.logger.Error("Schema normalization failed", zap.Error(err))
				break
			}

			if normalized == "" {
				s.logger.Warn("Empty normalized schema returned")
				break
			}

			// Sanitize OpenAPI spec to fix common AI-generated issues
			// (e.g., arrays missing 'items' property)
			sanitized, sanitizeErr := codegen.SanitizeOpenAPI(normalized)
			if sanitizeErr != nil {
				s.logger.Warn("Failed to sanitize OpenAPI spec, using original",
					zap.Error(sanitizeErr))
			} else if sanitized != normalized {
				s.logger.Info("Sanitized OpenAPI spec (fixed common issues)",
					zap.Int("original_len", len(normalized)),
					zap.Int("sanitized_len", len(sanitized)))
				normalized = sanitized
			} else {
				s.logger.Debug("OpenAPI spec needs no sanitization")
			}

			// CRITICAL: Validate the OpenAPI spec BEFORE saving
			// This catches truncation errors (token limit exceeded)
			specGen, parseErr := codegen.NewGenerator(normalized)
			if parseErr != nil {
				// FATAL error - JSON is malformed/truncated
				// Retrying won't help if we hit the token limit
				s.logger.Error("OpenAPI spec is invalid - FATAL (possibly truncated due to token limit)",
					zap.Error(parseErr),
					zap.Int("spec_length", len(normalized)))
				break // Don't retry - this is fatal
			}
			_ = specGen // Used just for validation
			s.logger.Info("OpenAPI spec parsed successfully", zap.Int("spec_length", len(normalized)))

			// Save the validated schema
			if err := s.db.SaveInferredSchema(ctx, host, "*", "/*", "openapi", normalized, len(allSchemas)); err != nil {
				s.logger.Error("Failed to save normalized schema", zap.Error(err))
				break
			}
			s.logger.Info("Normalized schemas with shared types")

			// Phase 3.5: Validate historical requests against the generated schema
			s.logger.Info("Validating schema against historical requests", zap.String("host", host))

			// Reload spec in validator (should succeed since we just validated above)
			if err := s.validator.LoadSpec(ctx, host); err != nil {
				s.logger.Error("Failed to load spec for validation", zap.Error(err))
				break // Fatal - spec should have loaded
			}

			validationResults, err := s.validator.ValidateHost(ctx, host, 50)
			if err != nil {
				s.logger.Error("Validation failed", zap.Error(err))
				lastErrors = append(lastErrors, fmt.Sprintf("Validation error: %v", err))
				continue
			}

			// Calculate error rate
			var invalidCount int
			lastErrors = nil // Reset for this attempt
			for _, result := range validationResults {
				if !result.Valid {
					invalidCount++
					// Collect unique error messages for feedback
					for _, e := range result.Errors {
						if e.Location != "spec" { // Skip spec-level errors
							errMsg := fmt.Sprintf("%s %s: %s - %s", result.Method, result.Path, e.Location, e.Message)
							lastErrors = append(lastErrors, errMsg)
						}
					}
				}
			}

			errorRate := float64(invalidCount) / float64(len(validationResults))
			s.logger.Info("Validation results",
				zap.Int("total", len(validationResults)),
				zap.Int("invalid", invalidCount),
				zap.Float64("error_rate", errorRate))

			// If error rate is acceptable (< 30%), consider success
			if errorRate < 0.30 {
				s.logger.Info("Schema validation passed",
					zap.Float64("error_rate", errorRate),
					zap.Int("attempt", attempt))
				validationSuccess = true
				break
			}

			// Deduplicate errors for AI feedback
			if len(lastErrors) > 10 {
				lastErrors = lastErrors[:10] // Limit to top 10 errors
			}

			s.logger.Warn("Schema validation failed, will retry with error feedback",
				zap.Float64("error_rate", errorRate),
				zap.Int("attempt", attempt),
				zap.Int("error_count", len(lastErrors)))
		}

		if !validationSuccess && len(lastErrors) > 0 {
			s.logger.Warn("Schema validation failed after max retries",
				zap.Int("max_retries", maxRetries),
				zap.Strings("last_errors", lastErrors))
		}
	}

	// Phase 4: Generate code from OpenAPI spec (deterministic - no AI)
	s.logger.Info("Phase 4: Generating code formats (deterministic)", zap.String("host", host))

	// Get the normalized OpenAPI spec we just created
	openapiSchemas, err := s.db.GetSchemasByFormat(ctx, host, "openapi")
	if err == nil && len(openapiSchemas) > 0 {
		openapiContent := openapiSchemas[0].Content

		// Create deterministic code generator
		gen, err := codegen.NewGenerator(openapiContent)
		if err != nil {
			s.logger.Error("Failed to create code generator", zap.Error(err))
		} else {
			// Generate TypeScript (deterministic)
			ts, err := gen.GenerateTypeScript()
			if err != nil {
				s.logger.Error("TypeScript generation failed", zap.Error(err))
			} else if ts != "" {
				s.db.SaveInferredSchema(ctx, host, "*", "/*", "typescript", ts, 0)
				s.logger.Info("Generated TypeScript interfaces (deterministic)")
			}

			// Generate Go structs (deterministic)
			// Use host as package name, sanitized
			pkgName := sanitizePackageName(host)
			goCode, err := gen.GenerateGo(pkgName)
			if err != nil {
				s.logger.Error("Go generation failed", zap.Error(err))
			} else if goCode != "" {
				s.db.SaveInferredSchema(ctx, host, "*", "/*", "go", goCode, 0)
				s.logger.Info("Generated Go structs (deterministic)")
			}

			// Generate Protobuf (deterministic)
			proto, err := gen.GenerateProtobuf(pkgName)
			if err != nil {
				s.logger.Error("Protobuf generation failed", zap.Error(err))
			} else if proto != "" {
				s.db.SaveInferredSchema(ctx, host, "*", "/*", "protobuf", proto, 0)
				s.logger.Info("Generated Protobuf schema (deterministic)")
			}

			// Generate JSON Schema (deterministic)
			jsonSchema, err := gen.GenerateJSONSchema()
			if err != nil {
				s.logger.Error("JSON Schema generation failed", zap.Error(err))
			} else if jsonSchema != "" {
				s.db.SaveInferredSchema(ctx, host, "*", "/*", "json-schema", jsonSchema, 0)
				s.logger.Info("Generated JSON Schema (deterministic)")
			}
		}
	}

	// Phase 5: Generate sequence diagram (AI will intelligently deduplicate)
	s.logger.Info("Phase 5: Generating sequence diagram", zap.String("host", host))

	if len(urlSamples) >= 2 {
		// Send up to 30 samples - AI will intelligently collapse/dedupe
		diagramSamples := urlSamples
		if len(diagramSamples) > 30 {
			diagramSamples = diagramSamples[:30]
		}

		diagram, err := s.inference.service.GenerateSequenceDiagram(ctx, diagramSamples)
		if err != nil {
			s.logger.Error("Diagram generation failed", zap.Error(err))
		} else if diagram != "" {
			s.logger.Info("Generated sequence diagram")
			if err := s.db.SaveInferredSchema(ctx, host, "*", "/*", "mermaid", diagram, len(diagramSamples)); err != nil {
				s.logger.Error("Failed to save diagram", zap.Error(err))
			}
		}
	}

	s.logger.Info("Inference completed", zap.String("host", host))
	return nil
}

// extractPatternPrefix gets the literal prefix of a pattern before any {param}
func extractPatternPrefix(pattern string) string {
	parts := strings.Split(pattern, "/")
	var prefix []string
	for _, part := range parts {
		if strings.HasPrefix(part, "{") {
			break
		}
		prefix = append(prefix, part)
	}
	result := strings.Join(prefix, "/")
	if result == "" {
		return "/"
	}
	return result
}

// sanitizePackageName converts a hostname to a valid Go/Protobuf package name
// e.g., "api.example.com" -> "api_example_com"
func sanitizePackageName(host string) string {
	// Remove port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	// Replace dots and hyphens with underscores
	re := regexp.MustCompile(`[.\-]`)
	name := re.ReplaceAllString(host, "_")

	// Ensure it starts with a letter (prepend 'pkg_' if it starts with a number)
	if len(name) > 0 && name[0] >= '0' && name[0] <= '9' {
		name = "pkg_" + name
	}

	// Remove any remaining invalid characters
	re = regexp.MustCompile(`[^a-zA-Z0-9_]`)
	name = re.ReplaceAllString(name, "")

	// Convert to lowercase for consistency
	return strings.ToLower(name)
}

// handleGetInferenceStatus gets the status of inference for a host
func (s *Server) handleGetInferenceStatus(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")

	if s.inference == nil {
		s.respondError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE",
			"LLM provider not configured")
		return
	}

	s.inference.mu.Lock()
	running := s.inference.running[host]
	s.inference.mu.Unlock()

	status := "pending"
	if running {
		status = "running"
	} else {
		// Check if we have schemas for this host
		schemas, err := s.db.ListSchemas(r.Context(), host)
		if err == nil && len(schemas) > 0 {
			status = "completed"
		}
	}

	s.respondJSON(w, http.StatusOK, InferenceStatus{
		Host:   host,
		Status: status,
	})
}

// SchemaFixResponse represents an AI-generated schema fix
type SchemaFixResponse struct {
	FixedSchema string   `json:"fixed_schema"`
	Changes     []string `json:"changes"`
	Reasoning   string   `json:"reasoning"`
}

// FixSchema uses AI to fix an OpenAPI schema based on validation errors
func (im *InferenceManager) FixSchema(ctx context.Context, currentSchema string, validationErrors []string, sampleData string) (*SchemaFixResponse, error) {
	req := &llm.SchemaFixRequest{
		CurrentSchema:    currentSchema,
		ValidationErrors: validationErrors,
		SampleData:       sampleData,
	}

	resp, err := im.service.FixSchemaErrors(ctx, req)
	if err != nil {
		return nil, err
	}

	return &SchemaFixResponse{
		FixedSchema: resp.FixedSchema,
		Changes:     resp.Changes,
		Reasoning:   resp.Reasoning,
	}, nil
}

// handleGetDiagram returns the Mermaid diagram for a host
func (s *Server) handleGetDiagram(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")

	// Get the mermaid diagram from stored schemas
	schemas, err := s.db.GetSchemasByFormat(r.Context(), host, "mermaid")
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get diagram")
		return
	}

	if len(schemas) == 0 {
		s.respondError(w, http.StatusNotFound, "NOT_FOUND", "No diagram available. Run analysis first.")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"host":    host,
		"diagram": schemas[0].Content,
	})
}