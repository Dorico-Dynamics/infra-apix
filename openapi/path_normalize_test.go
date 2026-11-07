package openapi

import "testing"

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/api/users", "/api/users"},
		{"/api/users/:id", "/api/users/{id}"},
		{"/api/users/{id}", "/api/users/{id}"},
		{"/api/posts/:postId/comments/:commentId", "/api/posts/{postId}/comments/{commentId}"},
		{"/api/files/*filepath", "/api/files/*filepath"}, // wildcard - keep as is for now
		{"/api/users/{id:[0-9]+}", "/api/users/{id}"},
		{"/api/users/{id:[0-9]{3}}/posts/{slug:[a-z-]+}", "/api/users/{id}/posts/{slug}"},
		{"/health", "/health"},
		{"/cafés/:id", "/cafés/{id}"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizePath(tt.input)
			if result != tt.expected {
				t.Errorf("normalizePath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
