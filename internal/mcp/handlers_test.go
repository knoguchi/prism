package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"prism/internal/storage"
	"prism/pkg/models"
)

// makeRequest creates a CallToolRequest with the given arguments
func makeRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

// getResultJSON extracts and parses JSON from a CallToolResult
func getResultJSON(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(textContent.Text), &data); err != nil {
		t.Fatalf("failed to parse JSON result: %v", err)
	}
	return data
}

// isErrorResult checks if a result is an error result
func isErrorResult(result *mcp.CallToolResult) bool {
	return result.IsError
}

// getErrorText extracts error text from an error result
func getErrorText(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return ""
	}
	return textContent.Text
}

// TestHandleListCaptures tests the list_captures handler
func TestHandleListCaptures(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]any
		mockSetup   func(*MockDB)
		wantErr     bool
		errContains string
		checkResult func(*testing.T, map[string]any)
	}{
		{
			name: "success_no_filters",
			args: map[string]any{},
			mockSetup: func(m *MockDB) {
				m.ListRequestsFunc = func(ctx context.Context, filter *storage.RequestFilter) ([]*models.CaptureListItem, int64, error) {
					return []*models.CaptureListItem{
						{UUID: "uuid-1", Method: "GET", Host: "api.example.com"},
					}, 1, nil
				}
			},
			checkResult: func(t *testing.T, result map[string]any) {
				captures := result["captures"].([]any)
				if len(captures) != 1 {
					t.Errorf("expected 1 capture, got %d", len(captures))
				}
				if result["total"].(float64) != 1 {
					t.Errorf("expected total=1, got %v", result["total"])
				}
			},
		},
		{
			name: "success_with_host_filter",
			args: map[string]any{"host": "api.example.com"},
			mockSetup: func(m *MockDB) {
				m.ListRequestsFunc = func(ctx context.Context, filter *storage.RequestFilter) ([]*models.CaptureListItem, int64, error) {
					if filter.Host != "api.example.com" {
						t.Errorf("expected host filter 'api.example.com', got '%s'", filter.Host)
					}
					return nil, 0, nil
				}
			},
		},
		{
			name: "success_with_method_filter",
			args: map[string]any{"method": "POST"},
			mockSetup: func(m *MockDB) {
				m.ListRequestsFunc = func(ctx context.Context, filter *storage.RequestFilter) ([]*models.CaptureListItem, int64, error) {
					if filter.Method != "POST" {
						t.Errorf("expected method filter 'POST', got '%s'", filter.Method)
					}
					return nil, 0, nil
				}
			},
		},
		{
			name: "success_with_status_filter",
			args: map[string]any{"status": "4xx"},
			mockSetup: func(m *MockDB) {
				m.ListRequestsFunc = func(ctx context.Context, filter *storage.RequestFilter) ([]*models.CaptureListItem, int64, error) {
					if filter.Status != "4xx" {
						t.Errorf("expected status filter '4xx', got '%s'", filter.Status)
					}
					return nil, 0, nil
				}
			},
		},
		{
			name: "success_with_path_filter",
			args: map[string]any{"path": "/api/*"},
			mockSetup: func(m *MockDB) {
				m.ListRequestsFunc = func(ctx context.Context, filter *storage.RequestFilter) ([]*models.CaptureListItem, int64, error) {
					if filter.Path != "/api/*" {
						t.Errorf("expected path filter '/api/*', got '%s'", filter.Path)
					}
					return nil, 0, nil
				}
			},
		},
		{
			name: "success_with_content_type_filter",
			args: map[string]any{"content_type": "application/json"},
			mockSetup: func(m *MockDB) {
				m.ListRequestsFunc = func(ctx context.Context, filter *storage.RequestFilter) ([]*models.CaptureListItem, int64, error) {
					if filter.ContentType != "application/json" {
						t.Errorf("expected content_type filter 'application/json', got '%s'", filter.ContentType)
					}
					return nil, 0, nil
				}
			},
		},
		{
			name: "limit_capped_at_500",
			args: map[string]any{"limit": float64(1000)},
			mockSetup: func(m *MockDB) {
				m.ListRequestsFunc = func(ctx context.Context, filter *storage.RequestFilter) ([]*models.CaptureListItem, int64, error) {
					if filter.Limit != 500 {
						t.Errorf("expected limit capped at 500, got %d", filter.Limit)
					}
					return nil, 0, nil
				}
			},
		},
		{
			name: "pagination_offset",
			args: map[string]any{"limit": float64(50), "offset": float64(100)},
			mockSetup: func(m *MockDB) {
				m.ListRequestsFunc = func(ctx context.Context, filter *storage.RequestFilter) ([]*models.CaptureListItem, int64, error) {
					// offset 100 with limit 50 = page 3
					if filter.Page != 3 {
						t.Errorf("expected page 3 for offset 100/limit 50, got %d", filter.Page)
					}
					return nil, 0, nil
				}
			},
		},
		{
			name: "database_error",
			args: map[string]any{},
			mockSetup: func(m *MockDB) {
				m.ListRequestsFunc = func(ctx context.Context, filter *storage.RequestFilter) ([]*models.CaptureListItem, int64, error) {
					return nil, 0, errors.New("database connection failed")
				}
			},
			wantErr:     true,
			errContains: "Failed to list captures",
		},
		{
			name: "empty_results",
			args: map[string]any{},
			mockSetup: func(m *MockDB) {
				m.ListRequestsFunc = func(ctx context.Context, filter *storage.RequestFilter) ([]*models.CaptureListItem, int64, error) {
					return []*models.CaptureListItem{}, 0, nil
				}
			},
			checkResult: func(t *testing.T, result map[string]any) {
				captures := result["captures"].([]any)
				if len(captures) != 0 {
					t.Errorf("expected 0 captures, got %d", len(captures))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockDB{}
			if tt.mockSetup != nil {
				tt.mockSetup(mock)
			}

			s := &Server{db: mock}
			result, err := s.handleListCaptures(context.Background(), makeRequest(tt.args))

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantErr {
				if !isErrorResult(result) {
					t.Errorf("expected error result")
				}
				if tt.errContains != "" && !strings.Contains(getErrorText(result), tt.errContains) {
					t.Errorf("error %q should contain %q", getErrorText(result), tt.errContains)
				}
				return
			}

			if isErrorResult(result) {
				t.Fatalf("unexpected error result: %s", getErrorText(result))
			}

			if tt.checkResult != nil {
				tt.checkResult(t, getResultJSON(t, result))
			}
		})
	}
}

