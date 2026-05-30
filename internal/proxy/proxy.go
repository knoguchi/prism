package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/elazarl/goproxy"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"prism/internal/ca"
	"prism/internal/inference"
	"prism/internal/storage"
	"prism/pkg/models"
)

// Server is the MITM proxy server
type Server struct {
	proxy     *goproxy.ProxyHttpServer
	db        *storage.DB
	caManager *ca.Manager
	logger    *zap.Logger
	config    *models.Config
	server    *http.Server
	inference *inference.Engine
	stopChan  chan struct{}
}

// NewServer creates a new proxy server
func NewServer(db *storage.DB, caManager *ca.Manager, logger *zap.Logger, cfg *models.Config) *Server {
	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = false

	// Create inference engine
	inferenceEngine := inference.NewEngine(logger, cfg.Inference.MinSamples)

	s := &Server{
		proxy:     proxy,
		db:        db,
		caManager: caManager,
		logger:    logger,
		config:    cfg,
		inference: inferenceEngine,
		stopChan:  make(chan struct{}),
	}

	// Configure HTTPS MITM
	s.setupHTTPS()

	// Setup request/response handlers
	s.setupHandlers()

	return s
}

// setupHTTPS configures HTTPS interception
func (s *Server) setupHTTPS() {
	// Custom certificate generation for MITM
	tlsConfig := func(host string, ctx *goproxy.ProxyCtx) (*tls.Config, error) {
		cert, err := s.caManager.GetCertificate(host)
		if err != nil {
			s.logger.Error("Failed to get certificate", zap.String("host", host), zap.Error(err))
			return nil, err
		}
		return &tls.Config{
			Certificates: []tls.Certificate{*cert},
		}, nil
	}

	// Intercept all HTTPS connections
	goproxy.OkConnect = &goproxy.ConnectAction{
		Action:    goproxy.ConnectMitm,
		TLSConfig: tlsConfig,
	}
	goproxy.MitmConnect = &goproxy.ConnectAction{
		Action:    goproxy.ConnectMitm,
		TLSConfig: tlsConfig,
	}

	s.proxy.OnRequest().HandleConnect(goproxy.AlwaysMitm)
}

// setupHandlers configures request/response capture
func (s *Server) setupHandlers() {
	// Capture all requests
	s.proxy.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		// Serve CA certificate for special paths/hosts
		// Supports: http://localhost:8080/ca.crt, http://prism.proxy/ca.crt, http://prism.proxy/
		if resp := s.serveCAIfRequested(req); resp != nil {
			return req, resp
		}

		// Generate UUID and start timing
		captureID := uuid.New().String()
		startTime := time.Now()

		// Store context for response handler
		ctx.UserData = &captureContext{
			ID:        captureID,
			StartTime: startTime,
		}

		// Check for WebSocket upgrade - handle separately
		if isWebSocketUpgrade(req) {
			s.logger.Debug("WebSocket upgrade detected", zap.String("url", req.URL.String()))
			// Still capture the upgrade request
		}

		// Capture request body
		var body []byte
		var err error
		if shouldCaptureBody(req.Header.Get("Content-Type")) {
			body, err = s.captureRequestBody(req)
			if err != nil {
				s.logger.Warn("Failed to capture request body", zap.Error(err))
			}
		}

		// Build and save request (async to not block proxy)
		httpReq := s.buildHTTPRequest(req, captureID, body)
		go func() {
			if err := s.db.SaveRequest(context.Background(), httpReq); err != nil {
				s.logger.Error("Failed to save request", zap.Error(err))
			} else {
				s.logRequest(httpReq)
			}
		}()

		// Store request in context for response handler
		captureCtx := ctx.UserData.(*captureContext)
		captureCtx.Request = httpReq

		return req, nil
	})

	// Capture all responses
	s.proxy.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		if resp == nil {
			return nil
		}

		captureCtx, ok := ctx.UserData.(*captureContext)
		if !ok || captureCtx == nil {
			return resp
		}

		// Calculate latency
		latency := time.Since(captureCtx.StartTime)

		// Capture response body
		var body []byte
		var err error
		if shouldCaptureBody(resp.Header.Get("Content-Type")) {
			body, err = s.captureResponseBody(resp)
			if err != nil {
				s.logger.Warn("Failed to capture response body", zap.Error(err))
			}
		}

		// Build and save response (async to not block proxy)
		httpResp := s.buildHTTPResponse(resp, captureCtx.ID, body, latency)
		capturedRequest := captureCtx.Request
		go func() {
			if err := s.db.SaveResponseForRequest(context.Background(), captureCtx.ID, httpResp); err != nil {
				s.logger.Error("Failed to save response", zap.Error(err))
			} else {
				s.logResponse(httpResp, latency)
			}

			// Learn from request/response pair for schema inference
			if capturedRequest != nil {
				if err := s.inference.LearnFromRequest(context.Background(), capturedRequest, httpResp); err != nil {
					s.logger.Debug("Schema inference failed", zap.Error(err))
				}
			}
		}()

		return resp
	})
}


