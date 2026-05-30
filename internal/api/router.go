package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"go.uber.org/zap"

	"prism/internal/ca"
	"prism/internal/storage"
	"prism/internal/validation"
	"prism/pkg/models"
)

// Server is the REST API server
type Server struct {
	router    *chi.Mux
	db        *storage.DB
	caManager *ca.Manager
	logger    *zap.Logger
	config    *models.Config
	server    *http.Server
	inference *InferenceManager
	validator *validation.Validator
	staticFS  fs.FS
}

// NewServer creates a new API server
// staticFS is the embedded web UI filesystem (can be nil for API-only mode)
func NewServer(db *storage.DB, caManager *ca.Manager, logger *zap.Logger, cfg *models.Config, staticFS fs.FS) *Server {
	s := &Server{
		router:    chi.NewRouter(),
		db:        db,
		caManager: caManager,
		logger:    logger,
		config:    cfg,
		validator: validation.NewValidator(db, logger),
		staticFS:  staticFS,
	}

	// Initialize inference manager if LLM is configured
	if cfg.LLM.APIKey != "" {
		inference, err := NewInferenceManager(db, &cfg.LLM, logger)
		if err != nil {
			logger.Warn("Failed to initialize LLM provider", zap.Error(err))
		} else {
			s.inference = inference
			logger.Info("LLM provider initialized", zap.String("provider", cfg.LLM.Provider))
		}
	} else {
		logger.Warn("LLM not configured - set ANTHROPIC_API_KEY or OPENAI_API_KEY environment variable")
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

// setupMiddleware configures middleware
func (s *Server) setupMiddleware() {
	// CORS
	s.router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.config.API.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Request logging
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Timeout(180 * time.Second)) // 3 min for LLM requests
}

// setupRoutes configures API routes
func (s *Server) setupRoutes() {
	r := s.router

	// Health check
	r.Get("/health", s.handleHealth)

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Captures
		r.Get("/captures", s.handleListCaptures)
		r.Get("/captures/{id}", s.handleGetCapture)
		r.Delete("/captures/{id}", s.handleDeleteCapture)
		r.Delete("/captures", s.handleClearCaptures)

		// Search
		r.Get("/search", s.handleSearch)

		// WebSocket
		r.Get("/websockets", s.handleListWebSockets)
		r.Get("/websockets/{id}/messages", s.handleGetWebSocketMessages)

		// Schemas
		r.Get("/schemas", s.handleListSchemas)
		r.Get("/schemas/{host}", s.handleGetHostSchema)
		r.Get("/schemas/{host}/format/{format}", s.handleGetSchemaByFormat)

		// Schema versioning and AI fix
		r.Get("/schemas/{host}/versions", s.handleListSchemaVersions)
		r.Post("/schemas/{host}/fix", s.handleRequestSchemaFix)
		r.Post("/schemas/{host}/preview-fix", s.handlePreviewSchemaFix)
		r.Post("/schemas/{host}/versions/{version}/activate", s.handleActivateSchemaVersion)

		// Statistics
		r.Get("/stats", s.handleGetStats)
		r.Get("/stats/timeline", s.handleGetTimeline)

		// Configuration
		r.Get("/config", s.handleGetConfig)
		r.Get("/config/ca", s.handleGetCAInfo)
		r.Get("/config/ca/download", s.handleDownloadCA)

		// Real-time events (SSE)
		r.Get("/events/traffic", s.handleTrafficEvents)
		r.Get("/events/validation/{host}", s.handleValidationEvents)

		// Hosts listing (lightweight - for sidebar)
		r.Get("/hosts", s.handleListHosts)

		// Enabled hosts for filtering
		r.Get("/hosts/enabled", s.handleGetEnabledHosts)
		r.Put("/hosts/enabled", s.handleSetEnabledHosts)
		r.Post("/hosts/enabled/{host}", s.handleAddEnabledHost)
		r.Delete("/hosts/enabled/{host}", s.handleRemoveEnabledHost)

		// AI Inference
		r.Post("/inference/{host}", s.handleAnalyzeHost)
		r.Get("/inference/{host}/status", s.handleGetInferenceStatus)
		r.Get("/inference/{host}/diagram", s.handleGetDiagram)

		// Validation
		r.Get("/validation/{host}", s.handleValidateHost)
		r.Get("/validation/{host}/summary", s.handleValidationSummary)
		r.Get("/validation/request/{id}", s.handleValidateRequest)
	})

	// Serve embedded static files (Web UI) - must be last
	r.Handle("/*", StaticHandler(s.staticFS))
}

