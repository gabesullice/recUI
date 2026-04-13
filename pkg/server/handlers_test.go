package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gabesullice/recui/pkg/recfile"
)

// newTestServer creates a minimal *srv loaded from the sample recfile.
func newTestServer(t *testing.T) *srv {
	t.Helper()
	types, err := recfile.ParseFile("../../testdata/sample.rec")
	if err != nil {
		t.Fatalf("parse sample.rec: %v", err)
	}
	s := &srv{
		cfg: Config{RecfilePath: "../../testdata/sample.rec"},
	}
	s.types.Store(&types)
	return s
}

func TestHandleHome_ContentType(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	if err := s.handleHome(w, req); err != nil {
		t.Fatalf("handleHome error: %v", err)
	}
	got := w.Header().Get("Content-Type")
	if !strings.Contains(got, "application/vnd.api+json") {
		t.Errorf("Content-Type = %q, want application/vnd.api+json", got)
	}
}

func TestHandleHome_DataType(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	if err := s.handleHome(w, req); err != nil {
		t.Fatalf("handleHome error: %v", err)
	}
	var doc map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var data struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(doc["data"], &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if data.Type != "recfile-index" {
		t.Errorf("data.type = %q, want recfile-index", data.Type)
	}
}

func TestHandleHome_304(t *testing.T) {
	s := newTestServer(t)
	// First request to get the ETag.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	if err := s.handleHome(w, req); err != nil {
		t.Fatal(err)
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag in response")
	}
	// Second request with If-None-Match.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	if err := s.handleHome(w2, req2); err != nil {
		t.Fatal(err)
	}
	if w2.Code != http.StatusNotModified {
		t.Errorf("status = %d, want 304", w2.Code)
	}
}

func TestHandleType_Author(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/types/Author", nil)
	req.SetPathValue("TypeName", "Author")
	w := httptest.NewRecorder()
	if err := s.handleType(w, req); err != nil {
		t.Fatalf("handleType error: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var doc map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var data struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(doc["data"], &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if data.Type != "record-type-collection" {
		t.Errorf("data.type = %q, want record-type-collection", data.Type)
	}
	if data.ID != "Author" {
		t.Errorf("data.id = %q, want Author", data.ID)
	}
}

func TestHandleType_NotFound(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/types/Nonexistent", nil)
	req.SetPathValue("TypeName", "Nonexistent")
	w := httptest.NewRecorder()
	err := s.handleType(w, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// writeError to get the full HTTP response.
	writeError(w, req, err)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/problem+json") {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
}

func TestHandleRecord_FirstAuthor(t *testing.T) {
	s := newTestServer(t)
	const firstAuthorID = "a1b2c3d4-0001-0000-0000-000000000000"
	req := httptest.NewRequest(http.MethodGet, "/types/Author/"+firstAuthorID, nil)
	req.SetPathValue("TypeName", "Author")
	req.SetPathValue("Key", firstAuthorID)
	w := httptest.NewRecorder()
	if err := s.handleRecord(w, req); err != nil {
		t.Fatalf("handleRecord error: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var doc map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var data struct {
		Type       string          `json:"type"`
		ID         string          `json:"id"`
		Attributes json.RawMessage `json:"attributes"`
	}
	if err := json.Unmarshal(doc["data"], &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if data.Type != "record" {
		t.Errorf("data.type = %q, want record", data.Type)
	}
	expectedID := "Author/" + firstAuthorID
	if data.ID != expectedID {
		t.Errorf("data.id = %q, want %q", data.ID, expectedID)
	}
	var attrs map[string]any
	if err := json.Unmarshal(data.Attributes, &attrs); err != nil {
		t.Fatalf("decode attrs: %v", err)
	}
	if name, ok := attrs["Name"]; !ok || name != "Douglas R. Hofstadter" {
		t.Errorf("attrs.Name = %v, want 'Douglas R. Hofstadter'", name)
	}
}

func TestHandleHead_LinkHeaders(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodHead, "/", nil)
	w := httptest.NewRecorder()
	if err := s.handleHead(w, req); err != nil {
		t.Fatalf("handleHead error: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	link := w.Header().Get("Link")
	if link == "" {
		t.Fatal("no Link header")
	}
	if !strings.Contains(link, `rel="item"`) {
		t.Errorf("Link header does not contain rel=\"item\": %q", link)
	}
	// Should contain all three types from sample.rec.
	for _, typeName := range []string{"Author", "Book", "Tag"} {
		if !strings.Contains(link, typeName) {
			t.Errorf("Link header missing type %q: %q", typeName, link)
		}
	}
}