// TestHandleGetRequest tests the get_request handler
func TestHandleGetRequest(t *testing.T) {
	sampleRequest := &models.HTTPRequest{
		UUID:        "test-uuid",
		Method:      "POST",
		URL:         "https://api.example.com/users",
		Host:        "api.example.com",
		Path:        "/users",
		QueryString: "page=1",
		Headers:     map[string]string{"Content-Type": "application/json"},
		Body:        []byte(`{"name": "test"}`),
		BodySize:    16,
		ContentType: "application/json",
		Protocol:    "HTTP/1.1",
		IsHTTPS:     true,
		RemoteAddr:  "192.168.1.1",
		CapturedAt:  time.Now(),
	}

	tests := []struct {
		name        string
		args        map[string]any
		mockSetup   func(*MockDB)
		wantErr     bool
		errContains string
		checkResult func(*testing.T, map[string]any)
	}{
		{
			name: "success_with_body",
			args: map[string]any{"id": "test-uuid", "include_body": true},
			mockSetup: func(m *MockDB) {
				m.GetRequestFunc = func(ctx context.Context, uuid string) (*models.HTTPRequest, error) {
					if uuid != "test-uuid" {
						t.Errorf("expected uuid 'test-uuid', got '%s'", uuid)
					}
					return sampleRequest, nil
				}
			},
			checkResult: func(t *testing.T, result map[string]any) {
				if result["id"] != "test-uuid" {
					t.Errorf("expected id 'test-uuid', got %v", result["id"])
				}
				if result["body"] == nil {
					t.Error("expected body to be included")
				}
			},
		},
		{
			name: "success_without_body",
			args: map[string]any{"id": "test-uuid", "include_body": false},
			mockSetup: func(m *MockDB) {
				m.GetRequestFunc = func(ctx context.Context, uuid string) (*models.HTTPRequest, error) {
					return sampleRequest, nil
				}
			},
			checkResult: func(t *testing.T, result map[string]any) {
				if _, ok := result["body"]; ok {
					t.Error("expected body to be excluded")
				}
			},
		},
		{
			name: "success_default_includes_body",
			args: map[string]any{"id": "test-uuid"},
			mockSetup: func(m *MockDB) {
				m.GetRequestFunc = func(ctx context.Context, uuid string) (*models.HTTPRequest, error) {
					return sampleRequest, nil
				}
			},
			checkResult: func(t *testing.T, result map[string]any) {
				if result["body"] == nil {
					t.Error("expected body to be included by default")
				}
			},
		},
		{
			name:        "missing_id_parameter",
			args:        map[string]any{},
			wantErr:     true,
			errContains: "Missing required parameter: id",
		},
		{
			name: "request_not_found",
			args: map[string]any{"id": "nonexistent"},
			mockSetup: func(m *MockDB) {
				m.GetRequestFunc = func(ctx context.Context, uuid string) (*models.HTTPRequest, error) {
					return nil, nil
				}
			},
			wantErr:     true,
			errContains: "Request not found",
		},
		{
			name: "database_error",
			args: map[string]any{"id": "test-uuid"},
			mockSetup: func(m *MockDB) {
				m.GetRequestFunc = func(ctx context.Context, uuid string) (*models.HTTPRequest, error) {
					return nil, errors.New("database error")
				}
			},
			wantErr:     true,
			errContains: "Failed to get request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockDB{}
			if tt.mockSetup != nil {
				tt.mockSetup(mock)
			}

			s := &Server{db: mock}
			result, err := s.handleGetRequest(context.Background(), makeRequest(tt.args))

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantErr {
				if !isErrorResult(result) {
					t.Errorf("expected error result")
				}
				if tt.errContains != "" && !strings.Contains(getErrorText(result), tt.errContains) {
					t.Errorf("error %q should contain %q", getErrorText(result), tt.errContains)
				}
				return
			}

			if isErrorResult(result) {
				t.Fatalf("unexpected error result: %s", getErrorText(result))
			}

			if tt.checkResult != nil {
				tt.checkResult(t, getResultJSON(t, result))
			}
		})
	}
}

