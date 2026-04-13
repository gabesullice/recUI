package server

import (
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	assets "github.com/gabesullice/recui"
	"github.com/gabesullice/recui/pkg/config"
	"github.com/gabesullice/recui/pkg/recfile"
)

// webFS is a sub-filesystem rooted at web/ within the embedded assets.
var webFS = func() fs.FS {
	sub, err := fs.Sub(assets.FS, "web")
	if err != nil {
		panic("assets: cannot sub into web/: " + err.Error())
	}
	return sub
}()

// templates holds the parsed HTML templates, embedded at compile time.
var templates = template.Must(
	template.New("").ParseFS(assets.FS,
		"web/home.html",
		"web/type-list.html",
		"web/record.html",
	),
)

// negotiated is the outcome of content negotiation for a request.
type negotiated int

const (
	negotiatedJSONAPI negotiated = iota
	negotiatedHTML
	negotiatedNotAcceptable
)

// negotiate inspects the request's Accept header and decides which
// representation to serve. Absent or `*/*` headers default to JSON:API.
// Headers listing only types we cannot satisfy yield negotiatedNotAcceptable.
func negotiate(r *http.Request) negotiated {
	accept := r.Header.Get("Accept")
	if accept == "" {
		return negotiatedJSONAPI
	}
	// */* always acceptable; default to JSON:API unless HTML was explicitly
	// preferred.
	hasStar := strings.Contains(accept, "*/*")
	htmlIdx := strings.Index(accept, "text/html")
	jsonapiIdx := strings.Index(accept, "application/vnd.api+json")
	htmlOK := htmlIdx != -1
	jsonapiOK := jsonapiIdx != -1
	if !htmlOK && !jsonapiOK && !hasStar {
		return negotiatedNotAcceptable
	}
	// HTML wins if present and either JSON:API is absent or HTML appears first.
	if htmlOK && (!jsonapiOK || htmlIdx < jsonapiIdx) {
		return negotiatedHTML
	}
	return negotiatedJSONAPI
}

// prefersHTML reports whether content negotiation selected HTML.
func prefersHTML(r *http.Request) bool {
	return negotiate(r) == negotiatedHTML
}

// homeData is the template data for the home page.
type homeData struct {
	Title string
	Types []typeRow
}

type typeRow struct {
	Name  string
	URL   string
	Count int
	Doc   string
}

// typeData is the template data for a record type listing page.
type typeData struct {
	Title     string
	TypeURL   string
	HomeURL   string
	Doc       string
	Records   []recordLink
	PrevURL   string
	PrevTitle string
	NextURL   string
	NextTitle string
}

type recordLink struct {
	Title string
	URL   string
}

// recordData is the template data for a single record detail page.
type recordData struct {
	Title     string
	Heading   string
	Fields    []renderedField
	HomeURL   string
	TypeURL   string
	TypeName  string
	PrevURL   string
	PrevTitle string
	NextURL   string
	NextTitle string
	UpURL     string
}

// linkItem pairs a display title with a URL, used for multi-valued link fields.
type linkItem struct {
	Title string
	URL   string
}

// renderedField carries pre-computed rendering instructions for a single
// (possibly multi-valued) field on the record detail page.
type renderedField struct {
	// Name is the field name (label).
	Name string
	// Kind controls which template block is used to render the field.
	// Values: "inline", "inline-sep", "fk-single", "fk-multi",
	//         "email-single", "email-multi", "url-single", "url-multi",
	//         "enum-multi", "block-single", "block-multi"
	// "block-multi" also covers list_format = "ol" — the block-multi template
	// already renders an <ol>, so there is no need for a separate kind.
	Kind string
	// Single is the single value (for single-valued fields or inline-sep).
	Single string
	// SingleURL is the URL for single-valued link fields.
	SingleURL string
	// Multi is the slice of plain values (for multi-valued non-link fields).
	Multi []string
	// Links is the slice of title+URL pairs for multi-valued link fields.
	Links []linkItem
	// Sep is the pre-computed separator for Kind=="inline-sep".
	Sep string
	// Label overrides the default label placement; empty means type-derived default.
	// Values: "h3" | "inline" | "omit" | ""
	Label string
}

