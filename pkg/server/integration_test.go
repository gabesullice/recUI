package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gabesullice/recui/pkg/config"
	"github.com/gabesullice/recui/pkg/recfile"
)

// newIntegrationServer starts an httptest.Server backed by a srv loaded from
// the sample recfile and returns it. The caller must call ts.Close().
func newIntegrationServer(t *testing.T) *httptest.Server {
	t.Helper()
	types, err := recfile.ParseFile("../../testdata/sample.rec")
	if err != nil {
		t.Fatalf("parse sample.rec: %v", err)
	}
	s := &srv{cfg: Config{RecfilePath: "../../testdata/sample.rec"}}
	s.types.Store(&types)
	// Wrap in requestLogger to exercise the full middleware stack.
	return httptest.NewServer(requestLogger(s.buildMux()))
}

// getJSON issues a GET request to url with Accept: application/vnd.api+json and
// decodes the response body into dest. Returns the *http.Response for header
// inspection.
func getJSON(t *testing.T, client *http.Client, url string, dest any) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Accept", contentTypeJSONAPI)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		t.Fatalf("decode response from %s: %v", url, err)
	}
	return resp
}

// TestIntegration_Home asserts that GET / returns 200 with a recfile-index
// document whose record-types relationship is non-empty.
func TestIntegration_Home(t *testing.T) {
	ts := newIntegrationServer(t)
	defer ts.Close()

	var doc map[string]any
	resp := getJSON(t, ts.Client(), ts.URL+"/", &doc)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/vnd.api+json") {
		t.Errorf("Content-Type = %q, want application/vnd.api+json", ct)
	}

	data, ok := doc["data"].(map[string]any)
	if !ok {
		t.Fatalf("data is not an object: %T", doc["data"])
	}
	if got := data["type"]; got != "recfile-index" {
		t.Errorf("data.type = %q, want recfile-index", got)
	}

	rels, ok := data["relationships"].(map[string]any)
	if !ok {
		t.Fatalf("data.relationships is not an object: %T", data["relationships"])
	}
	recordTypes, ok := rels["record-types"].(map[string]any)
	if !ok {
		t.Fatalf("data.relationships.record-types is not an object: %T", rels["record-types"])
	}
	items, ok := recordTypes["data"].([]any)
	if !ok {
		t.Fatalf("data.relationships.record-types.data is not an array: %T", recordTypes["data"])
	}
	if len(items) == 0 {
		t.Error("data.relationships.record-types.data is empty")
	}
}

// TestIntegration_TypeCollection follows the first record-type linkage from GET /
// and asserts the collection resource is returned correctly.
func TestIntegration_TypeCollection(t *testing.T) {
	ts := newIntegrationServer(t)
	defer ts.Close()

	// Fetch the home document to get the first type's ID.
	var homeDoc map[string]any
	getJSON(t, ts.Client(), ts.URL+"/", &homeDoc)

	data := homeDoc["data"].(map[string]any)
	rels := data["relationships"].(map[string]any)
	recordTypes := rels["record-types"].(map[string]any)
	items := recordTypes["data"].([]any)
	firstLinkage := items[0].(map[string]any)
	firstTypeID := firstLinkage["id"].(string) // e.g. "Author"

	collURL := ts.URL + "/types/" + firstTypeID
	var collDoc map[string]any
	resp := getJSON(t, ts.Client(), collURL, &collDoc)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	collData, ok := collDoc["data"].(map[string]any)
	if !ok {
		t.Fatalf("data is not an object: %T", collDoc["data"])
	}
	if got := collData["type"]; got != "record-type-collection" {
		t.Errorf("data.type = %q, want record-type-collection", got)
	}

	collRels, ok := collData["relationships"].(map[string]any)
	if !ok {
		t.Fatalf("data.relationships is not an object")
	}
	itemsRel, ok := collRels["items"].(map[string]any)
	if !ok {
		t.Fatalf("data.relationships.items is not an object: %T", collRels["items"])
	}
	recordItems, ok := itemsRel["data"].([]any)
	if !ok {
		t.Fatalf("data.relationships.items.data is not an array: %T", itemsRel["data"])
	}
	if len(recordItems) == 0 {
		t.Error("data.relationships.items.data is empty")
	}
}