// TestHandleGetResponse tests the get_response handler
func TestHandleGetResponse(t *testing.T) {
	sampleResponse := &models.HTTPResponse{
		StatusCode:  200,
		StatusText:  "OK",
		Headers:     map[string]string{"Content-Type": "application/json"},
		Body:        []byte(`{"success": true}`),
		BodySize:    17,
		ContentType: "application/json",
		LatencyMs:   45,
		CapturedAt:  time.Now(),
	}

	tests := []struct {
		name        string
		args        map[string]any
		mockSetup   func(*MockDB)
		wantErr     bool
		errContains string
		checkResult func(*testing.T, map[string]any)
	}{
		{
			name: "success_with_body",
			args: map[string]any{"request_id": "req-uuid", "include_body": true},
			mockSetup: func(m *MockDB) {
				m.GetResponseFunc = func(ctx context.Context, requestUUID string) (*models.HTTPResponse, error) {
					if requestUUID != "req-uuid" {
						t.Errorf("expected requestUUID 'req-uuid', got '%s'", requestUUID)
					}
					return sampleResponse, nil
				}
			},
			checkResult: func(t *testing.T, result map[string]any) {
				if result["status_code"].(float64) != 200 {
					t.Errorf("expected status_code 200, got %v", result["status_code"])
				}
				if result["body"] == nil {
					t.Error("expected body to be included")
				}
			},
		},
		{
			name: "success_without_body",
			args: map[string]any{"request_id": "req-uuid", "include_body": false},
			mockSetup: func(m *MockDB) {
				m.GetResponseFunc = func(ctx context.Context, requestUUID string) (*models.HTTPResponse, error) {
					return sampleResponse, nil
				}
			},
			checkResult: func(t *testing.T, result map[string]any) {
				if _, ok := result["body"]; ok {
					t.Error("expected body to be excluded")
				}
			},
		},
		{
			name:        "missing_request_id",
			args:        map[string]any{},
			wantErr:     true,
			errContains: "Missing required parameter: request_id",
		},
		{
			name: "response_not_found",
			args: map[string]any{"request_id": "nonexistent"},
			mockSetup: func(m *MockDB) {
				m.GetResponseFunc = func(ctx context.Context, requestUUID string) (*models.HTTPResponse, error) {
					return nil, nil
				}
			},
			wantErr:     true,
			errContains: "Response not found",
		},
		{
			name: "database_error",
			args: map[string]any{"request_id": "req-uuid"},
			mockSetup: func(m *MockDB) {
				m.GetResponseFunc = func(ctx context.Context, requestUUID string) (*models.HTTPResponse, error) {
					return nil, errors.New("database error")
				}
			},
			wantErr:     true,
			errContains: "Failed to get response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockDB{}
			if tt.mockSetup != nil {
				tt.mockSetup(mock)
			}

			s := &Server{db: mock}
			result, err := s.handleGetResponse(context.Background(), makeRequest(tt.args))

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantErr {
				if !isErrorResult(result) {
					t.Errorf("expected error result")
				}
				if tt.errContains != "" && !strings.Contains(getErrorText(result), tt.errContains) {
					t.Errorf("error %q should contain %q", getErrorText(result), tt.errContains)
				}
				return
			}

			if isErrorResult(result) {
				t.Fatalf("unexpected error result: %s", getErrorText(result))
			}

			if tt.checkResult != nil {
				tt.checkResult(t, getResultJSON(t, result))
			}
		})
	}
}