// recordTitle resolves the display title for a record, returning both the title
// string and the field name whose value was selected. fieldName is empty when a
// template title was used or the first-field fallback applied; the caller uses it
// to skip that field from the record detail list.
func recordTitle(rec recfile.Record, rt recfile.RecordType, tc config.TypeConfig) (title, fieldName string) {
	// a. Configured title: Go template or plain field name.
	if tc.Title != "" {
		if tmpl := tc.TitleTemplate(); tmpl != nil {
			// Escape contract: this template's output is a plain Go string
			// that flows into recordData.Heading/Title, both of which are
			// rendered by html/template downstream. html/template auto-escapes
			// string values, which is why it is safe to use text/template
			// here even though the input is user-controlled via the TOML
			// config. DO NOT wrap this result in template.HTML, and DO NOT
			// change Heading/Title to template.HTML in recordData — either
			// change would silently disable the escaping and open a stored
			// XSS path. The template itself is pre-parsed at config-load
			// time by config.LoadConfig, so a malformed template is a
			// startup error rather than a silent render-time fallback;
			// execution errors below (e.g. a field missing from a specific
			// record) are still handled by the fallback chain.
			data := make(map[string]string, len(rec.FieldNames()))
			for _, name := range rec.FieldNames() {
				if vals := rec.FieldValues(name); len(vals) > 0 {
					data[name] = vals[0]
				}
			}
			var buf strings.Builder
			if err := tmpl.Execute(&buf, data); err == nil {
				return buf.String(), ""
			}
			// Fall through on execution error.
		} else {
			if vals := rec.FieldValues(tc.Title); len(vals) > 0 {
				return vals[0], tc.Title
			}
			slog.Warn("configured title field absent from record; falling back",
				"type", rt.Name, "title_field", tc.Title)
			// Fall through to auto-detect.
		}
	}
	// b. Well-known field names in priority order.
	fieldSet := make(map[string]bool, len(rec.FieldNames()))
	for _, n := range rec.FieldNames() {
		fieldSet[n] = true
	}
	for _, candidate := range []string{"Title", "Label", "Name", "ID"} {
		if fieldSet[candidate] {
			if vals := rec.FieldValues(candidate); len(vals) > 0 {
				return vals[0], candidate
			}
		}
	}
	// c. First unique+mandatory field (in record field order).
	uniqueSet := make(map[string]bool, len(rt.Unique))
	for _, n := range rt.Unique {
		uniqueSet[n] = true
	}
	mandatorySet := make(map[string]bool, len(rt.Mandatory))
	for _, n := range rt.Mandatory {
		mandatorySet[n] = true
	}
	for _, name := range rec.FieldNames() {
		if uniqueSet[name] && mandatorySet[name] {
			if vals := rec.FieldValues(name); len(vals) > 0 {
				return vals[0], name
			}
		}
	}
	// d. First unique-only field (in record field order).
	for _, name := range rec.FieldNames() {
		if uniqueSet[name] {
			if vals := rec.FieldValues(name); len(vals) > 0 {
				return vals[0], name
			}
		}
	}
	// e. First field.
	first := rec.FirstField()
	return first.Value, first.Name
}

// orderedFieldNames returns the field names for rec in display order.
// Fields listed in tc.FieldOrder appear first (in that order); remaining
// fields follow in their original declaration order.
func orderedFieldNames(rec recfile.Record, tc config.TypeConfig) []string {
	all := rec.FieldNames()
	if len(tc.FieldOrder) == 0 {
		return all
	}
	seen := make(map[string]bool, len(all))
	for _, n := range all {
		seen[n] = true
	}
	ordered := make([]string, 0, len(all))
	for _, n := range tc.FieldOrder {
		if seen[n] {
			ordered = append(ordered, n)
			seen[n] = false // mark consumed
		}
	}
	for _, n := range all {
		if seen[n] {
			ordered = append(ordered, n)
		}
	}
	return ordered
}

// renderHome renders the home page listing all record types.
func (s *srv) renderHome(w http.ResponseWriter, r *http.Request, types []recfile.RecordType) error {
	rows := make([]typeRow, 0, len(types))
	for _, rt := range types {
		if !s.cfg.UIConfig[rt.Name].Browseable() {
			continue
		}
		rows = append(rows, typeRow{
			Name:  rt.Name,
			URL:   typeRoute{TypeName: rt.Name}.URL(),
			Count: len(rt.Records),
			Doc:   rt.Doc,
		})
	}
	data := homeData{
		Title: "recui",
		Types: rows,
	}
	return executeTemplate(w, "home", data)
}

