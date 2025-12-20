package inference

import (
	"testing"
)

func TestDetectPathPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Basic cases
		{"/users", "/users"},
		{"/users/", "/users"},
		{"/", "/"},
		{"", ""},

		// Integer IDs
		{"/users/123", "/users/{id}"},
		{"/users/123/posts", "/users/{id}/posts"},
		{"/users/123/posts/456", "/users/{id}/posts/{id}"},

		// UUIDs
		{"/users/550e8400-e29b-41d4-a716-446655440000", "/users/{uuid}"},
		{"/orders/550E8400-E29B-41D4-A716-446655440000/items", "/orders/{uuid}/items"},

		// Version prefixes (should stay literal)
		{"/v1/users", "/v1/users"},
		{"/v2/users/123", "/v2/users/{id}"},
		{"/api/v3/products", "/api/v3/products"},

		// Slugs with numbers
		{"/posts/hello-world-123", "/posts/{slug}"},
		{"/articles/my-article-2024", "/articles/{slug}"},

		// Known literals (actions, resources)
		{"/users/search", "/users/search"},
		{"/api/login", "/api/login"},
		{"/auth/logout", "/auth/logout"},

		// Dates
		{"/reports/2024-01-15", "/reports/{date}"},

		// Timestamps
		{"/events/1705312800", "/events/{timestamp}"},
		{"/events/1705312800000", "/events/{timestamp}"},

		// Complex paths
		{"/v1/users/123/posts/456/comments", "/v1/users/{id}/posts/{id}/comments"},
		{"/api/v2/orders/550e8400-e29b-41d4-a716-446655440000/items/789", "/api/v2/orders/{uuid}/items/{id}"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := DetectPathPattern(tt.input)
			if result != tt.expected {
				t.Errorf("DetectPathPattern(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMatchPaths(t *testing.T) {
	tests := []struct {
		path1    string
		path2    string
		expected bool
	}{
		{"/users/123", "/users/456", true},
		{"/users/123", "/users/abc", false}, // abc is not an integer
		{"/users/123/posts", "/users/456/posts", true},
		{"/v1/users", "/v2/users", false}, // Different versions
		{"/users/search", "/users/filter", false}, // Different actions
	}

	for _, tt := range tests {
		t.Run(tt.path1+" vs "+tt.path2, func(t *testing.T) {
			result := MatchPaths(tt.path1, tt.path2)
			if result != tt.expected {
				t.Errorf("MatchPaths(%q, %q) = %v, want %v", tt.path1, tt.path2, result, tt.expected)
			}
		})
	}
}

func TestExtractPathParams(t *testing.T) {
	tests := []struct {
		path     string
		pattern  string
		expected map[string]string
	}{
		{
			"/users/123",
			"/users/{id}",
			map[string]string{"id": "123"},
		},
		{
			"/users/550e8400-e29b-41d4-a716-446655440000/posts",
			"/users/{uuid}/posts",
			map[string]string{"uuid": "550e8400-e29b-41d4-a716-446655440000"},
		},
		{
			"/v1/orders/123/items/456",
			"/v1/orders/{id}/items/{id}",
			map[string]string{"id": "456"}, // Last one wins for same name
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := ExtractPathParams(tt.path, tt.pattern)
			if result == nil {
				t.Errorf("ExtractPathParams(%q, %q) returned nil", tt.path, tt.pattern)
				return
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("ExtractPathParams(%q, %q)[%q] = %q, want %q", tt.path, tt.pattern, k, result[k], v)
				}
			}
		})
	}
}

func TestDetectUUIDVersion(t *testing.T) {
	tests := []struct {
		uuid     string
		expected string
	}{
		{"550e8400-e29b-11d4-a716-446655440000", "v1"},
		{"550e8400-e29b-41d4-a716-446655440000", "v4"},
		{"550e8400-e29b-51d4-a716-446655440000", "v5"},
		{"018d5f1c-5e7a-7000-8000-000000000000", "v7"},
		{"invalid-uuid", ""},
	}

	for _, tt := range tests {
		t.Run(tt.uuid, func(t *testing.T) {
			result := DetectUUIDVersion(tt.uuid)
			if result != tt.expected {
				t.Errorf("DetectUUIDVersion(%q) = %q, want %q", tt.uuid, result, tt.expected)
			}
		})
	}
}

func TestClassifySegment(t *testing.T) {
	tests := []struct {
		segment  string
		expected PathSegmentType
	}{
		{"users", SegmentLiteral},
		{"v1", SegmentLiteral},
		{"api", SegmentLiteral},
		{"login", SegmentLiteral},
		{"123", SegmentInteger},
		{"550e8400-e29b-41d4-a716-446655440000", SegmentUUID},
		{"2024-01-15", SegmentDate},
		{"1705312800", SegmentTimestamp},
		{"hello-world-123", SegmentSlug},
	}

	for _, tt := range tests {
		t.Run(tt.segment, func(t *testing.T) {
			result := classifySegment(tt.segment)
			if result != tt.expected {
				t.Errorf("classifySegment(%q) = %v, want %v", tt.segment, result, tt.expected)
			}
		})
	}
}
