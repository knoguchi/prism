package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"ai-proxy/internal/storage"
	"ai-proxy/pkg/models"
)

// handleListCaptures handles the list_captures tool
func (s *Server) handleListCaptures(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filter := &storage.RequestFilter{
		Page:  1,
		Limit: 50,
		Sort:  "captured_at",
		Order: "desc",
	}

	if host := request.GetString("host", ""); host != "" {
		filter.Host = host
	}
	if method := request.GetString("method", ""); method != "" {
		filter.Method = method
	}
	if status := request.GetString("status", ""); status != "" {
		filter.Status = status
	}
	if path := request.GetString("path", ""); path != "" {
		filter.Path = path
	}
	if contentType := request.GetString("content_type", ""); contentType != "" {
		filter.ContentType = contentType
	}
	if limit := request.GetFloat("limit", 0); limit > 0 {
		filter.Limit = int(limit)
		if filter.Limit > 500 {
			filter.Limit = 500
		}
	}
	if offset := request.GetFloat("offset", 0); offset > 0 {
		filter.Page = int(offset/float64(filter.Limit)) + 1
	}

	items, total, err := s.db.ListRequests(ctx, filter)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list captures: %v", err)), nil
	}

	result := map[string]interface{}{
		"captures": items,
		"total":    total,
		"page":     filter.Page,
		"limit":    filter.Limit,
	}

	return jsonResult(result)
}

// handleGetRequest handles the get_request tool
func (s *Server) handleGetRequest(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := request.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("Missing required parameter: id"), nil
	}

	req, err := s.db.GetRequest(ctx, id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get request: %v", err)), nil
	}
	if req == nil {
		return mcp.NewToolResultError("Request not found"), nil
	}

	// Check if body should be included (default true)
	includeBody := request.GetBool("include_body", true)

	result := map[string]interface{}{
		"id":           req.UUID,
		"method":       req.Method,
		"url":          req.URL,
		"host":         req.Host,
		"path":         req.Path,
		"query_string": req.QueryString,
		"headers":      req.Headers,
		"content_type": req.ContentType,
		"body_size":    req.BodySize,
		"protocol":     req.Protocol,
		"is_https":     req.IsHTTPS,
		"remote_addr":  req.RemoteAddr,
		"captured_at":  req.CapturedAt,
	}

	if includeBody && len(req.Body) > 0 {
		result["body"] = string(req.Body)
	}

	return jsonResult(result)
}

// handleGetResponse handles the get_response tool
func (s *Server) handleGetResponse(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	requestID, err := request.RequireString("request_id")
	if err != nil {
		return mcp.NewToolResultError("Missing required parameter: request_id"), nil
	}

	resp, err := s.db.GetResponse(ctx, requestID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get response: %v", err)), nil
	}
	if resp == nil {
		return mcp.NewToolResultError("Response not found"), nil
	}

	// Check if body should be included (default true)
	includeBody := request.GetBool("include_body", true)

	result := map[string]interface{}{
		"status_code":  resp.StatusCode,
		"status_text":  resp.StatusText,
		"headers":      resp.Headers,
		"content_type": resp.ContentType,
		"body_size":    resp.BodySize,
		"latency_ms":   resp.LatencyMs,
		"captured_at":  resp.CapturedAt,
	}

	if includeBody && len(resp.Body) > 0 {
		result["body"] = string(resp.Body)
	}

	return jsonResult(result)
}

// handleSearchTraffic handles the search_traffic tool
func (s *Server) handleSearchTraffic(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("Missing required parameter: query"), nil
	}

	opts := &storage.SearchOptions{
		Limit: 20,
	}

	if host := request.GetString("host", ""); host != "" {
		opts.Host = host
	}
	if method := request.GetString("method", ""); method != "" {
		opts.Method = method
	}
	if limit := request.GetFloat("limit", 0); limit > 0 {
		opts.Limit = int(limit)
		if opts.Limit > 100 {
			opts.Limit = 100
		}
	}

	results, total, err := s.db.SearchRequests(ctx, query, opts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Search failed: %v", err)), nil
	}

	result := map[string]interface{}{
		"query":   query,
		"results": results,
		"total":   total,
	}

	return jsonResult(result)
}