// renderType renders the listing page for a single record type.
func (s *srv) renderType(w http.ResponseWriter, r *http.Request, rt recfile.RecordType) error {
	allTypes := *s.types.Load()
	tc := s.cfg.UIConfig[rt.Name]
	links := make([]recordLink, 0, len(rt.Records))
	for i, rec := range rt.Records {
		key := recordID(rt, i, rec)
		title, _ := recordTitle(rec, rt, tc)
		if title == "" {
			title = key
		}
		links = append(links, recordLink{
			Title: title,
			URL:   recordRoute{TypeName: rt.Name, Key: key}.URL(),
		})
	}
	data := typeData{
		Title:   rt.Name,
		TypeURL: typeRoute{TypeName: rt.Name}.URL(),
		HomeURL: homeRoute{}.URL(),
		Doc:     rt.Doc,
		Records: links,
	}
	// Build prev/next using only browseable types.
	browseableTypes := make([]recfile.RecordType, 0, len(allTypes))
	for _, t := range allTypes {
		if s.cfg.UIConfig[t.Name].Browseable() {
			browseableTypes = append(browseableTypes, t)
		}
	}
	for i, t := range browseableTypes {
		if t.Name != rt.Name {
			continue
		}
		if i > 0 {
			prev := browseableTypes[i-1]
			data.PrevURL = typeRoute{TypeName: prev.Name}.URL()
			data.PrevTitle = prev.Name
		}
		if i < len(browseableTypes)-1 {
			next := browseableTypes[i+1]
			data.NextURL = typeRoute{TypeName: next.Name}.URL()
			data.NextTitle = next.Name
		}
		break
	}
	return executeTemplate(w, "type-list", data)
}

// renderRecord renders the detail page for a single record.
func (s *srv) renderRecord(w http.ResponseWriter, r *http.Request, rt recfile.RecordType, idx int, rec recfile.Record) error {
	types := *s.types.Load()
	tc := s.cfg.UIConfig[rt.Name]

	heading, titleFieldName := recordTitle(rec, rt, tc)

	// Build prev/next navigation.
	var prevURL, prevTitle, nextURL, nextTitle string
	if idx > 0 {
		prev := rt.Records[idx-1]
		prevURL = recordRoute{TypeName: rt.Name, Key: recordID(rt, idx-1, prev)}.URL()
		prevTitle, _ = recordTitle(prev, rt, tc)
	}
	if idx < len(rt.Records)-1 {
		next := rt.Records[idx+1]
		nextURL = recordRoute{TypeName: rt.Name, Key: recordID(rt, idx+1, next)}.URL()
		nextTitle, _ = recordTitle(next, rt, tc)
	}

	// Build rendered fields using config-ordered field names.
	names := orderedFieldNames(rec, tc)
	fields := make([]renderedField, 0, len(names))
	for _, name := range names {
		// Title field is rendered as the page heading; skip it here.
		if name == titleFieldName {
			continue
		}
		fc := tc.Fields[name]
		if fc.Exclude {
			continue
		}
		vals := rec.FieldValues(name)
		rf := buildRenderedField(name, vals, rt, types, fc, s.cfg.UIConfig)
		fields = append(fields, rf)
	}

	data := recordData{
		Title:     heading + " — " + rt.Name,
		Heading:   heading,
		Fields:    fields,
		HomeURL:   homeRoute{}.URL(),
		TypeURL:   typeRoute{TypeName: rt.Name}.URL(),
		TypeName:  rt.Name,
		PrevURL:   prevURL,
		PrevTitle: prevTitle,
		NextURL:   nextURL,
		NextTitle: nextTitle,
		UpURL:     s.upURL(rt.Name),
	}
	return executeTemplate(w, "record", data)
}