// TestHandleGetWebSocketMessages tests the get_websocket_messages handler
func TestHandleGetWebSocketMessages(t *testing.T) {
	textMsg := &models.WebSocketMessage{
		UUID:        "msg-1",
		Direction:   "c2s",
		MessageType: models.WSMessageTypeText,
		Payload:     []byte(`{"action": "subscribe"}`),
		PayloadSize: 24,
		SequenceNum: 1,
		CapturedAt:  time.Now(),
	}

	binaryMsg := &models.WebSocketMessage{
		UUID:        "msg-2",
		Direction:   "s2c",
		MessageType: models.WSMessageTypeBinary,
		Payload:     []byte{0x01, 0x02, 0x03},
		PayloadSize: 3,
		SequenceNum: 2,
		CapturedAt:  time.Now(),
	}

	tests := []struct {
		name        string
		args        map[string]any
		mockSetup   func(*MockDB)
		wantErr     bool
		errContains string
		checkResult func(*testing.T, map[string]any)
	}{
		{
			name: "success_default",
			args: map[string]any{"request_id": "ws-uuid"},
			mockSetup: func(m *MockDB) {
				m.GetWebSocketMessagesFunc = func(ctx context.Context, requestUUID string, filter *storage.WSFilter) ([]*models.WebSocketMessage, int64, error) {
					if requestUUID != "ws-uuid" {
						t.Errorf("expected requestUUID 'ws-uuid', got '%s'", requestUUID)
					}
					if filter.Direction != "both" {
						t.Errorf("expected default direction 'both', got '%s'", filter.Direction)
					}
					if filter.MessageType != "all" {
						t.Errorf("expected default message_type 'all', got '%s'", filter.MessageType)
					}
					return []*models.WebSocketMessage{textMsg}, 1, nil
				}
			},
			checkResult: func(t *testing.T, result map[string]any) {
				messages := result["messages"].([]any)
				if len(messages) != 1 {
					t.Errorf("expected 1 message, got %d", len(messages))
				}
			},
		},
		{
			name: "success_c2s_direction",
			args: map[string]any{"request_id": "ws-uuid", "direction": "c2s"},
			mockSetup: func(m *MockDB) {
				m.GetWebSocketMessagesFunc = func(ctx context.Context, requestUUID string, filter *storage.WSFilter) ([]*models.WebSocketMessage, int64, error) {
					if filter.Direction != "c2s" {
						t.Errorf("expected direction 'c2s', got '%s'", filter.Direction)
					}
					return nil, 0, nil
				}
			},
		},
		{
			name: "success_text_type",
			args: map[string]any{"request_id": "ws-uuid", "message_type": "text"},
			mockSetup: func(m *MockDB) {
				m.GetWebSocketMessagesFunc = func(ctx context.Context, requestUUID string, filter *storage.WSFilter) ([]*models.WebSocketMessage, int64, error) {
					if filter.MessageType != "text" {
						t.Errorf("expected message_type 'text', got '%s'", filter.MessageType)
					}
					return nil, 0, nil
				}
			},
		},
		{
			name: "limit_capped_at_1000",
			args: map[string]any{"request_id": "ws-uuid", "limit": float64(5000)},
			mockSetup: func(m *MockDB) {
				m.GetWebSocketMessagesFunc = func(ctx context.Context, requestUUID string, filter *storage.WSFilter) ([]*models.WebSocketMessage, int64, error) {
					if filter.Limit != 1000 {
						t.Errorf("expected limit capped at 1000, got %d", filter.Limit)
					}
					return nil, 0, nil
				}
			},
		},
		{
			name: "text_payload_as_string",
			args: map[string]any{"request_id": "ws-uuid"},
			mockSetup: func(m *MockDB) {
				m.GetWebSocketMessagesFunc = func(ctx context.Context, requestUUID string, filter *storage.WSFilter) ([]*models.WebSocketMessage, int64, error) {
					return []*models.WebSocketMessage{textMsg}, 1, nil
				}
			},
			checkResult: func(t *testing.T, result map[string]any) {
				messages := result["messages"].([]any)
				msg := messages[0].(map[string]any)
				if msg["payload"] == nil {
					t.Error("expected payload for text message")
				}
			},
		},
		{
			name: "binary_payload_excluded",
			args: map[string]any{"request_id": "ws-uuid"},
			mockSetup: func(m *MockDB) {
				m.GetWebSocketMessagesFunc = func(ctx context.Context, requestUUID string, filter *storage.WSFilter) ([]*models.WebSocketMessage, int64, error) {
					return []*models.WebSocketMessage{binaryMsg}, 1, nil
				}
			},
			checkResult: func(t *testing.T, result map[string]any) {
				messages := result["messages"].([]any)
				msg := messages[0].(map[string]any)
				if _, ok := msg["payload"]; ok {
					t.Error("expected no payload for binary message")
				}
			},
		},
		{
			name:        "missing_request_id",
			args:        map[string]any{},
			wantErr:     true,
			errContains: "Missing required parameter: request_id",
		},
		{
			name: "database_error",
			args: map[string]any{"request_id": "ws-uuid"},
			mockSetup: func(m *MockDB) {
				m.GetWebSocketMessagesFunc = func(ctx context.Context, requestUUID string, filter *storage.WSFilter) ([]*models.WebSocketMessage, int64, error) {
					return nil, 0, errors.New("database error")
				}
			},
			wantErr:     true,
			errContains: "Failed to get messages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockDB{}
			if tt.mockSetup != nil {
				tt.mockSetup(mock)
			}

			s := &Server{db: mock}
			result, err := s.handleGetWebSocketMessages(context.Background(), makeRequest(tt.args))

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantErr {
				if !isErrorResult(result) {
					t.Errorf("expected error result")
				}
				if tt.errContains != "" && !strings.Contains(getErrorText(result), tt.errContains) {
					t.Errorf("error %q should contain %q", getErrorText(result), tt.errContains)
				}
				return
			}

			if isErrorResult(result) {
				t.Fatalf("unexpected error result: %s", getErrorText(result))
			}

			if tt.checkResult != nil {
				tt.checkResult(t, getResultJSON(t, result))
			}
		})
	}
}

