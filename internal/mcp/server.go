package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"prism/internal/storage"
	"prism/pkg/models"
)

// DBStore defines the storage interface used by MCP handlers.
// This allows for easy mocking in tests.
type DBStore interface {
	ListRequests(ctx context.Context, filter *storage.RequestFilter) ([]*models.CaptureListItem, int64, error)
	GetRequest(ctx context.Context, uuid string) (*models.HTTPRequest, error)
	GetResponse(ctx context.Context, requestUUID string) (*models.HTTPResponse, error)
	GetCapture(ctx context.Context, uuid string) (*models.Capture, error)
	GetWebSocketMessages(ctx context.Context, requestUUID string, filter *storage.WSFilter) ([]*models.WebSocketMessage, int64, error)
	GetSchema(ctx context.Context, host, method, pathPattern string, format models.SchemaFormat) (*models.InferredSchema, error)
	ListEndpointPatterns(ctx context.Context, host string) ([]*models.EndpointPattern, error)
	ListEndpointUsage(ctx context.Context, host, method, pathPrefix string, limit int) ([]*models.EndpointUsage, error)
}

// Server is the MCP server for Prism
type Server struct {
	db        DBStore
	mcpServer *server.MCPServer
}

// NewServer creates a new MCP server
func NewServer(db *storage.DB) *Server {
	s := &Server{
		db: db,
	}

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"prism",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// Register tools
	s.registerTools(mcpServer)

	s.mcpServer = mcpServer
	return s
}