// Response helpers
func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) respondError(w http.ResponseWriter, status int, code, message string) {
	s.respondJSON(w, status, map[string]interface{}{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

// Handler implementations
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListCaptures(w http.ResponseWriter, r *http.Request) {
	filter := &storage.RequestFilter{
		Host:        r.URL.Query().Get("host"),
		Hosts:       r.URL.Query()["hosts[]"], // Multiple hosts support
		Method:      r.URL.Query().Get("method"),
		Status:      r.URL.Query().Get("status"),
		Path:        r.URL.Query().Get("path"),
		ContentType: r.URL.Query().Get("content_type"),
		Sort:        r.URL.Query().Get("sort"),
		Order:       r.URL.Query().Get("order"),
	}

	// Parse pagination
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	filter.Page = page

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 500 {
		limit = 50
	}
	filter.Limit = limit

	// Limit per path (for per-endpoint sampling)
	limitPerPath, _ := strconv.Atoi(r.URL.Query().Get("limit_per_path"))
	if limitPerPath > 0 && limitPerPath <= 100 {
		filter.LimitPerPath = limitPerPath
	}

	// Ephemeral limit (for non-selected hosts)
	ephemeralLimit, _ := strconv.Atoi(r.URL.Query().Get("ephemeral_limit"))
	if ephemeralLimit > 0 && ephemeralLimit <= 500 {
		filter.EphemeralLimit = ephemeralLimit
	}

	// Parse time filters
	if from := r.URL.Query().Get("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			filter.From = &t
		}
	}
	if to := r.URL.Query().Get("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			filter.To = &t
		}
	}

	items, total, err := s.db.ListRequests(r.Context(), filter)
	if err != nil {
		s.logger.Error("Failed to list captures", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list captures")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"data": items,
		"pagination": map[string]interface{}{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

func (s *Server) handleGetCapture(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	capture, err := s.db.GetCapture(r.Context(), id)
	if err != nil {
		s.logger.Error("Failed to get capture", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get capture")
		return
	}

	if capture == nil {
		s.respondError(w, http.StatusNotFound, "NOT_FOUND", "Capture not found")
		return
	}

	// Convert to API response (handles []byte to string conversion)
	s.respondJSON(w, http.StatusOK, NewCaptureResponse(capture))
}

func (s *Server) handleDeleteCapture(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := s.db.DeleteRequest(r.Context(), id); err != nil {
		s.logger.Error("Failed to delete capture", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete capture")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleClearCaptures(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	filter := &storage.RequestFilter{
		Host:  host,
		Limit: 10000,
	}

	deleted, err := s.db.DeleteRequests(r.Context(), filter)
	if err != nil {
		s.logger.Error("Failed to clear captures", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to clear captures")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"deleted": deleted,
	})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Query parameter 'q' is required")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 50
	}

	opts := &storage.SearchOptions{
		Host:   r.URL.Query().Get("host"),
		Method: r.URL.Query().Get("method"),
		Limit:  limit,
	}

	results, total, err := s.db.SearchRequests(r.Context(), query, opts)
	if err != nil {
		s.logger.Error("Failed to search", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to search")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"query":   query,
		"results": results,
		"total":   total,
	})
}

func (s *Server) handleListWebSockets(w http.ResponseWriter, r *http.Request) {
	connections, err := s.db.ListWebSocketConnections(r.Context())
	if err != nil {
		s.logger.Error("Failed to list WebSocket connections", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list connections")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"data": connections,
	})
}

func (s *Server) handleGetWebSocketMessages(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	filter := &storage.WSFilter{
		Direction:   r.URL.Query().Get("direction"),
		MessageType: r.URL.Query().Get("type"),
	}

	if from := r.URL.Query().Get("from_seq"); from != "" {
		filter.FromSequence, _ = strconv.Atoi(from)
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	filter.Limit = limit

	messages, total, err := s.db.GetWebSocketMessages(r.Context(), id, filter)
	if err != nil {
		s.logger.Error("Failed to get WebSocket messages", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get messages")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"messages": messages,
		"total":    total,
	})
}

func (s *Server) handleListSchemas(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")

	schemas, err := s.db.ListSchemas(r.Context(), host)
	if err != nil {
		s.logger.Error("Failed to list schemas", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list schemas")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"schemas": schemas,
	})
}

func (s *Server) handleGetHostSchema(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")

	// For now, return the endpoint patterns
	patterns, err := s.db.ListEndpointPatterns(r.Context(), host)
	if err != nil {
		s.logger.Error("Failed to get schemas", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get schemas")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"host":      host,
		"endpoints": patterns,
	})
}

func (s *Server) handleGetSchemaByFormat(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	format := chi.URLParam(r, "format")

	schemas, err := s.db.GetSchemasByFormat(r.Context(), host, format)
	if err != nil {
		s.logger.Error("Failed to get schema", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get schema")
		return
	}

	if len(schemas) == 0 {
		s.respondError(w, http.StatusNotFound, "NOT_FOUND", "No schema found for this format")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"host":    host,
		"format":  format,
		"content": schemas[0].Content,
	})
}

func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "day"
	}

	stats, err := s.db.GetStats(r.Context(), period)
	if err != nil {
		s.logger.Error("Failed to get stats", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get stats")
		return
	}

	s.respondJSON(w, http.StatusOK, stats)
}

func (s *Server) handleGetTimeline(w http.ResponseWriter, r *http.Request) {
	interval := r.URL.Query().Get("interval")
	if interval == "" {
		interval = "hour"
	}

	// Default time range: last 24 hours
	to := time.Now()
	from := to.Add(-24 * time.Hour)

	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = t
		}
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = t
		}
	}

	timeline, err := s.db.GetTimeline(r.Context(), from, to, interval)
	if err != nil {
		s.logger.Error("Failed to get timeline", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get timeline")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"timeline": timeline,
		"from":     from.Format(time.RFC3339),
		"to":       to.Format(time.RFC3339),
		"interval": interval,
	})
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	// Return non-sensitive config
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"proxy": map[string]interface{}{
			"listen": s.config.Proxy.Listen,
		},
		"api": map[string]interface{}{
			"listen": s.config.API.Listen,
		},
		"inference": s.config.Inference,
		"llm": map[string]interface{}{
			"provider":      s.config.LLM.Provider,
			"model":         s.config.LLM.Model,
			"has_api_key":   s.config.LLM.APIKey != "",
			"api_key_len":   len(s.config.LLM.APIKey),
			"llm_ready":     s.inference != nil,
		},
	})
}

func (s *Server) handleGetCAInfo(w http.ResponseWriter, r *http.Request) {
	cert := s.caManager.CACert()
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"subject":    cert.Subject.CommonName,
		"issuer":     cert.Issuer.CommonName,
		"not_before": cert.NotBefore.Format(time.RFC3339),
		"not_after":  cert.NotAfter.Format(time.RFC3339),
	})
}