// TestHandleListEndpoints tests the list_endpoints handler
func TestHandleListEndpoints(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]any
		mockSetup   func(*MockDB)
		wantErr     bool
		errContains string
		checkResult func(*testing.T, map[string]any)
	}{
		{
			name: "success",
			args: map[string]any{"host": "api.example.com"},
			mockSetup: func(m *MockDB) {
				m.ListEndpointUsageFunc = func(ctx context.Context, host, method, pathPrefix string, limit int) ([]*models.EndpointUsage, error) {
					return []*models.EndpointUsage{
						{Method: "GET", Path: "/v3/users/kenji", SampleCount: 5, LastSeen: time.Now()},
					}, nil
				}
			},
			checkResult: func(t *testing.T, result map[string]any) {
				endpoints := result["endpoints"].([]any)
				if len(endpoints) != 1 {
					t.Fatalf("expected 1 endpoint, got %d", len(endpoints))
				}
			},
		},
		{
			name:        "missing_host",
			args:        map[string]any{},
			wantErr:     true,
			errContains: "Missing required parameter: host",
		},
		{
			name: "db_error",
			args: map[string]any{"host": "api.example.com"},
			mockSetup: func(m *MockDB) {
				m.ListEndpointUsageFunc = func(ctx context.Context, host, method, pathPrefix string, limit int) ([]*models.EndpointUsage, error) {
					return nil, errors.New("database error")
				}
			},
			wantErr:     true,
			errContains: "Failed to list endpoints",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockDB{}
			if tt.mockSetup != nil {
				tt.mockSetup(mock)
			}

			s := &Server{db: mock}
			result, err := s.handleListEndpoints(context.Background(), makeRequest(tt.args))

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantErr {
				if !isErrorResult(result) {
					t.Errorf("expected error result")
				}
				if tt.errContains != "" && !strings.Contains(getErrorText(result), tt.errContains) {
					t.Errorf("error %q should contain %q", getErrorText(result), tt.errContains)
				}
				return
			}

			if isErrorResult(result) {
				t.Fatalf("unexpected error result: %s", getErrorText(result))
			}

			if tt.checkResult != nil {
				tt.checkResult(t, getResultJSON(t, result))
			}
		})
	}
}