// registerTools registers all MCP tools
func (s *Server) registerTools(mcpServer *server.MCPServer) {
	// list_captures
	mcpServer.AddTool(mcp.NewTool("list_captures",
		mcp.WithDescription("List captured HTTP requests with optional filtering. Returns request metadata without full bodies for efficiency."),
		mcp.WithString("host",
			mcp.Description("Filter by hostname. Supports wildcards: *.example.com"),
		),
		mcp.WithString("method",
			mcp.Description("Filter by HTTP method (GET, POST, PUT, DELETE, etc.)"),
		),
		mcp.WithString("status",
			mcp.Description("Filter by status code. Examples: '200', '4xx', '500-599'"),
		),
		mcp.WithString("path",
			mcp.Description("Filter by URL path pattern. Supports wildcards: /api/*/users"),
		),
		mcp.WithString("content_type",
			mcp.Description("Filter by content type. Example: 'application/json'"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum results to return (default: 50, max: 500)"),
		),
		mcp.WithNumber("offset",
			mcp.Description("Offset for pagination"),
		),
	), s.handleListCaptures)

	// list_endpoints
	mcpServer.AddTool(mcp.NewTool("list_endpoints",
		mcp.WithDescription("List observed endpoint method/path pairs with traffic counts. Useful for scoping to a subset of the API."),
		mcp.WithString("host",
			mcp.Required(),
			mcp.Description("Target hostname (e.g., 'api.example.com')"),
		),
		mcp.WithString("method",
			mcp.Description("Filter by HTTP method (GET, POST, etc.)"),
		),
		mcp.WithString("path_prefix",
			mcp.Description("Filter by path prefix (e.g., '/v3/users')"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum results to return (default: 100, max: 500)"),
		),
	), s.handleListEndpoints)

	// get_request
	mcpServer.AddTool(mcp.NewTool("get_request",
		mcp.WithDescription("Get complete details of a captured HTTP request including headers and body."),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("Request UUID"),
		),
		mcp.WithBoolean("include_body",
			mcp.Description("Whether to include request body (default: true)"),
		),
	), s.handleGetRequest)

	// get_response
	mcpServer.AddTool(mcp.NewTool("get_response",
		mcp.WithDescription("Get the HTTP response for a captured request."),
		mcp.WithString("request_id",
			mcp.Required(),
			mcp.Description("Request UUID"),
		),
		mcp.WithBoolean("include_body",
			mcp.Description("Whether to include response body (default: true)"),
		),
	), s.handleGetResponse)

	// get_examples
	mcpServer.AddTool(mcp.NewTool("get_examples",
		mcp.WithDescription("Get example request/response pairs for a method/path pattern."),
		mcp.WithString("host",
			mcp.Required(),
			mcp.Description("Target hostname (e.g., 'api.example.com')"),
		),
		mcp.WithString("method",
			mcp.Description("Filter by HTTP method (GET, POST, etc.)"),
		),
		mcp.WithString("path",
			mcp.Description("Path or wildcard pattern (supports '*', e.g., '/v3/users/*')"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum examples to return (default: 3, max: 10)"),
		),
		mcp.WithBoolean("include_body",
			mcp.Description("Whether to include request/response bodies (default: true)"),
		),
		mcp.WithBoolean("include_headers",
			mcp.Description("Whether to include request/response headers (default: false)"),
		),
		mcp.WithNumber("max_body_size",
			mcp.Description("Maximum body size to include in bytes (default: 2000, max: 10000)"),
		),
	), s.handleGetExamples)

	// get_slice
	mcpServer.AddTool(mcp.NewTool("get_slice",
		mcp.WithDescription("Get a scoped slice of the API: endpoints + example request/response pairs for each endpoint."),
		mcp.WithString("host",
			mcp.Required(),
			mcp.Description("Target hostname (e.g., 'api.example.com')"),
		),
		mcp.WithString("method",
			mcp.Description("Filter by HTTP method (GET, POST, etc.)"),
		),
		mcp.WithString("path_prefix",
			mcp.Description("Filter endpoints by path prefix (e.g., '/v3/users')"),
		),
		mcp.WithNumber("limit_endpoints",
			mcp.Description("Maximum endpoints to include (default: 25, max: 100)"),
		),
		mcp.WithNumber("examples_per_endpoint",
			mcp.Description("Examples per endpoint (default: 2, max: 5)"),
		),
		mcp.WithBoolean("include_body",
			mcp.Description("Whether to include request/response bodies (default: true)"),
		),
		mcp.WithBoolean("include_headers",
			mcp.Description("Whether to include request/response headers (default: false)"),
		),
		mcp.WithNumber("max_body_size",
			mcp.Description("Maximum body size to include in bytes (default: 2000, max: 10000)"),
		),
	), s.handleGetSlice)

	// get_websocket_messages
	mcpServer.AddTool(mcp.NewTool("get_websocket_messages",
		mcp.WithDescription("Get WebSocket messages for a connection."),
		mcp.WithString("request_id",
			mcp.Required(),
			mcp.Description("UUID of the WebSocket upgrade request"),
		),
		mcp.WithString("direction",
			mcp.Description("Filter by message direction: 'c2s', 's2c', or 'both' (default: both)"),
		),
		mcp.WithString("message_type",
			mcp.Description("Filter by message type: 'text', 'binary', or 'all' (default: all)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum messages to return (default: 100, max: 1000)"),
		),
	), s.handleGetWebSocketMessages)

	// get_schema
	mcpServer.AddTool(mcp.NewTool("get_schema",
		mcp.WithDescription("Get inferred schema for an API endpoint in the specified format. The schema is learned from captured traffic."),
		mcp.WithString("host",
			mcp.Required(),
			mcp.Description("Target hostname (e.g., 'api.example.com')"),
		),
		mcp.WithString("method",
			mcp.Description("HTTP method. If omitted, returns schemas for all methods."),
		),
		mcp.WithString("path",
			mcp.Description("URL path or pattern. If omitted, returns full API spec for host."),
		),
		mcp.WithString("format",
			mcp.Description("Output format: 'openapi', 'protobuf', 'avro', 'sql', 'json-schema', 'typescript', 'go', 'graphql' (default: openapi)"),
		),
	), s.handleGetSchema)

}

// Run starts the MCP server on stdio
func (s *Server) Run() error {
	return server.ServeStdio(s.mcpServer)
}