// ListenAndServe starts the proxy server
func (s *Server) ListenAndServe(addr string) error {
	s.server = &http.Server{
		Addr:    addr,
		Handler: s.proxy,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	return s.server.Serve(ln)
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	// Stop the pruner
	close(s.stopChan)

	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// StartPruner starts a background goroutine that periodically prunes old requests.
// Enabled hosts keep up to maxPerPathEnabled per (host, method, path).
// Non-enabled hosts keep only the most recent ephemeralLimit records.
//
// FIXME TODO: Wire this up in cmd/proxy/main.go with appropriate config values.
// Example: proxyServer.StartPruner(time.Minute, 100, 1000)
func (s *Server) StartPruner(interval time.Duration, maxPerPathEnabled, ephemeralLimit int) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-s.stopChan:
				return
			case <-ticker.C:
				// Get enabled hosts
				enabledHosts, err := s.db.GetEnabledHosts(context.Background())
				if err != nil {
					s.logger.Warn("Failed to get enabled hosts for pruning", zap.Error(err))
					continue
				}

				// Run pruning
				deleted, err := s.db.PruneRequests(context.Background(), enabledHosts, maxPerPathEnabled, ephemeralLimit)
				if err != nil {
					s.logger.Warn("Failed to prune requests", zap.Error(err))
				} else if deleted > 0 {
					s.logger.Info("Pruned old requests",
						zap.Int64("deleted", deleted),
						zap.Int("enabled_hosts", len(enabledHosts)),
					)
				}
			}
		}
	}()
}

// InferenceEngine returns the schema inference engine
func (s *Server) InferenceEngine() *inference.Engine {
	return s.inference
}

// serveCAIfRequested checks if the request is for the CA certificate and returns it
// Supports multiple access methods:
// - http://localhost:8080/ca.crt (direct to proxy)
// - http://prism.proxy/ca.crt or http://prism.proxy/ (magic hostname)
func (s *Server) serveCAIfRequested(req *http.Request) *http.Response {
	host := req.Host
	path := req.URL.Path

	// Check for magic hostname "prism.proxy"
	if host == "prism.proxy" || host == "prism.proxy:80" {
		return s.buildCAResponse(req)
	}

	// Check for direct request to proxy with /ca.crt path
	// This works when browser sends request directly to proxy (not via CONNECT)
	if path == "/ca.crt" || path == "/prism-ca.crt" {
		// Only serve if it's a direct request (no host or localhost)
		if host == "" || host == "localhost" || host == "localhost:8080" || host == "127.0.0.1" || host == "127.0.0.1:8080" {
			return s.buildCAResponse(req)
		}
	}

	return nil
}

// buildCAResponse creates an HTTP response with the CA certificate
func (s *Server) buildCAResponse(req *http.Request) *http.Response {
	certPEM := s.caManager.CACertPEM()

	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        make(http.Header),
		Body:          io.NopCloser(bytes.NewReader(certPEM)),
		ContentLength: int64(len(certPEM)),
		Request:       req,
	}

	resp.Header.Set("Content-Type", "application/x-pem-file")
	resp.Header.Set("Content-Disposition", "attachment; filename=\"prism-ca.crt\"")
	resp.Header.Set("Cache-Control", "no-cache")

	s.logger.Info("Served CA certificate",
		zap.String("path", req.URL.Path),
		zap.String("host", req.Host),
	)

	return resp
}

// Generator returns a new schema generator using the inference engine
func (s *Server) Generator() *inference.Generator {
	return inference.NewGenerator(s.inference)
}
