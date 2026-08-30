// Package reddit self-check: run with `go test -run=SelfCheck ./internal/reddit`
package reddit

import (
	"testing"
)

func TestSelfCheck(t *testing.T) {
	// Test normalizePostRef
	tests := []struct {
		in   string
		want string
	}{
		{"t3_abc123", "t3_abc123"},
		{"abc123", "t3_abc123"},
		{"https://redd.it/abc123", "t3_abc123"},
		{"https://www.reddit.com/r/x/comments/abc123/slug/", "t3_abc123"},
		{"/r/x/comments/abc123/slug/", "t3_abc123"},
	}
	for _, tc := range tests {
		got, err := normalizePostRef(tc.in)
		if err != nil {
			t.Errorf("normalizePostRef(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("normalizePostRef(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	// Test sanitizeSub
	if SanitizeSub("r/golang") != "golang" {
		t.Errorf("SanitizeSub(r/golang) != golang")
	}
	if SanitizeSub("/r/golang/") != "golang" {
		t.Errorf("SanitizeSub(/r/golang/) != golang")
	}

	// Test Post.IsEdited
	p := &Post{Edited: true}
	if !p.IsEdited() {
		t.Errorf("Post.IsEdited() true")
	}
	p.Edited = float64(1234567890)
	if !p.IsEdited() {
		t.Errorf("Post.IsEdited() float64")
	}
	p.Edited = false
	if p.IsEdited() {
		t.Errorf("Post.IsEdited() false")
	}
}