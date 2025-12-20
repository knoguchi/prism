package models

import (
	"time"
)

// HTTPRequest represents a captured HTTP request
type HTTPRequest struct {
	ID          int64             `json:"id"`
	UUID        string            `json:"uuid"`
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Host        string            `json:"host"`
	Path        string            `json:"path"`
	QueryString string            `json:"query_string,omitempty"`
	Headers     map[string]string `json:"headers"`
	Body        []byte            `json:"body,omitempty"`
	BodySize    int64             `json:"body_size"`
	ContentType string            `json:"content_type,omitempty"`
	Protocol    string            `json:"protocol"`
	IsHTTPS     bool              `json:"is_https"`
	RemoteAddr  string            `json:"remote_addr,omitempty"`
	CapturedAt  time.Time         `json:"captured_at"`
	Tags        []string          `json:"tags,omitempty"`
	Notes       string            `json:"notes,omitempty"`
}

// HTTPResponse represents a captured HTTP response
type HTTPResponse struct {
	ID          int64             `json:"id"`
	RequestID   int64             `json:"request_id"`
	UUID        string            `json:"uuid"`
	StatusCode  int               `json:"status_code"`
	StatusText  string            `json:"status_text,omitempty"`
	Headers     map[string]string `json:"headers"`
	Body        []byte            `json:"body,omitempty"`
	BodySize    int64             `json:"body_size"`
	ContentType string            `json:"content_type,omitempty"`
	LatencyMs   int64             `json:"latency_ms"`
	CapturedAt  time.Time         `json:"captured_at"`
}

// Capture combines a request and its response
type Capture struct {
	Request  *HTTPRequest  `json:"request"`
	Response *HTTPResponse `json:"response,omitempty"`
}

// CaptureListItem is a summary for list views
type CaptureListItem struct {
	ID           string    `json:"id"`
	Method       string    `json:"method"`
	URL          string    `json:"url"`
	Host         string    `json:"host"`
	Path         string    `json:"path"`
	StatusCode   int       `json:"status_code"`
	ContentType  string    `json:"content_type,omitempty"`
	RequestSize  int64     `json:"request_size"`
	ResponseSize int64     `json:"response_size"`
	LatencyMs    int64     `json:"latency_ms"`
	CapturedAt   time.Time `json:"captured_at"`
}