// buildRenderedField constructs a renderedField for the given field name and
// values, looking up type metadata and foreign key targets as needed.
// fc carries any field-level display overrides from the UI config.
func buildRenderedField(name string, vals []string, rt recfile.RecordType, allTypes []recfile.RecordType, fc config.FieldConfig, uiCfg config.UIConfig) renderedField {
	ft, hasType := rt.FieldTypes[name]
	targetTypeName := rt.ForeignKeys[name]
	multi := len(vals) > 1

	switch {
	case hasType && ft.Kind == "rec" && !multi:
		// Single foreign key: → Title link
		url, title := resolveForeignKey(targetTypeName, vals[0], allTypes, uiCfg)
		return renderedField{Name: name, Kind: "fk-single", SingleURL: url, Single: title, Label: fc.Label}

	case hasType && ft.Kind == "rec" && multi:
		// Multi foreign key: list of → Title links
		items := make([]linkItem, 0, len(vals))
		for _, v := range vals {
			u, title := resolveForeignKey(targetTypeName, v, allTypes, uiCfg)
			items = append(items, linkItem{Title: title, URL: u})
		}
		return renderedField{Name: name, Kind: "fk-multi", Links: items, Label: fc.Label}

	case hasType && ft.Kind == "email" && !multi:
		return renderedField{Name: name, Kind: "email-single", Single: vals[0], Label: fc.Label}

	case hasType && ft.Kind == "email" && multi:
		return renderedField{Name: name, Kind: "email-multi", Multi: vals, Label: fc.Label}

	case hasType && ft.Kind == "url" && !multi:
		return renderedField{Name: name, Kind: "url-single", Single: vals[0], Label: fc.Label}

	case hasType && ft.Kind == "url" && multi:
		return renderedField{Name: name, Kind: "url-multi", Multi: vals, Label: fc.Label}

	case hasType && (ft.Kind == "uuid" || ft.Kind == "date") && !multi:
		return renderedField{Name: name, Kind: "inline", Single: vals[0], Label: fc.Label}

	case hasType && (ft.Kind == "uuid" || ft.Kind == "date") && multi:
		return renderedField{Name: name, Kind: "block-multi", Multi: vals, Label: fc.Label}

	case hasType && ft.Kind == "enum" && !multi:
		return renderedField{Name: name, Kind: "inline", Single: vals[0], Label: fc.Label}

	case hasType && ft.Kind == "enum" && multi:
		// enum-multi: apply list_format override if configured.
		if fc.ListFormat == "ol" {
			return renderedField{Name: name, Kind: "block-multi", Multi: vals, Label: fc.Label}
		}
		if fc.ListFormat == "inline" {
			sep := fc.Sep
			if sep == "" {
				sep = ", "
			}
			return renderedField{Name: name, Kind: "inline-sep", Single: strings.Join(vals, sep), Sep: sep, Label: fc.Label}
		}
		return renderedField{Name: name, Kind: "enum-multi", Multi: vals, Label: fc.Label}

	case !multi:
		// Untyped single-valued field: apply label override.
		kind := "block-single"
		if fc.Label == "inline" {
			kind = "inline"
		}
		return renderedField{Name: name, Kind: kind, Single: vals[0], Label: fc.Label}

	default:
		// Untyped multi-valued field: apply list_format override.
		if fc.ListFormat == "ol" {
			return renderedField{Name: name, Kind: "block-multi", Multi: vals, Label: fc.Label}
		}
		if fc.ListFormat == "inline" {
			sep := fc.Sep
			if sep == "" {
				sep = ", "
			}
			return renderedField{Name: name, Kind: "inline-sep", Single: strings.Join(vals, sep), Sep: sep, Label: fc.Label}
		}
		return renderedField{Name: name, Kind: "block-multi", Multi: vals, Label: fc.Label}
	}
}

// resolveForeignKey looks up a referenced record and returns its URL and display
// title. Falls back to the raw value string if the record is not found.
func resolveForeignKey(targetTypeName, keyVal string, allTypes []recfile.RecordType, uiCfg config.UIConfig) (url, title string) {
	targetRT, found := findType(allTypes, targetTypeName)
	if !found {
		return "", keyVal
	}
	idx, rec, found := findRecord(targetRT, keyVal)
	if !found {
		return typeRoute{TypeName: targetTypeName}.URL(), keyVal
	}
	url = recordRoute{TypeName: targetTypeName, Key: recordID(targetRT, idx, rec)}.URL()
	tc := uiCfg[targetTypeName]
	title, _ = recordTitle(rec, targetRT, tc)
	if title == "" {
		title = keyVal
	}
	return url, title
}

// upURL returns the URL for the "up" breadcrumb from a record page.
// For non-browseable types the collection page does not exist, so we send the
// user to the home page instead.
func (s *srv) upURL(typeName string) string {
	if s.cfg.UIConfig[typeName].Browseable() {
		return typeRoute{TypeName: typeName}.URL()
	}
	return homeRoute{}.URL()
}

// executeTemplate renders the named template (one of the self-contained
// per-page templates in web/) to the response with the given data.
func executeTemplate(w http.ResponseWriter, name string, data any) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return templates.ExecuteTemplate(w, name, data)
}
