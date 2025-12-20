package inference

import (
	"testing"
)

func TestInferParamNamesFromResponse(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		responseBody string
		wantMatches  int
		wantField    string
	}{
		{
			name:         "username in profile response",
			path:         "/v3/profile/kenji",
			responseBody: `{"data": {"username": "kenji", "id": 123, "bio": "hello"}}`,
			wantMatches:  1,
			wantField:    "username",
		},
		{
			name:         "numeric id match",
			path:         "/users/123",
			responseBody: `{"user_id": 123, "name": "John"}`,
			wantMatches:  1,
			wantField:    "user_id",
		},
		{
			name:         "bucket in nested metadata",
			path:         "/v1/buckets/main/collections/test",
			responseBody: `{"metadata": {"bucket": "main", "id": "test"}}`,
			wantMatches:  2,
			wantField:    "bucket", // first match
		},
		{
			name:         "no match - known literal",
			path:         "/api/users",
			responseBody: `{"users": []}`,
			wantMatches:  0,
			wantField:    "",
		},
		{
			name:         "no match - value not in response",
			path:         "/posts/abc123",
			responseBody: `{"title": "Hello", "id": 999}`,
			wantMatches:  0,
			wantField:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := InferParamNamesFromResponse(tt.path, []byte(tt.responseBody))

			if len(matches) != tt.wantMatches {
				t.Errorf("got %d matches, want %d", len(matches), tt.wantMatches)
			}

			if tt.wantMatches > 0 && len(matches) > 0 {
				if matches[0].FieldName != tt.wantField {
					t.Errorf("got field name %q, want %q", matches[0].FieldName, tt.wantField)
				}
			}
		})
	}
}

func TestRefinePathPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		matches []ParamMatch
		want    string
	}{
		{
			name:    "refine slug to username",
			pattern: "/v3/profile/{slug}",
			matches: []ParamMatch{
				{SegmentIndex: 2, FieldName: "username"},
			},
			want: "/v3/profile/{username}",
		},
		{
			name:    "refine id to user_id",
			pattern: "/users/{id}/posts",
			matches: []ParamMatch{
				{SegmentIndex: 1, FieldName: "user_id"},
			},
			want: "/users/{user_id}/posts",
		},
		{
			name:    "multiple refinements",
			pattern: "/v1/buckets/{id}/collections/{slug}",
			matches: []ParamMatch{
				{SegmentIndex: 2, FieldName: "bucket"},
				{SegmentIndex: 4, FieldName: "collection_id"},
			},
			want: "/v1/buckets/{bucket}/collections/{collection_id}",
		},
		{
			name:    "no matches - unchanged",
			pattern: "/api/users/{id}",
			matches: []ParamMatch{},
			want:    "/api/users/{id}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RefinePathPattern(tt.pattern, tt.matches)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAggregateParamMatches(t *testing.T) {
	allMatches := [][]ParamMatch{
		{
			{SegmentIndex: 2, FieldName: "username"},
		},
		{
			{SegmentIndex: 2, FieldName: "username"},
		},
		{
			{SegmentIndex: 2, FieldName: "name"}, // different name, less common
		},
	}

	result := AggregateParamMatches(allMatches)

	if len(result) != 1 {
		t.Errorf("got %d results, want 1", len(result))
	}

	if result[2] != "username" {
		t.Errorf("got %q for segment 2, want 'username'", result[2])
	}
}