// TestHandleGetExamples tests the get_examples handler
func TestHandleGetExamples(t *testing.T) {
	sampleCapture := &models.Capture{
		Request: &models.HTTPRequest{
			UUID:        "req-1",
			Method:      "GET",
			URL:         "https://api.example.com/v3/users/kenji",
			Path:        "/v3/users/kenji",
			QueryString: "",
			Headers:     map[string]string{"X-Test": "true"},
			Body:        []byte("hello world"),
			BodySize:    11,
			ContentType: "application/json",
			CapturedAt:  time.Now(),
		},
		Response: &models.HTTPResponse{
			StatusCode:  200,
			StatusText:  "OK",
			Headers:     map[string]string{"Content-Type": "application/json"},
			Body:        []byte(`{"username":"kenji"}`),
			BodySize:    20,
			ContentType: "application/json",
			LatencyMs:   10,
			CapturedAt:  time.Now(),
		},
	}

	tests := []struct {
		name        string
		args        map[string]any
		mockSetup   func(*MockDB)
		wantErr     bool
		errContains string
		checkResult func(*testing.T, map[string]any)
	}{
		{
			name: "success_truncated",
			args: map[string]any{"host": "api.example.com", "limit": 1, "max_body_size": 5},
			mockSetup: func(m *MockDB) {
				m.ListRequestsFunc = func(ctx context.Context, filter *storage.RequestFilter) ([]*models.CaptureListItem, int64, error) {
					return []*models.CaptureListItem{
						{ID: "req-1"},
					}, 1, nil
				}
				m.GetCaptureFunc = func(ctx context.Context, uuid string) (*models.Capture, error) {
					return sampleCapture, nil
				}
			},
			checkResult: func(t *testing.T, result map[string]any) {
				examples := result["examples"].([]any)
				if len(examples) != 1 {
					t.Fatalf("expected 1 example, got %d", len(examples))
				}
				example := examples[0].(map[string]any)
				req := example["request"].(map[string]any)
				if req["body"].(string) != "hello..." {
					t.Fatalf("expected truncated body, got %v", req["body"])
				}
				if _, ok := req["headers"]; ok {
					t.Fatalf("expected headers to be omitted by default")
				}
			},
		},
		{
			name:        "missing_host",
			args:        map[string]any{},
			wantErr:     true,
			errContains: "Missing required parameter: host",
		},
		{
			name: "list_error",
			args: map[string]any{"host": "api.example.com"},
			mockSetup: func(m *MockDB) {
				m.ListRequestsFunc = func(ctx context.Context, filter *storage.RequestFilter) ([]*models.CaptureListItem, int64, error) {
					return nil, 0, errors.New("database error")
				}
			},
			wantErr:     true,
			errContains: "Failed to list examples",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockDB{}
			if tt.mockSetup != nil {
				tt.mockSetup(mock)
			}

			s := &Server{db: mock}
			result, err := s.handleGetExamples(context.Background(), makeRequest(tt.args))

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantErr {
				if !isErrorResult(result) {
					t.Errorf("expected error result")
				}
				if tt.errContains != "" && !strings.Contains(getErrorText(result), tt.errContains) {
					t.Errorf("error %q should contain %q", getErrorText(result), tt.errContains)
				}
				return
			}

			if isErrorResult(result) {
				t.Fatalf("unexpected error result: %s", getErrorText(result))
			}

			if tt.checkResult != nil {
				tt.checkResult(t, getResultJSON(t, result))
			}
		})
	}
}