func (s *Server) handleDownloadCA(w http.ResponseWriter, r *http.Request) {
	pem := s.caManager.CACertPEM()

	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", "attachment; filename=\"prism-ca.crt\"")
	w.Write(pem)
}

// handleTrafficEvents serves Server-Sent Events for real-time traffic updates
func (s *Server) handleTrafficEvents(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Send initial connection message
	w.Write([]byte("event: connected\ndata: {\"status\":\"connected\"}\n\n"))
	flusher.Flush()

	// Poll for new captures every second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastCheck := time.Now()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// Get new captures since last check
			filter := &storage.RequestFilter{
				From:  &lastCheck,
				Limit: 20,
				Sort:  "captured_at",
				Order: "asc",
			}
			lastCheck = time.Now()

			captures, _, err := s.db.ListRequests(r.Context(), filter)
			if err != nil {
				s.logger.Error("Failed to get new captures", zap.Error(err))
				continue
			}

			for _, capture := range captures {
				data, _ := json.Marshal(map[string]interface{}{
					"type": "capture",
					"data": capture,
				})
				w.Write([]byte("data: " + string(data) + "\n\n"))
			}

			if len(captures) > 0 {
				flusher.Flush()
			}
		}
	}
}

// handleValidationEvents serves SSE for real-time validation of traffic
func (s *Server) handleValidationEvents(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	if host == "" {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Host is required")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Try to load the OpenAPI spec for this host
	ctx := r.Context()
	specLoaded := false
	if err := s.validator.LoadSpec(ctx, host); err != nil {
		// Send warning but continue - will mark requests as "unmatched"
		s.logger.Warn("No OpenAPI spec available for validation", zap.String("host", host), zap.Error(err))
		data, _ := json.Marshal(map[string]interface{}{
			"type":    "warning",
			"message": "No OpenAPI spec available. Run 'Analyze' first. Requests will show as unmatched.",
		})
		w.Write([]byte("event: warning\ndata: " + string(data) + "\n\n"))
	} else {
		specLoaded = true
		data, _ := json.Marshal(map[string]interface{}{
			"type": "connected",
			"host": host,
		})
		w.Write([]byte("event: connected\ndata: " + string(data) + "\n\n"))
	}
	flusher.Flush()

	// First, send recent historical traffic (last 50 captures)
	historicalFilter := &storage.RequestFilter{
		Host:  host,
		Limit: 50,
		Sort:  "captured_at",
		Order: "desc", // Most recent first
	}
	historicalRequests, _, err := s.db.ListRequests(r.Context(), historicalFilter)
	if err == nil && len(historicalRequests) > 0 {
		// Reverse to send oldest first
		for i := len(historicalRequests) - 1; i >= 0; i-- {
			req := historicalRequests[i]
			capture, err := s.db.GetCapture(r.Context(), req.ID)
			if err != nil || capture == nil {
				continue
			}

			var status string
			var validationResult *validation.ValidationResult

			if specLoaded {
				validationResult, err = s.validator.ValidateRequest(r.Context(), capture)
				if err != nil {
					status = "unmatched"
				} else if validationResult.Valid {
					status = "valid"
				} else {
					isUnmatched := false
					for _, e := range validationResult.Errors {
						if e.Location == "path" || e.Location == "spec" {
							isUnmatched = true
							break
						}
					}
					if isUnmatched {
						status = "unmatched"
					} else {
						status = "invalid"
					}
				}
			} else {
				status = "unmatched"
			}

			eventData := map[string]interface{}{
				"request_id":  req.ID,
				"method":      req.Method,
				"path":        req.Path,
				"status_code": req.StatusCode,
				"status":      status,
				"timestamp":   req.CapturedAt,
				"historical":  true,
			}

			if validationResult != nil {
				eventData["matched_path"] = validationResult.MatchedPath
				if len(validationResult.Errors) > 0 {
					eventData["errors"] = validationResult.Errors
				}
			}

			data, _ := json.Marshal(eventData)
			w.Write([]byte("event: validation\ndata: " + string(data) + "\n\n"))
		}
		flusher.Flush()
	}

	// Poll for new captures for this host
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	lastCheck := time.Now()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// Get new captures for this host since last check
			filter := &storage.RequestFilter{
				Host:  host,
				From:  &lastCheck,
				Limit: 20,
				Sort:  "captured_at",
				Order: "asc",
			}
			lastCheck = time.Now()

			requests, _, err := s.db.ListRequests(r.Context(), filter)
			if err != nil {
				s.logger.Error("Failed to get new captures", zap.Error(err))
				continue
			}

			for _, req := range requests {
				// Get full capture for validation
				capture, err := s.db.GetCapture(r.Context(), req.ID)
				if err != nil || capture == nil {
					continue
				}

				// Determine validation status
				var status string // "valid", "invalid", "unmatched"
				var validationResult *validation.ValidationResult

				if specLoaded {
					validationResult, err = s.validator.ValidateRequest(r.Context(), capture)
					if err != nil {
						status = "unmatched"
					} else if validationResult.Valid {
						status = "valid"
					} else {
						// Check if it's unmatched (no route found) vs invalid (route found but validation failed)
						isUnmatched := false
						for _, e := range validationResult.Errors {
							if e.Location == "path" || e.Location == "spec" {
								isUnmatched = true
								break
							}
						}
						if isUnmatched {
							status = "unmatched"
						} else {
							status = "invalid"
						}
					}
				} else {
					status = "unmatched"
				}

				// Build event data
				eventData := map[string]interface{}{
					"request_id":  req.ID,
					"method":      req.Method,
					"path":        req.Path,
					"status_code": req.StatusCode,
					"status":      status, // valid, invalid, unmatched
					"timestamp":   req.CapturedAt,
				}

				if validationResult != nil {
					eventData["matched_path"] = validationResult.MatchedPath
					if len(validationResult.Errors) > 0 {
						eventData["errors"] = validationResult.Errors
					}
				}

				data, _ := json.Marshal(map[string]interface{}{
					"type": "validation",
					"data": eventData,
				})
				w.Write([]byte("data: " + string(data) + "\n\n"))
			}

			if len(requests) > 0 {
				flusher.Flush()
			}
		}
	}
}

