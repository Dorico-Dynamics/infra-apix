package pathtemplate

import "testing"

func TestStripPatterns(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/items/{id}", "/items/{id}"},
		{"/items/{id:[0-9]+}", "/items/{id}"},
		{"/items/{id:[0-9]{3}}/parts/{part:[a-z]+}", "/items/{id}/parts/{part}"},
		{"/files/{rest:[a-z/]+}", "/files/{rest}"},
		{"/items/{id:[0-9]+", "/items/{id:[0-9]+"},
		{"/users/:id", "/users/:id"},
		{"/movements:receipt", "/movements:receipt"},
		{"/", "/"},
	}
	for _, tt := range tests {
		if got := StripPatterns(tt.path); got != tt.want {
			t.Errorf("StripPatterns(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