// TestHandleGetSlice tests the get_slice handler
func TestHandleGetSlice(t *testing.T) {
	sampleCapture := &models.Capture{
		Request: &models.HTTPRequest{
			UUID:        "req-1",
			Method:      "GET",
			URL:         "https://api.example.com/v3/users/kenji",
			Path:        "/v3/users/kenji",
			QueryString: "",
			Headers:     map[string]string{"X-Test": "true"},
			Body:        []byte("hello world"),
			BodySize:    11,
			ContentType: "application/json",
			CapturedAt:  time.Now(),
		},
		Response: &models.HTTPResponse{
			StatusCode:  200,
			StatusText:  "OK",
			Headers:     map[string]string{"Content-Type": "application/json"},
			Body:        []byte(`{"username":"kenji"}`),
			BodySize:    20,
			ContentType: "application/json",
			LatencyMs:   10,
			CapturedAt:  time.Now(),
		},
	}

	tests := []struct {
		name        string
		args        map[string]any
		mockSetup   func(*MockDB)
		wantErr     bool
		errContains string
		checkResult func(*testing.T, map[string]any)
	}{
		{
			name: "success",
			args: map[string]any{"host": "api.example.com", "limit_endpoints": 1, "examples_per_endpoint": 1},
			mockSetup: func(m *MockDB) {
				m.ListEndpointUsageFunc = func(ctx context.Context, host, method, pathPrefix string, limit int) ([]*models.EndpointUsage, error) {
					return []*models.EndpointUsage{
						{Method: "GET", Path: "/v3/users/kenji", SampleCount: 3, LastSeen: time.Now()},
					}, nil
				}
				m.ListRequestsFunc = func(ctx context.Context, filter *storage.RequestFilter) ([]*models.CaptureListItem, int64, error) {
					return []*models.CaptureListItem{{ID: "req-1"}}, 1, nil
				}
				m.GetCaptureFunc = func(ctx context.Context, uuid string) (*models.Capture, error) {
					return sampleCapture, nil
				}
			},
			checkResult: func(t *testing.T, result map[string]any) {
				endpoints := result["endpoints"].([]any)
				if len(endpoints) != 1 {
					t.Fatalf("expected 1 endpoint, got %d", len(endpoints))
				}
				endpoint := endpoints[0].(map[string]any)
				examples := endpoint["examples"].([]any)
				if len(examples) != 1 {
					t.Fatalf("expected 1 example, got %d", len(examples))
				}
			},
		},
		{
			name:        "missing_host",
			args:        map[string]any{},
			wantErr:     true,
			errContains: "Missing required parameter: host",
		},
		{
			name: "endpoints_error",
			args: map[string]any{"host": "api.example.com"},
			mockSetup: func(m *MockDB) {
				m.ListEndpointUsageFunc = func(ctx context.Context, host, method, pathPrefix string, limit int) ([]*models.EndpointUsage, error) {
					return nil, errors.New("database error")
				}
			},
			wantErr:     true,
			errContains: "Failed to list endpoints",
		},
		{
			name: "examples_error",
			args: map[string]any{"host": "api.example.com"},
			mockSetup: func(m *MockDB) {
				m.ListEndpointUsageFunc = func(ctx context.Context, host, method, pathPrefix string, limit int) ([]*models.EndpointUsage, error) {
					return []*models.EndpointUsage{
						{Method: "GET", Path: "/v3/users/kenji", SampleCount: 3, LastSeen: time.Now()},
					}, nil
				}
				m.ListRequestsFunc = func(ctx context.Context, filter *storage.RequestFilter) ([]*models.CaptureListItem, int64, error) {
					return nil, 0, errors.New("database error")
				}
			},
			wantErr:     true,
			errContains: "Failed to list examples",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockDB{}
			if tt.mockSetup != nil {
				tt.mockSetup(mock)
			}

			s := &Server{db: mock}
			result, err := s.handleGetSlice(context.Background(), makeRequest(tt.args))

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantErr {
				if !isErrorResult(result) {
					t.Errorf("expected error result")
				}
				if tt.errContains != "" && !strings.Contains(getErrorText(result), tt.errContains) {
					t.Errorf("error %q should contain %q", getErrorText(result), tt.errContains)
				}
				return
			}

			if isErrorResult(result) {
				t.Fatalf("unexpected error result: %s", getErrorText(result))
			}

			if tt.checkResult != nil {
				tt.checkResult(t, getResultJSON(t, result))
			}
		})
	}
}