// handleListHosts returns a lightweight list of unique hosts (for sidebar)
func (s *Server) handleListHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.db.GetDistinctHosts(r.Context())
	if err != nil {
		s.logger.Error("Failed to list hosts", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list hosts")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"hosts": hosts,
	})
}

// Enabled hosts handlers
func (s *Server) handleGetEnabledHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.db.GetEnabledHosts(r.Context())
	if err != nil {
		s.logger.Error("Failed to get enabled hosts", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get enabled hosts")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"hosts": hosts,
	})
}

func (s *Server) handleSetEnabledHosts(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Hosts []string `json:"hosts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON")
		return
	}

	if err := s.db.SetEnabledHosts(r.Context(), req.Hosts); err != nil {
		s.logger.Error("Failed to set enabled hosts", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to set enabled hosts")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"hosts": req.Hosts,
	})
}

func (s *Server) handleAddEnabledHost(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	if host == "" {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Host is required")
		return
	}

	if err := s.db.AddEnabledHost(r.Context(), host); err != nil {
		s.logger.Error("Failed to add enabled host", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to add enabled host")
		return
	}

	hosts, _ := s.db.GetEnabledHosts(r.Context())
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"hosts": hosts,
	})
}

func (s *Server) handleRemoveEnabledHost(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	if host == "" {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Host is required")
		return
	}

	if err := s.db.RemoveEnabledHost(r.Context(), host); err != nil {
		s.logger.Error("Failed to remove enabled host", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to remove enabled host")
		return
	}

	hosts, _ := s.db.GetEnabledHosts(r.Context())
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"hosts": hosts,
	})
}

// ListenAndServe starts the API server
func (s *Server) ListenAndServe(addr string) error {
	s.server = &http.Server{
		Addr:    addr,
		Handler: s.router,
	}
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// Validation handlers

// handleValidateHost validates all requests for a host against its OpenAPI spec
func (s *Server) handleValidateHost(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	if host == "" {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Host is required")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 500 {
		limit = 100
	}

	results, err := s.validator.ValidateHost(r.Context(), host, limit)
	if err != nil {
		s.logger.Error("Validation failed", zap.String("host", host), zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "VALIDATION_ERROR", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"host":    host,
		"results": results,
		"total":   len(results),
	})
}

// handleValidationSummary returns a summary of validation results for a host
func (s *Server) handleValidationSummary(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	if host == "" {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Host is required")
		return
	}

	summary, err := s.validator.GetValidationSummary(r.Context(), host)
	if err != nil {
		s.logger.Error("Failed to get validation summary", zap.String("host", host), zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "VALIDATION_ERROR", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, summary)
}

// handleValidateRequest validates a single request against its host's OpenAPI spec
func (s *Server) handleValidateRequest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Request ID is required")
		return
	}

	capture, err := s.db.GetCapture(r.Context(), id)
	if err != nil {
		s.logger.Error("Failed to get capture", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get capture")
		return
	}

	if capture == nil {
		s.respondError(w, http.StatusNotFound, "NOT_FOUND", "Request not found")
		return
	}

	result, err := s.validator.ValidateRequest(r.Context(), capture)
	if err != nil {
		s.logger.Error("Validation failed", zap.String("id", id), zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "VALIDATION_ERROR", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, result)
}

// Schema versioning and AI fix handlers

// handleListSchemaVersions lists all versions for a host's schema
func (s *Server) handleListSchemaVersions(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	if host == "" {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Host is required")
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "openapi"
	}

	versions, err := s.db.ListSchemaVersions(r.Context(), host, format)
	if err != nil {
		s.logger.Error("Failed to list schema versions", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list versions")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"host":     host,
		"format":   format,
		"versions": versions,
	})
}

// handleRequestSchemaFix uses AI to fix schema based on validation errors
func (s *Server) handleRequestSchemaFix(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	if host == "" {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Host is required")
		return
	}

	if s.inference == nil {
		s.respondError(w, http.StatusServiceUnavailable, "LLM_NOT_CONFIGURED", "LLM is not configured")
		return
	}

	var req struct {
		ValidationErrors []string `json:"validation_errors"`
		SampleData       string   `json:"sample_data,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON")
		return
	}

	if len(req.ValidationErrors) == 0 {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Validation errors are required")
		return
	}

	// Get current OpenAPI schema
	schemas, err := s.db.GetSchemasByFormat(r.Context(), host, "openapi")
	if err != nil || len(schemas) == 0 {
		s.respondError(w, http.StatusNotFound, "NOT_FOUND", "No OpenAPI schema found for this host")
		return
	}

	currentSchema := schemas[0].Content

	// Call AI to fix the schema
	fixResp, err := s.inference.FixSchema(r.Context(), currentSchema, req.ValidationErrors, req.SampleData)
	if err != nil {
		s.logger.Error("Failed to fix schema", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "FIX_FAILED", err.Error())
		return
	}

	// Save as a new version (not active yet - pending review)
	errorsFixed, _ := json.Marshal(req.ValidationErrors)
	version, err := s.db.SaveSchemaVersion(
		r.Context(),
		host,
		"openapi",
		fixResp.FixedSchema,
		fixResp.Reasoning,
		string(errorsFixed),
		false, // Don't make active - user needs to approve
	)
	if err != nil {
		s.logger.Error("Failed to save schema version", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "SAVE_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"version":      version.Version,
		"fixed_schema": fixResp.FixedSchema,
		"changes":      fixResp.Changes,
		"reasoning":    fixResp.Reasoning,
	})
}

