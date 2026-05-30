package mcp

import (
	"context"

	"prism/internal/storage"
	"prism/pkg/models"
)

// MockDB is a mock implementation of DBStore for testing
type MockDB struct {
	// Function fields for configurable behavior per test
	ListRequestsFunc         func(ctx context.Context, filter *storage.RequestFilter) ([]*models.CaptureListItem, int64, error)
	GetRequestFunc           func(ctx context.Context, uuid string) (*models.HTTPRequest, error)
	GetResponseFunc          func(ctx context.Context, requestUUID string) (*models.HTTPResponse, error)
	GetCaptureFunc           func(ctx context.Context, uuid string) (*models.Capture, error)
	GetWebSocketMessagesFunc func(ctx context.Context, requestUUID string, filter *storage.WSFilter) ([]*models.WebSocketMessage, int64, error)
	GetSchemaFunc            func(ctx context.Context, host, method, pathPattern string, format models.SchemaFormat) (*models.InferredSchema, error)
	ListEndpointPatternsFunc func(ctx context.Context, host string) ([]*models.EndpointPattern, error)
	ListEndpointUsageFunc    func(ctx context.Context, host, method, pathPrefix string, limit int) ([]*models.EndpointUsage, error)
}

// Ensure MockDB implements DBStore
var _ DBStore = (*MockDB)(nil)

func (m *MockDB) ListRequests(ctx context.Context, filter *storage.RequestFilter) ([]*models.CaptureListItem, int64, error) {
	if m.ListRequestsFunc != nil {
		return m.ListRequestsFunc(ctx, filter)
	}
	return nil, 0, nil
}

func (m *MockDB) GetRequest(ctx context.Context, uuid string) (*models.HTTPRequest, error) {
	if m.GetRequestFunc != nil {
		return m.GetRequestFunc(ctx, uuid)
	}
	return nil, nil
}

func (m *MockDB) GetResponse(ctx context.Context, requestUUID string) (*models.HTTPResponse, error) {
	if m.GetResponseFunc != nil {
		return m.GetResponseFunc(ctx, requestUUID)
	}
	return nil, nil
}

func (m *MockDB) GetCapture(ctx context.Context, uuid string) (*models.Capture, error) {
	if m.GetCaptureFunc != nil {
		return m.GetCaptureFunc(ctx, uuid)
	}
	return nil, nil
}

func (m *MockDB) GetWebSocketMessages(ctx context.Context, requestUUID string, filter *storage.WSFilter) ([]*models.WebSocketMessage, int64, error) {
	if m.GetWebSocketMessagesFunc != nil {
		return m.GetWebSocketMessagesFunc(ctx, requestUUID, filter)
	}
	return nil, 0, nil
}

func (m *MockDB) GetSchema(ctx context.Context, host, method, pathPattern string, format models.SchemaFormat) (*models.InferredSchema, error) {
	if m.GetSchemaFunc != nil {
		return m.GetSchemaFunc(ctx, host, method, pathPattern, format)
	}
	return nil, nil
}

func (m *MockDB) ListEndpointPatterns(ctx context.Context, host string) ([]*models.EndpointPattern, error) {
	if m.ListEndpointPatternsFunc != nil {
		return m.ListEndpointPatternsFunc(ctx, host)
	}
	return nil, nil
}

func (m *MockDB) ListEndpointUsage(ctx context.Context, host, method, pathPrefix string, limit int) ([]*models.EndpointUsage, error) {
	if m.ListEndpointUsageFunc != nil {
		return m.ListEndpointUsageFunc(ctx, host, method, pathPrefix, limit)
	}
	return nil, nil
}
