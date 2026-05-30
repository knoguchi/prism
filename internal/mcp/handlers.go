package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"prism/internal/storage"
	"prism/pkg/models"
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
		// When filtering by specific path, default to smaller limit (10 instead of 50)
		if filter.Limit == 50 {
			filter.Limit = 10
		}
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

// handleListEndpoints handles the list_endpoints tool
func (s *Server) handleListEndpoints(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	host, err := request.RequireString("host")
	if err != nil {
		return mcp.NewToolResultError("Missing required parameter: host"), nil
	}

	method := request.GetString("method", "")
	pathPrefix := request.GetString("path_prefix", "")
	limit := int(request.GetFloat("limit", 0))
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	endpoints, err := s.db.ListEndpointUsage(ctx, host, method, pathPrefix, limit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list endpoints: %v", err)), nil
	}

	return jsonResult(map[string]interface{}{
		"host":        host,
		"method":      method,
		"path_prefix": pathPrefix,
		"limit":       limit,
		"endpoints":   endpoints,
	})
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

// handleGetExamples handles the get_examples tool
func (s *Server) handleGetExamples(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	host, err := request.RequireString("host")
	if err != nil {
		return mcp.NewToolResultError("Missing required parameter: host"), nil
	}

	method := request.GetString("method", "")
	path := request.GetString("path", "")
	limit := int(request.GetFloat("limit", 0))
	if limit <= 0 {
		limit = 3
	}
	if limit > 10 {
		limit = 10
	}

	includeBody := request.GetBool("include_body", true)
	includeHeaders := request.GetBool("include_headers", false)
	maxBodySize := int(request.GetFloat("max_body_size", 0))
	if maxBodySize <= 0 {
		maxBodySize = 2000
	}
	if maxBodySize > 10000 {
		maxBodySize = 10000
	}

	filter := &storage.RequestFilter{
		Host:  host,
		Page:  1,
		Limit: limit,
		Sort:  "captured_at",
		Order: "desc",
	}
	if method != "" {
		filter.Method = method
	}
	if path != "" {
		filter.Path = path
	}

	items, total, err := s.db.ListRequests(ctx, filter)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list examples: %v", err)), nil
	}

	var examples []map[string]interface{}
	for _, item := range items {
		capture, err := s.db.GetCapture(ctx, item.ID)
		if err != nil || capture == nil || capture.Request == nil {
			continue
		}
		examples = append(examples, buildExample(capture, includeBody, includeHeaders, maxBodySize))
	}

	return jsonResult(map[string]interface{}{
		"host":          host,
		"method":        method,
		"path":          path,
		"limit":         limit,
		"total":         total,
		"examples":      examples,
		"max_body_size": maxBodySize,
	})
}

// handleGetSlice handles the get_slice tool
func (s *Server) handleGetSlice(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	host, err := request.RequireString("host")
	if err != nil {
		return mcp.NewToolResultError("Missing required parameter: host"), nil
	}

	method := request.GetString("method", "")
	pathPrefix := request.GetString("path_prefix", "")

	limitEndpoints := int(request.GetFloat("limit_endpoints", 0))
	if limitEndpoints <= 0 {
		limitEndpoints = 25
	}
	if limitEndpoints > 100 {
		limitEndpoints = 100
	}

	examplesPerEndpoint := int(request.GetFloat("examples_per_endpoint", 0))
	if examplesPerEndpoint <= 0 {
		examplesPerEndpoint = 2
	}
	if examplesPerEndpoint > 5 {
		examplesPerEndpoint = 5
	}

	includeBody := request.GetBool("include_body", true)
	includeHeaders := request.GetBool("include_headers", false)
	maxBodySize := int(request.GetFloat("max_body_size", 0))
	if maxBodySize <= 0 {
		maxBodySize = 2000
	}
	if maxBodySize > 10000 {
		maxBodySize = 10000
	}

	endpoints, err := s.db.ListEndpointUsage(ctx, host, method, pathPrefix, limitEndpoints)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list endpoints: %v", err)), nil
	}

	var endpointItems []map[string]interface{}
	for _, endpoint := range endpoints {
		filter := &storage.RequestFilter{
			Host:   host,
			Method: endpoint.Method,
			Path:   endpoint.Path,
			Page:   1,
			Limit:  examplesPerEndpoint,
			Sort:   "captured_at",
			Order:  "desc",
		}

		items, _, err := s.db.ListRequests(ctx, filter)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list examples: %v", err)), nil
		}

		var examples []map[string]interface{}
		for _, item := range items {
			capture, err := s.db.GetCapture(ctx, item.ID)
			if err != nil || capture == nil || capture.Request == nil {
				continue
			}
			examples = append(examples, buildExample(capture, includeBody, includeHeaders, maxBodySize))
		}

		endpointItems = append(endpointItems, map[string]interface{}{
			"method":       endpoint.Method,
			"path":         endpoint.Path,
			"sample_count": endpoint.SampleCount,
			"last_seen":    endpoint.LastSeen,
			"examples":     examples,
		})
	}

	return jsonResult(map[string]interface{}{
		"host":                  host,
		"method":                method,
		"path_prefix":           pathPrefix,
		"limit_endpoints":       limitEndpoints,
		"examples_per_endpoint": examplesPerEndpoint,
		"max_body_size":         maxBodySize,
		"endpoints":             endpointItems,
	})
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

func truncateBody(body []byte, maxSize int) (string, bool) {
	if maxSize <= 0 || len(body) <= maxSize {
		return string(body), false
	}
	return string(body[:maxSize]) + "...", true
}

func buildExample(capture *models.Capture, includeBody, includeHeaders bool, maxBodySize int) map[string]interface{} {
	req := capture.Request
	reqMap := map[string]interface{}{
		"id":           req.UUID,
		"method":       req.Method,
		"url":          req.URL,
		"path":         req.Path,
		"query_string": req.QueryString,
		"content_type": req.ContentType,
		"body_size":    req.BodySize,
		"captured_at":  req.CapturedAt,
	}
	if includeHeaders {
		reqMap["headers"] = req.Headers
	}
	if includeBody && len(req.Body) > 0 {
		body, truncated := truncateBody(req.Body, maxBodySize)
		reqMap["body"] = body
		if truncated {
			reqMap["body_truncated"] = true
		}
	}

	var respMap map[string]interface{}
	if capture.Response != nil {
		resp := capture.Response
		respMap = map[string]interface{}{
			"status_code":  resp.StatusCode,
			"status_text":  resp.StatusText,
			"content_type": resp.ContentType,
			"body_size":    resp.BodySize,
			"latency_ms":   resp.LatencyMs,
			"captured_at":  resp.CapturedAt,
		}
		if includeHeaders {
			respMap["headers"] = resp.Headers
		}
		if includeBody && len(resp.Body) > 0 {
			body, truncated := truncateBody(resp.Body, maxBodySize)
			respMap["body"] = body
			if truncated {
				respMap["body_truncated"] = true
			}
		}
	}

	return map[string]interface{}{
		"request":  reqMap,
		"response": respMap,
	}
}

// jsonResult creates a JSON tool result
func jsonResult(data interface{}) (*mcp.CallToolResult, error) {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(jsonBytes)), nil
}