// handlePreviewSchemaFix validates sample requests against a proposed schema version
func (s *Server) handlePreviewSchemaFix(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	if host == "" {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Host is required")
		return
	}

	var req struct {
		Version    int      `json:"version"`
		RequestIDs []string `json:"request_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON")
		return
	}

	// Get the schema version
	version, err := s.db.GetSchemaVersion(r.Context(), host, "openapi", req.Version)
	if err != nil || version == nil {
		s.respondError(w, http.StatusNotFound, "NOT_FOUND", "Schema version not found")
		return
	}

	// Create a temporary validator with the proposed schema
	previewValidator := validation.NewValidatorWithSchema(s.db, s.logger, host, version.Content)

	results := make([]map[string]interface{}, 0)
	validCount := 0
	invalidCount := 0

	for _, requestID := range req.RequestIDs {
		capture, err := s.db.GetCapture(r.Context(), requestID)
		if err != nil || capture == nil {
			continue
		}

		result, err := previewValidator.ValidateRequest(r.Context(), capture)
		if err != nil {
			results = append(results, map[string]interface{}{
				"request_id": requestID,
				"valid":      false,
				"error":      err.Error(),
			})
			invalidCount++
			continue
		}

		if result.Valid {
			validCount++
		} else {
			invalidCount++
		}

		results = append(results, map[string]interface{}{
			"request_id":   requestID,
			"method":       result.Method,
			"path":         result.Path,
			"valid":        result.Valid,
			"matched_path": result.MatchedPath,
			"errors":       result.Errors,
		})
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"version":       req.Version,
		"results":       results,
		"valid_count":   validCount,
		"invalid_count": invalidCount,
	})
}

// handleActivateSchemaVersion activates a schema version (user accepts the fix)
func (s *Server) handleActivateSchemaVersion(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	if host == "" {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Host is required")
		return
	}

	versionStr := chi.URLParam(r, "version")
	version, err := strconv.Atoi(versionStr)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid version number")
		return
	}

	// Activate the version
	if err := s.db.ActivateSchemaVersion(r.Context(), host, "openapi", version); err != nil {
		s.logger.Error("Failed to activate schema version", zap.Error(err))
		s.respondError(w, http.StatusInternalServerError, "ACTIVATION_FAILED", err.Error())
		return
	}

	// Clear the validator cache so it reloads the new schema
	s.validator.ClearCache(host)

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"host":    host,
		"version": version,
		"message": "Schema version activated successfully",
	})
}
