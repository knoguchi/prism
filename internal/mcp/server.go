package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"ai-proxy/internal/storage"
)

// Server is the MCP server for AI Proxy
type Server struct {
	db        *storage.DB
	mcpServer *server.MCPServer
}

// NewServer creates a new MCP server
func NewServer(db *storage.DB) *Server {
	s := &Server{
		db: db,
	}

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"ai-proxy",
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

	// search_traffic
	mcpServer.AddTool(mcp.NewTool("search_traffic",
		mcp.WithDescription("Full-text search across captured HTTP traffic including URLs, headers, and bodies."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query. Supports SQLite FTS5 syntax: AND, OR, NOT, quotes for phrases"),
		),
		mcp.WithString("host",
			mcp.Description("Limit search to specific host"),
		),
		mcp.WithString("method",
			mcp.Description("Limit search to specific HTTP method"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum results to return (default: 20, max: 100)"),
		),
	), s.handleSearchTraffic)

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

	// list_schemas
	mcpServer.AddTool(mcp.NewTool("list_schemas",
		mcp.WithDescription("List all inferred schemas organized by host and endpoint."),
		mcp.WithString("host",
			mcp.Description("Filter to specific host"),
		),
	), s.handleListSchemas)

	// get_statistics
	mcpServer.AddTool(mcp.NewTool("get_statistics",
		mcp.WithDescription("Get traffic statistics and summary metrics."),
		mcp.WithString("period",
			mcp.Description("Time period: 'hour', 'day', 'week', 'month', 'all' (default: day)"),
		),
		mcp.WithString("group_by",
			mcp.Description("Group statistics by: 'host', 'method', 'status', 'content_type', 'path'"),
		),
		mcp.WithString("host",
			mcp.Description("Filter to specific host"),
		),
	), s.handleGetStatistics)

	// analyze_auth
	mcpServer.AddTool(mcp.NewTool("analyze_auth",
		mcp.WithDescription("Analyze authentication patterns in captured traffic. Detects Bearer tokens, API keys, Basic auth, cookies, and more."),
		mcp.WithString("host",
			mcp.Description("Analyze auth for specific host"),
		),
		mcp.WithBoolean("include_tokens",
			mcp.Description("Include actual token values (sensitive!). Default: false"),
		),
	), s.handleAnalyzeAuth)
}

// Run starts the MCP server on stdio
func (s *Server) Run() error {
	return server.ServeStdio(s.mcpServer)
}
