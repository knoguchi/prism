package proxy

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"ai-proxy/pkg/models"
)

const (
	// MaxBodySize is the maximum body size to capture (10MB)
	MaxBodySize = 10 * 1024 * 1024
	// MaxBodyCapture is the maximum body size to store in DB (1MB)
	MaxBodyCapture = 1 * 1024 * 1024
)

// captureContext holds data for a single request/response cycle
type captureContext struct {
	ID        string
	StartTime time.Time
	Request   *models.HTTPRequest
}

// captureRequestBody reads and captures the request body
func (s *Server) captureRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}

	// Read body with size limit
	limitedReader := io.LimitReader(req.Body, MaxBodySize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
	}

	// Restore body for forwarding
	req.Body = io.NopCloser(bytes.NewReader(body))

	return body, nil
}

// captureResponseBody reads and captures the response body
func (s *Server) captureResponseBody(resp *http.Response) ([]byte, error) {
	if resp.Body == nil {
		return nil, nil
	}

	// Read body with size limit
	limitedReader := io.LimitReader(resp.Body, MaxBodySize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
	}

	// Restore body for client
	resp.Body = io.NopCloser(bytes.NewReader(body))

	// Try to decompress if needed
	decompressed := s.decompressBody(body, resp.Header.Get("Content-Encoding"))
	if decompressed != nil {
		return decompressed, nil
	}

	return body, nil
}

// decompressBody attempts to decompress the body based on encoding
func (s *Server) decompressBody(body []byte, encoding string) []byte {
	encoding = strings.ToLower(encoding)

	switch encoding {
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil
		}
		defer reader.Close()
		decompressed, err := io.ReadAll(io.LimitReader(reader, MaxBodySize))
		if err != nil {
			return nil
		}
		return decompressed

	case "deflate":
		reader, err := zlib.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil
		}
		defer reader.Close()
		decompressed, err := io.ReadAll(io.LimitReader(reader, MaxBodySize))
		if err != nil {
			return nil
		}
		return decompressed
	}

	return nil
}

// truncateBody truncates body for storage if needed
func truncateBody(body []byte) []byte {
	if len(body) > MaxBodyCapture {
		return body[:MaxBodyCapture]
	}
	return body
}

// buildHTTPRequest creates an HTTPRequest model from an http.Request
func (s *Server) buildHTTPRequest(req *http.Request, id string, body []byte) *models.HTTPRequest {
	// Build headers map (flatten multi-value headers)
	headers := make(map[string]string)
	for key, values := range req.Header {
		if len(values) > 0 {
			headers[key] = strings.Join(values, ", ")
		}
	}

	// Determine if HTTPS
	isHTTPS := req.URL.Scheme == "https" || req.TLS != nil

	// Build full URL
	url := req.URL.String()
	if req.URL.Host == "" {
		scheme := "http"
		if isHTTPS {
			scheme = "https"
		}
		url = scheme + "://" + req.Host + req.URL.RequestURI()
	}

	return &models.HTTPRequest{
		UUID:        id,
		Method:      req.Method,
		URL:         url,
		Host:        req.Host,
		Path:        req.URL.Path,
		QueryString: req.URL.RawQuery,
		Headers:     headers,
		Body:        truncateBody(body),
		BodySize:    int64(len(body)),
		ContentType: req.Header.Get("Content-Type"),
		Protocol:    req.Proto,
		IsHTTPS:     isHTTPS,
		RemoteAddr:  req.RemoteAddr,
		CapturedAt:  time.Now(),
	}
}

// buildHTTPResponse creates an HTTPResponse model from an http.Response
func (s *Server) buildHTTPResponse(resp *http.Response, requestID string, body []byte, latency time.Duration) *models.HTTPResponse {
	// Build headers map
	headers := make(map[string]string)
	for key, values := range resp.Header {
		if len(values) > 0 {
			headers[key] = strings.Join(values, ", ")
		}
	}

	return &models.HTTPResponse{
		UUID:        uuid.New().String(),
		StatusCode:  resp.StatusCode,
		StatusText:  resp.Status,
		Headers:     headers,
		Body:        truncateBody(body),
		BodySize:    int64(len(body)),
		ContentType: resp.Header.Get("Content-Type"),
		LatencyMs:   latency.Milliseconds(),
		CapturedAt:  time.Now(),
	}
}

// isWebSocketUpgrade checks if request is a WebSocket upgrade
func isWebSocketUpgrade(req *http.Request) bool {
	return strings.ToLower(req.Header.Get("Upgrade")) == "websocket" &&
		strings.Contains(strings.ToLower(req.Header.Get("Connection")), "upgrade")
}

// shouldCaptureBody determines if we should capture the body based on content type
func shouldCaptureBody(contentType string) bool {
	// Skip binary content types that are not useful to capture
	skipTypes := []string{
		"image/",
		"video/",
		"audio/",
		"application/octet-stream",
		"application/zip",
		"application/gzip",
		"application/pdf",
	}

	contentType = strings.ToLower(contentType)
	for _, skip := range skipTypes {
		if strings.HasPrefix(contentType, skip) {
			return false
		}
	}

	return true
}

// logRequest logs request details at debug level
func (s *Server) logRequest(req *models.HTTPRequest) {
	s.logger.Debug("Captured request",
		zap.String("id", req.UUID),
		zap.String("method", req.Method),
		zap.String("url", req.URL),
		zap.Int64("body_size", req.BodySize),
	)
}

// logResponse logs response details at debug level
func (s *Server) logResponse(resp *models.HTTPResponse, latency time.Duration) {
	s.logger.Debug("Captured response",
		zap.String("id", resp.UUID),
		zap.Int("status", resp.StatusCode),
		zap.Int64("body_size", resp.BodySize),
		zap.Duration("latency", latency),
	)
}