// TestIntegration_Record follows the first record linkage from the Author
// collection and asserts the record document contains the expected field value.
func TestIntegration_Record(t *testing.T) {
	ts := newIntegrationServer(t)
	defer ts.Close()

	// Fetch the Author collection directly (we know it exists in the fixture).
	var collDoc map[string]any
	getJSON(t, ts.Client(), ts.URL+"/types/Author", &collDoc)

	collData := collDoc["data"].(map[string]any)
	collRels := collData["relationships"].(map[string]any)
	itemsRel := collRels["items"].(map[string]any)
	recordItems := itemsRel["data"].([]any)
	firstLinkage := recordItems[0].(map[string]any)
	firstRecordID := firstLinkage["id"].(string) // e.g. "Author/a1b2c3d4-..."

	// The ID is "Author/<key>"; strip the type prefix to form the URL segment.
	key := strings.TrimPrefix(firstRecordID, "Author/")
	recURL := ts.URL + "/types/Author/" + key

	var recDoc map[string]any
	resp := getJSON(t, ts.Client(), recURL, &recDoc)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	recData, ok := recDoc["data"].(map[string]any)
	if !ok {
		t.Fatalf("data is not an object: %T", recDoc["data"])
	}
	if got := recData["type"]; got != "record" {
		t.Errorf("data.type = %q, want record", got)
	}
	if id, ok := recData["id"].(string); !ok || id == "" {
		t.Errorf("data.id is empty or missing: %v", recData["id"])
	}

	attrs, ok := recData["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("data.attributes is not an object: %T", recData["attributes"])
	}
	if name := attrs["Name"]; name != "Douglas R. Hofstadter" {
		t.Errorf("data.attributes.Name = %q, want Douglas R. Hofstadter", name)
	}
}

// TestIntegration_NotFound asserts that a request to a nonexistent type returns
// 404 with Content-Type: application/problem+json.
func TestIntegration_NotFound(t *testing.T) {
	ts := newIntegrationServer(t)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/types/NoSuchType", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Accept", contentTypeJSONAPI)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /types/NoSuchType: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/problem+json") {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
}

// itemRecfile is a minimal recfile used by the config rendering tests below.
const itemRecfile = `%rec: Item
%key: ID

ID: alpha
Name: First Item
Tags: foo
Tags: bar
Tags: baz
`

// newItemSrv builds a *srv loaded from itemRecfile with the given UIConfig.
func newItemSrv(t *testing.T, uiCfg config.UIConfig) (*srv, recfile.RecordType) {
	t.Helper()
	types, err := recfile.ParseReader(strings.NewReader(itemRecfile))
	if err != nil {
		t.Fatalf("parse itemRecfile: %v", err)
	}
	s := &srv{cfg: Config{RecfilePath: "item.rec", UIConfig: uiCfg}}
	s.types.Store(&types)
	// Return the Item type for key lookup convenience.
	rt := types[0]
	return s, rt
}