// TestHandleGetSchema tests the get_schema handler
func TestHandleGetSchema(t *testing.T) {
	sampleSchema := &models.InferredSchema{
		Host:        "api.example.com",
		Method:      "GET",
		PathPattern: "/users/{id}",
		Format:      "openapi",
		Content:     `{"openapi": "3.0.0"}`,
		SampleCount: 10,
		UpdatedAt:   time.Now(),
	}

	tests := []struct {
		name        string
		args        map[string]any
		mockSetup   func(*MockDB)
		wantErr     bool
		errContains string
		checkResult func(*testing.T, map[string]any)
	}{
		{
			name: "success_with_schema",
			args: map[string]any{"host": "api.example.com"},
			mockSetup: func(m *MockDB) {
				m.GetSchemaFunc = func(ctx context.Context, host, method, pathPattern string, format models.SchemaFormat) (*models.InferredSchema, error) {
					return sampleSchema, nil
				}
			},
			checkResult: func(t *testing.T, result map[string]any) {
				if result["host"] != "api.example.com" {
					t.Errorf("expected host 'api.example.com', got %v", result["host"])
				}
				if result["schema"] == nil {
					t.Error("expected schema content")
				}
			},
		},
		{
			name: "success_with_format",
			args: map[string]any{"host": "api.example.com", "format": "typescript"},
			mockSetup: func(m *MockDB) {
				m.GetSchemaFunc = func(ctx context.Context, host, method, pathPattern string, format models.SchemaFormat) (*models.InferredSchema, error) {
					if format != "typescript" {
						t.Errorf("expected format 'typescript', got '%s'", format)
					}
					return sampleSchema, nil
				}
			},
		},
		{
			name: "success_with_method_path",
			args: map[string]any{"host": "api.example.com", "method": "GET", "path": "/users"},
			mockSetup: func(m *MockDB) {
				m.GetSchemaFunc = func(ctx context.Context, host, method, pathPattern string, format models.SchemaFormat) (*models.InferredSchema, error) {
					if method != "GET" {
						t.Errorf("expected method 'GET', got '%s'", method)
					}
					if pathPattern != "/users" {
						t.Errorf("expected pathPattern '/users', got '%s'", pathPattern)
					}
					return sampleSchema, nil
				}
			},
		},
		{
			name: "fallback_to_patterns",
			args: map[string]any{"host": "api.example.com"},
			mockSetup: func(m *MockDB) {
				m.GetSchemaFunc = func(ctx context.Context, host, method, pathPattern string, format models.SchemaFormat) (*models.InferredSchema, error) {
					return nil, nil // No schema
				}
				m.ListEndpointPatternsFunc = func(ctx context.Context, host string) ([]*models.EndpointPattern, error) {
					return []*models.EndpointPattern{
						{Host: "api.example.com", Method: "GET", PathPattern: "/users"},
					}, nil
				}
			},
			checkResult: func(t *testing.T, result map[string]any) {
				if result["note"] == nil {
					t.Error("expected note about no pre-generated schema")
				}
				endpoints := result["endpoints"].([]any)
				if len(endpoints) != 1 {
					t.Errorf("expected 1 endpoint pattern, got %d", len(endpoints))
				}
			},
		},
		{
			name: "no_schema_no_patterns",
			args: map[string]any{"host": "api.example.com"},
			mockSetup: func(m *MockDB) {
				m.GetSchemaFunc = func(ctx context.Context, host, method, pathPattern string, format models.SchemaFormat) (*models.InferredSchema, error) {
					return nil, nil
				}
				m.ListEndpointPatternsFunc = func(ctx context.Context, host string) ([]*models.EndpointPattern, error) {
					return nil, nil
				}
			},
			checkResult: func(t *testing.T, result map[string]any) {
				if result["note"] == nil {
					t.Error("expected note")
				}
			},
		},
		{
			name:        "missing_host_parameter",
			args:        map[string]any{},
			wantErr:     true,
			errContains: "Missing required parameter: host",
		},
		{
			name: "get_schema_error",
			args: map[string]any{"host": "api.example.com"},
			mockSetup: func(m *MockDB) {
				m.GetSchemaFunc = func(ctx context.Context, host, method, pathPattern string, format models.SchemaFormat) (*models.InferredSchema, error) {
					return nil, errors.New("database error")
				}
			},
			wantErr:     true,
			errContains: "Failed to get schema",
		},
		{
			name: "list_patterns_error",
			args: map[string]any{"host": "api.example.com"},
			mockSetup: func(m *MockDB) {
				m.GetSchemaFunc = func(ctx context.Context, host, method, pathPattern string, format models.SchemaFormat) (*models.InferredSchema, error) {
					return nil, nil
				}
				m.ListEndpointPatternsFunc = func(ctx context.Context, host string) ([]*models.EndpointPattern, error) {
					return nil, errors.New("database error")
				}
			},
			wantErr:     true,
			errContains: "Failed to get patterns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockDB{}
			if tt.mockSetup != nil {
				tt.mockSetup(mock)
			}

			s := &Server{db: mock}
			result, err := s.handleGetSchema(context.Background(), makeRequest(tt.args))

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantErr {
				if !isErrorResult(result) {
					t.Errorf("expected error result")
				}
				if tt.errContains != "" && !strings.Contains(getErrorText(result), tt.errContains) {
					t.Errorf("error %q should contain %q", getErrorText(result), tt.errContains)
				}
				return
			}

			if isErrorResult(result) {
				t.Fatalf("unexpected error result: %s", getErrorText(result))
			}

			if tt.checkResult != nil {
				tt.checkResult(t, getResultJSON(t, result))
			}
		})
	}
}

// TestJsonResult tests the jsonResult helper function
func TestJsonResult(t *testing.T) {
	tests := []struct {
		name      string
		input     interface{}
		wantError bool
	}{
		{
			name:  "simple_map",
			input: map[string]string{"key": "value"},
		},
		{
			name:  "nil_input",
			input: nil,
		},
		{
			name: "nested_struct",
			input: struct {
				Name  string `json:"name"`
				Count int    `json:"count"`
			}{
				Name:  "test",
				Count: 42,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := jsonResult(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantError {
				if !isErrorResult(result) {
					t.Error("expected error result")
				}
				return
			}

			if isErrorResult(result) {
				t.Errorf("unexpected error result: %s", getErrorText(result))
			}
		})
	}
}
