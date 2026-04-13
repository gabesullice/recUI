package server

import "testing"

// TestTypeRoute_PathEscape verifies that type names containing URL-reserved
// characters (`/`, `?`, `#`, space) are percent-encoded when building URLs.
func TestTypeRoute_PathEscape(t *testing.T) {
	cases := []struct {
		name, typeName, want string
	}{
		{"plain", "Author", "/types/Author"},
		{"slash", "foo/bar", "/types/foo%2Fbar"},
		{"question", "foo?bar", "/types/foo%3Fbar"},
		{"hash", "foo#bar", "/types/foo%23bar"},
		{"space", "foo bar", "/types/foo%20bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := typeRoute{TypeName: tc.typeName}.URL()
			if got != tc.want {
				t.Errorf("URL() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRecordRoute_PathEscape verifies that both the type-name and key segments
// of a record URL are percent-encoded. URL-reserved characters in either
// segment must not leak into the raw path.
func TestRecordRoute_PathEscape(t *testing.T) {
	cases := []struct {
		name, typeName, key, want string
	}{
		{"plain", "Author", "abc123", "/types/Author/abc123"},
		{"key slash", "Author", "foo/bar", "/types/Author/foo%2Fbar"},
		{"key question", "Author", "foo?bar", "/types/Author/foo%3Fbar"},
		{"key hash", "Author", "foo#bar", "/types/Author/foo%23bar"},
		{"type slash", "foo/bar", "key", "/types/foo%2Fbar/key"},
		{"both escaped", "foo#bar", "baz?qux", "/types/foo%23bar/baz%3Fqux"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := recordRoute{TypeName: tc.typeName, Key: tc.key}.URL()
			if got != tc.want {
				t.Errorf("URL() = %q, want %q", got, tc.want)
			}
		})
	}
}