// getHTML issues a GET to the given path on s using Accept: text/html and
// returns the response body as a string.
func getHTML(t *testing.T, s *srv, path, recordType, key string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Accept", "text/html")
	req.SetPathValue("TypeName", recordType)
	req.SetPathValue("Key", key)
	w := httptest.NewRecorder()
	if err := s.handleRecord(w, req); err != nil {
		t.Fatalf("handleRecord: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	return w.Body.String()
}

// TestRenderRecord_ConfigTitle verifies that when tc.Title names a non-first
// field, that field's value is used as the <h2> heading in the HTML response.
func TestRenderRecord_ConfigTitle(t *testing.T) {
	uiCfg := config.UIConfig{
		"Item": {Title: "Name"},
	}
	s, _ := newItemSrv(t, uiCfg)
	// Key is the ID field value; Title is "Name" so heading should be "First Item".
	body := getHTML(t, s, "/types/Item/alpha", "Item", "alpha")
	if !strings.Contains(body, "<h2>First Item</h2>") {
		t.Errorf("expected <h2>First Item</h2> in body; got:\n%s", body)
	}
	// The ID field value must not appear as the heading.
	if strings.Contains(body, "<h2>alpha</h2>") {
		t.Errorf("unexpected <h2>alpha</h2> (first-field fallback) in body")
	}
}

// TestRenderRecord_ExcludeField verifies that a field configured with
// Exclude: true does not appear anywhere in the HTML response body.
func TestRenderRecord_ExcludeField(t *testing.T) {
	uiCfg := config.UIConfig{
		"Item": {Fields: map[string]config.FieldConfig{
			"Tags": {Exclude: true},
		}},
	}
	s, _ := newItemSrv(t, uiCfg)
	body := getHTML(t, s, "/types/Item/alpha", "Item", "alpha")
	// Neither the field name nor any of its values should appear in the output.
	if strings.Contains(body, "Tags") {
		t.Errorf("excluded field name 'Tags' found in body")
	}
	for _, v := range []string{"foo", "bar", "baz"} {
		if strings.Contains(body, v) {
			t.Errorf("excluded field value %q found in body", v)
		}
	}
}

// TestRenderRecord_InlineSep verifies that list_format = "inline" with a
// custom sep joins multi-valued field entries with that separator in the HTML.
func TestRenderRecord_InlineSep(t *testing.T) {
	uiCfg := config.UIConfig{
		"Item": {Fields: map[string]config.FieldConfig{
			"Tags": {ListFormat: "inline", Sep: " | "},
		}},
	}
	s, _ := newItemSrv(t, uiCfg)
	body := getHTML(t, s, "/types/Item/alpha", "Item", "alpha")
	// The three tag values must appear joined by the configured separator.
	if !strings.Contains(body, "foo | bar | baz") {
		t.Errorf("expected 'foo | bar | baz' in body; got:\n%s", body)
	}
}

// newIntegrationServerWithConfig starts an httptest.Server backed by a srv
// loaded from the sample recfile with the given UIConfig applied. The caller
// must call ts.Close().
func newIntegrationServerWithConfig(t *testing.T, uiCfg config.UIConfig) *httptest.Server {
	t.Helper()
	types, err := recfile.ParseFile("../../testdata/sample.rec")
	if err != nil {
		t.Fatalf("parse sample.rec: %v", err)
	}
	s := &srv{cfg: Config{RecfilePath: "../../testdata/sample.rec", UIConfig: uiCfg}}
	s.types.Store(&types)
	return httptest.NewServer(requestLogger(s.buildMux()))
}

// TestHandleRecord_BrowseGuard asserts that a direct GET to a record URL for a
// non-browseable type returns 404, matching handleType's contract. Covers both
// the HTML and JSON:API code paths (fix 1 in the recui branch review).
func TestHandleRecord_BrowseGuard(t *testing.T) {
	browseFalse := false
	uiCfg := config.UIConfig{
		"Author": {Browse: &browseFalse},
	}
	ts := newIntegrationServerWithConfig(t, uiCfg)
	defer ts.Close()
	// Any valid Author record key from sample.rec.
	const key = "a1b2c3d4-0001-0000-0000-000000000000"
	url := ts.URL + "/types/Author/" + key
	for _, accept := range []string{contentTypeJSONAPI, "text/html"} {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("Accept", accept)
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("accept %q: status = %d, want 404", accept, resp.StatusCode)
		}
	}
}

// TestMethodNotAllowed_AllowHeader asserts that a 405 response carries an
// Allow header listing the methods the endpoint does accept (RFC 7231 §6.5.5
// MUST requirement; fix 7 in the recui branch review).
func TestMethodNotAllowed_AllowHeader(t *testing.T) {
	ts := newIntegrationServer(t)
	defer ts.Close()
	cases := []struct {
		name, method, path, wantAllow string
	}{
		// Home supports GET and HEAD.
		{"home POST", http.MethodPost, "/", "GET, HEAD"},
		// Type collection supports GET only.
		{"type POST", http.MethodPost, "/types/Author", "GET"},
		// Record supports GET only.
		{"record DELETE", http.MethodDelete, "/types/Author/a1b2c3d4-0001-0000-0000-000000000000", "GET"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, nil)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			resp, err := ts.Client().Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", tc.method, tc.path, err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want 405", resp.StatusCode)
			}
			allow := resp.Header.Get("Allow")
			if allow != tc.wantAllow {
				t.Errorf("Allow = %q, want %q", allow, tc.wantAllow)
			}
		})
	}
}

// TestNotAcceptable asserts that a request whose Accept header lists no
// representation the server can satisfy returns 406 instead of silently
// falling back to JSON:API (fix 9 in the recui branch review).
func TestNotAcceptable(t *testing.T) {
	ts := newIntegrationServer(t)
	defer ts.Close()
	for _, tc := range []struct {
		name, accept string
	}{
		{"xml only", "application/xml"},
		{"image png", "image/png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			req.Header.Set("Accept", tc.accept)
			resp, err := ts.Client().Do(req)
			if err != nil {
				t.Fatalf("GET /: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusNotAcceptable {
				t.Errorf("status = %d, want 406", resp.StatusCode)
			}
		})
	}
}