// handleGetWebSocketMessages handles the get_websocket_messages tool
func (s *Server) handleGetWebSocketMessages(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	requestID, err := request.RequireString("request_id")
	if err != nil {
		return mcp.NewToolResultError("Missing required parameter: request_id"), nil
	}

	filter := &storage.WSFilter{
		Direction:   "both",
		MessageType: "all",
		Limit:       100,
	}

	if direction := request.GetString("direction", ""); direction != "" {
		filter.Direction = direction
	}
	if msgType := request.GetString("message_type", ""); msgType != "" {
		filter.MessageType = msgType
	}
	if limit := request.GetFloat("limit", 0); limit > 0 {
		filter.Limit = int(limit)
		if filter.Limit > 1000 {
			filter.Limit = 1000
		}
	}

	messages, total, err := s.db.GetWebSocketMessages(ctx, requestID, filter)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get messages: %v", err)), nil
	}

	// Convert messages to response format
	var msgList []map[string]interface{}
	for _, msg := range messages {
		m := map[string]interface{}{
			"sequence":     msg.SequenceNum,
			"direction":    msg.Direction,
			"type":         msg.MessageType,
			"payload_size": msg.PayloadSize,
			"captured_at":  msg.CapturedAt,
		}
		if msg.MessageType == models.WSMessageTypeText {
			m["payload"] = string(msg.Payload)
		}
		msgList = append(msgList, m)
	}

	result := map[string]interface{}{
		"messages": msgList,
		"total":    total,
	}

	return jsonResult(result)
}

// handleGetSchema handles the get_schema tool
func (s *Server) handleGetSchema(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	host, err := request.RequireString("host")
	if err != nil {
		return mcp.NewToolResultError("Missing required parameter: host"), nil
	}

	format := request.GetString("format", "openapi")
	method := request.GetString("method", "")
	path := request.GetString("path", "")

	// Get schema from database
	schema, err := s.db.GetSchema(ctx, host, method, path, models.SchemaFormat(format))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get schema: %v", err)), nil
	}

	if schema == nil {
		// Return endpoint patterns if no generated schema
		patterns, err := s.db.ListEndpointPatterns(ctx, host)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get patterns: %v", err)), nil
		}

		result := map[string]interface{}{
			"host":      host,
			"format":    format,
			"endpoints": patterns,
			"note":      "No pre-generated schema available. Showing detected endpoint patterns.",
		}
		return jsonResult(result)
	}

	result := map[string]interface{}{
		"host":         schema.Host,
		"method":       schema.Method,
		"path_pattern": schema.PathPattern,
		"format":       schema.Format,
		"schema":       schema.Content,
		"sample_count": schema.SampleCount,
		"updated_at":   schema.UpdatedAt,
	}

	return jsonResult(result)
}

// handleListSchemas handles the list_schemas tool
func (s *Server) handleListSchemas(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	host := request.GetString("host", "")

	schemas, err := s.db.ListSchemas(ctx, host)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list schemas: %v", err)), nil
	}

	return jsonResult(map[string]interface{}{
		"schemas": schemas,
	})
}

// handleGetStatistics handles the get_statistics tool
func (s *Server) handleGetStatistics(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	period := request.GetString("period", "day")

	stats, err := s.db.GetStats(ctx, period)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get statistics: %v", err)), nil
	}

	return jsonResult(stats)
}

// handleAnalyzeAuth handles the analyze_auth tool
func (s *Server) handleAnalyzeAuth(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	host := request.GetString("host", "")
	includeTokens := request.GetBool("include_tokens", false)

	// Get requests with auth headers
	filter := &storage.RequestFilter{
		Host:  host,
		Limit: 1000,
	}

	requests, _, err := s.db.ListRequests(ctx, filter)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to analyze auth: %v", err)), nil
	}

	// Analyze auth patterns
	authSchemes := analyzeAuthPatterns(s.db, requests, includeTokens)

	result := &models.AuthAnalysis{
		Host:        host,
		AuthSchemes: authSchemes,
	}

	return jsonResult(result)
}

// analyzeAuthPatterns detects authentication patterns from requests
func analyzeAuthPatterns(db *storage.DB, requests []*models.CaptureListItem, includeTokens bool) []*models.AuthScheme {
	// Note: Full auth analysis would require reading full request headers
	// For now, return a placeholder indicating analysis is available
	_ = requests
	_ = includeTokens

	var schemes []*models.AuthScheme

	// If no schemes detected, indicate that
	schemes = append(schemes, &models.AuthScheme{
		Type: "none",
		Name: "Auth analysis requires examining request headers. Use get_request to inspect individual requests.",
	})

	return schemes
}

// maskToken masks a token for safe display
func maskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

// jsonResult creates a JSON tool result
func jsonResult(data interface{}) (*mcp.CallToolResult, error) {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(jsonBytes)), nil
}
