package storage

import (
	"context"
	"time"

	"ai-proxy/pkg/models"
)

// Repository defines the interface for all storage operations
type Repository interface {
	RequestRepository
	ResponseRepository
	WebSocketRepository
	SchemaRepository
	StatsRepository
	Close() error
}

// RequestRepository handles HTTP request storage
type RequestRepository interface {
	SaveRequest(ctx context.Context, req *models.HTTPRequest) error
	GetRequest(ctx context.Context, uuid string) (*models.HTTPRequest, error)
	ListRequests(ctx context.Context, filter *RequestFilter) ([]*models.CaptureListItem, int64, error)
	DeleteRequest(ctx context.Context, uuid string) error
	DeleteRequests(ctx context.Context, filter *RequestFilter) (int64, error)
	SearchRequests(ctx context.Context, query string, opts *SearchOptions) ([]*models.CaptureListItem, int64, error)
}

// ResponseRepository handles HTTP response storage
type ResponseRepository interface {
	SaveResponse(ctx context.Context, resp *models.HTTPResponse) error
	GetResponse(ctx context.Context, requestUUID string) (*models.HTTPResponse, error)
}

// WebSocketRepository handles WebSocket message storage
type WebSocketRepository interface {
	SaveWebSocketMessage(ctx context.Context, msg *models.WebSocketMessage) error
	GetWebSocketMessages(ctx context.Context, requestUUID string, filter *WSFilter) ([]*models.WebSocketMessage, int64, error)
	ListWebSocketConnections(ctx context.Context) ([]*models.WebSocketConnection, error)
}

// SchemaRepository handles inferred schema storage
type SchemaRepository interface {
	SaveSchema(ctx context.Context, schema *models.InferredSchema) error
	GetSchema(ctx context.Context, host, method, pathPattern string, format models.SchemaFormat) (*models.InferredSchema, error)
	ListSchemas(ctx context.Context, host string) ([]*models.SchemaListItem, error)
	SaveEndpointPattern(ctx context.Context, pattern *models.EndpointPattern) error
	GetEndpointPattern(ctx context.Context, host, method, pathPattern string) (*models.EndpointPattern, error)
}

// StatsRepository handles statistics queries
type StatsRepository interface {
	GetStats(ctx context.Context, period string) (*models.Stats, error)
	GetTimeline(ctx context.Context, from, to time.Time, interval string) ([]*models.TimelinePoint, error)
}

// RequestFilter contains filter options for listing requests
type RequestFilter struct {
	Host          string
	Hosts         []string // Selected hosts for accumulation (max LimitPerPath per endpoint)
	Method        string
	Status        string // "200", "4xx", "500-599"
	Path          string
	ContentType   string
	From          *time.Time
	To            *time.Time
	Page          int
	Limit         int
	LimitPerPath  int // For selected hosts: max N per host+path (accumulates)
	EphemeralLimit int // For non-selected hosts: recent traffic limit (ephemeral)
	Sort          string
	Order         string
}

// SearchOptions contains options for full-text search
type SearchOptions struct {
	Scope  []string // "url", "headers", "request_body", "response_body"
	Host   string
	Method string
	Limit  int
}

// WSFilter contains filter options for WebSocket messages
type WSFilter struct {
	Direction    string // "c2s", "s2c", "both"
	MessageType  string // "text", "binary", "all"
	FromSequence int
	Limit        int
}

// DefaultRequestFilter returns a filter with default values
func DefaultRequestFilter() *RequestFilter {
	return &RequestFilter{
		Page:  1,
		Limit: 50,
		Sort:  "captured_at",
		Order: "desc",
	}
}

// DefaultWSFilter returns a WebSocket filter with default values
func DefaultWSFilter() *WSFilter {
	return &WSFilter{
		Direction:   "both",
		MessageType: "all",
		Limit:       100,
	}
}
