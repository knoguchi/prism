package api

import (
	"encoding/base64"
	"strings"
	"time"
	"unicode/utf8"

	"ai-proxy/pkg/models"
)

// CaptureResponse is the API response for a capture detail
type CaptureResponse struct {
	ID       string            `json:"id"`
	Request  *RequestResponse  `json:"request"`
	Response *ResponseResponse `json:"response,omitempty"`
}

// RequestResponse is the API response for request details
type RequestResponse struct {
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Host        string            `json:"host"`
	Path        string            `json:"path"`
	QueryString string            `json:"query_string,omitempty"`
	Headers     map[string]string `json:"headers"`
	Body        string            `json:"body,omitempty"`
	BodySize    int64             `json:"body_size"`
	ContentType string            `json:"content_type,omitempty"`
	Protocol    string            `json:"protocol"`
	IsHTTPS     bool              `json:"is_https"`
	RemoteAddr  string            `json:"remote_addr,omitempty"`
	CapturedAt  time.Time         `json:"captured_at"`
}

// ResponseResponse is the API response for response details
type ResponseResponse struct {
	StatusCode  int               `json:"status_code"`
	StatusText  string            `json:"status_text,omitempty"`
	Headers     map[string]string `json:"headers"`
	Body        string            `json:"body,omitempty"`
	BodySize    int64             `json:"body_size"`
	ContentType string            `json:"content_type,omitempty"`
	LatencyMs   int64             `json:"latency_ms"`
	CapturedAt  time.Time         `json:"captured_at"`
}

// isTextContent checks if the content type indicates text content
func isTextContent(contentType string) bool {
	ct := strings.ToLower(contentType)
	textTypes := []string{
		"text/",
		"application/json",
		"application/xml",
		"application/javascript",
		"application/x-www-form-urlencoded",
		"application/graphql",
		"application/ld+json",
	}
	for _, t := range textTypes {
		if strings.Contains(ct, t) {
			return true
		}
	}
	return false
}

// bodyToString converts a byte body to string, handling binary data
func bodyToString(body []byte, contentType string) string {
	if len(body) == 0 {
		return ""
	}

	// If it's text content or valid UTF-8, return as string
	if isTextContent(contentType) || utf8.Valid(body) {
		return string(body)
	}

	// For binary content, return base64 with prefix
	return "base64:" + base64.StdEncoding.EncodeToString(body)
}

// NewCaptureResponse converts a models.Capture to an API response
func NewCaptureResponse(capture *models.Capture) *CaptureResponse {
	if capture == nil {
		return nil
	}

	resp := &CaptureResponse{
		ID: capture.Request.UUID,
	}

	if capture.Request != nil {
		resp.Request = &RequestResponse{
			Method:      capture.Request.Method,
			URL:         capture.Request.URL,
			Host:        capture.Request.Host,
			Path:        capture.Request.Path,
			QueryString: capture.Request.QueryString,
			Headers:     capture.Request.Headers,
			Body:        bodyToString(capture.Request.Body, capture.Request.ContentType),
			BodySize:    capture.Request.BodySize,
			ContentType: capture.Request.ContentType,
			Protocol:    capture.Request.Protocol,
			IsHTTPS:     capture.Request.IsHTTPS,
			RemoteAddr:  capture.Request.RemoteAddr,
			CapturedAt:  capture.Request.CapturedAt,
		}
	}

	if capture.Response != nil {
		resp.Response = &ResponseResponse{
			StatusCode:  capture.Response.StatusCode,
			StatusText:  capture.Response.StatusText,
			Headers:     capture.Response.Headers,
			Body:        bodyToString(capture.Response.Body, capture.Response.ContentType),
			BodySize:    capture.Response.BodySize,
			ContentType: capture.Response.ContentType,
			LatencyMs:   capture.Response.LatencyMs,
			CapturedAt:  capture.Response.CapturedAt,
		}
	}

	return resp
}