// TestNotAcceptable_StarAllowed asserts that Accept: */* is treated as
// satisfiable and defaults to JSON:API, rather than being rejected with 406.
func TestNotAcceptable_StarAllowed(t *testing.T) {
	ts := newIntegrationServer(t)
	defer ts.Close()
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Accept", "*/*")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/vnd.api+json") {
		t.Errorf("Content-Type = %q, want application/vnd.api+json", ct)
	}
}

// TestRecord_CollectionRelationshipShape asserts that the "collection"
// back-reference on a record resource is a to-one relationship — its `data`
// field is a single object, not an array. Covers fix 6 in the recui branch
// review (JSON:API §7.5).
func TestRecord_CollectionRelationshipShape(t *testing.T) {
	ts := newIntegrationServer(t)
	defer ts.Close()
	const key = "a1b2c3d4-0001-0000-0000-000000000000"
	var doc map[string]any
	resp := getJSON(t, ts.Client(), ts.URL+"/types/Author/"+key, &doc)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	data := doc["data"].(map[string]any)
	rels, ok := data["relationships"].(map[string]any)
	if !ok {
		t.Fatalf("data.relationships is not an object: %T", data["relationships"])
	}
	coll, ok := rels["collection"].(map[string]any)
	if !ok {
		t.Fatalf("data.relationships.collection is not an object: %T", rels["collection"])
	}
	// Must be a single object, not an array.
	if _, isArr := coll["data"].([]any); isArr {
		t.Errorf("collection.data is an array; JSON:API §7.5 requires to-one `data` to be a single object")
	}
	linkage, ok := coll["data"].(map[string]any)
	if !ok {
		t.Fatalf("collection.data is not an object: %T", coll["data"])
	}
	if linkage["type"] != "record-type-collection" {
		t.Errorf("collection.data.type = %v, want record-type-collection", linkage["type"])
	}
	if linkage["id"] != "Author" {
		t.Errorf("collection.data.id = %v, want Author", linkage["id"])
	}
}

// TestRecord_UpLinkNonBrowseable asserts that an included record resource for
// a non-browseable foreign key target has its "up" link pointed at the home
// URL rather than the 404ing type collection URL. This brings the JSON:API
// path to parity with the HTML upURL helper (fix 8 in the recui branch
// review).
//
// The test makes the Author type non-browseable, then fetches a Book record
// whose Author field references an Author — the Author appears as an included
// resource on the Book response and its `up` link must be "/".
func TestRecord_UpLinkNonBrowseable(t *testing.T) {
	browseFalse := false
	uiCfg := config.UIConfig{
		"Author": {Browse: &browseFalse},
	}
	ts := newIntegrationServerWithConfig(t, uiCfg)
	defer ts.Close()
	// A Book that references the first Author from sample.rec.
	const bookKey = "b0000001-0000-0000-0000-000000000001"
	var doc map[string]any
	resp := getJSON(t, ts.Client(), ts.URL+"/types/Book/"+bookKey, &doc)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	included, ok := doc["included"].([]any)
	if !ok {
		t.Fatalf("included is not an array: %T", doc["included"])
	}
	// Find the included Author record.
	var authorRes map[string]any
	for _, r := range included {
		res := r.(map[string]any)
		if res["type"] == "record" {
			if id, _ := res["id"].(string); strings.HasPrefix(id, "Author/") {
				authorRes = res
				break
			}
		}
	}
	if authorRes == nil {
		t.Fatal("no included Author record resource found")
	}
	links, ok := authorRes["links"].(map[string]any)
	if !ok {
		t.Fatalf("author links missing: %T", authorRes["links"])
	}
	up, _ := links["up"].(string)
	if up != "/" {
		t.Errorf("included Author up link = %q, want %q (home URL for non-browseable type)", up, "/")
	}
}

// TestIntegration_HeadHome asserts that HEAD / returns 200 with a Link header
// containing rel="item".
func TestIntegration_HeadHome(t *testing.T) {
	ts := newIntegrationServer(t)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodHead, ts.URL+"/", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("HEAD /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	link := resp.Header.Get("Link")
	if link == "" {
		t.Fatal("no Link header in HEAD / response")
	}
	if !strings.Contains(link, `rel="item"`) {
		t.Errorf("Link header does not contain rel=\"item\": %q", link)
	}
}
