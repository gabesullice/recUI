package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gabesullice/recui/pkg/recfile"
)

const contentTypeJSONAPI = "application/vnd.api+json"

// RecfileIndexAttrs are the attributes for the recfile-index resource.
type RecfileIndexAttrs struct {
	Filename string `json:"filename"`
}

// RecordTypeCollectionAttrs are the attributes for a record-type-collection resource.
type RecordTypeCollectionAttrs struct {
	RecordType string `json:"record-type"`
	Count      int    `json:"count"`
	Doc        string `json:"doc,omitempty"`
}

// collectionAttrsFor builds the JSON:API attribute struct for the given
// record type's collection resource. Centralised so the three call sites
// (home, type, record) stay in sync.
func collectionAttrsFor(rt recfile.RecordType) RecordTypeCollectionAttrs {
	return RecordTypeCollectionAttrs{
		RecordType: rt.Name,
		Count:      len(rt.Records),
		Doc:        rt.Doc,
	}
}

// writeJSON encodes v as JSON to w with the given Content-Type and status.
func writeJSON(w http.ResponseWriter, contentType string, status int, v any) error {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

// setCommonHeaders writes ETag, Cache-Control, Vary, and Last-Modified. Returns
// true (and sends 304) if the request has a matching If-None-Match header.
// Last-Modified is sourced from the cached s.lastMtime (refreshed by pollLoop)
// so it is consistent with the ETag and does not incur a per-request syscall.
func (s *srv) setCommonHeaders(w http.ResponseWriter, r *http.Request) bool {
	etag := s.etag()
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Vary", "Accept")
	if mtimeNanos := s.lastMtime.Load(); mtimeNanos != 0 {
		w.Header().Set("Last-Modified", time.Unix(0, mtimeNanos).UTC().Format(time.RFC1123))
	}
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}

// warnKeyless emits a one-per-session warning for keyless record types.
func (s *srv) warnKeyless(typeName string) {
	if _, loaded := s.keylessWarned.LoadOrStore(typeName, struct{}{}); !loaded {
		slog.Warn("serving keyless record type; URLs use positional index", "type", typeName)
	}
}

// recordID returns the URL key segment for a record: key field value for keyed
// types, zero-based decimal index for keyless types.
func recordID(rt recfile.RecordType, idx int, rec recfile.Record) string {
	if rt.Key != "" {
		vals := rec.FieldValues(rt.Key)
		if len(vals) > 0 {
			return vals[0]
		}
	}
	return fmt.Sprintf("%d", idx)
}

// buildRecordResource constructs the AnyResource for a single record, including
// prev/next/first/last links. upURL is the URL to use for the "up" link — for
// browseable types this is the type collection, for non-browseable types the
// caller passes the home URL since the collection 404s.
func buildRecordResource(rt recfile.RecordType, idx int, rec recfile.Record, upURL string) (AnyResource, error) {
	id := fmt.Sprintf("%s/%s", rt.Name, recordID(rt, idx, rec))
	selfURL := recordRoute{TypeName: rt.Name, Key: recordID(rt, idx, rec)}.URL()

	// Build attributes: field name → string or []string for multivalued.
	attrs := make(map[string]any)
	for _, name := range rec.FieldNames() {
		vals := rec.FieldValues(name)
		if len(vals) == 1 {
			attrs[name] = vals[0]
		} else {
			attrs[name] = vals
		}
	}

	attrsRaw, err := json.Marshal(attrs)
	if err != nil {
		return AnyResource{}, err
	}

	links := Links{
		"self": selfURL,
		"up":   upURL,
	}
	// first/last
	links["first"] = recordRoute{TypeName: rt.Name, Key: recordID(rt, 0, rt.Records[0])}.URL()
	last := len(rt.Records) - 1
	links["last"] = recordRoute{TypeName: rt.Name, Key: recordID(rt, last, rt.Records[last])}.URL()
	// prev
	if idx > 0 {
		links["prev"] = recordRoute{TypeName: rt.Name, Key: recordID(rt, idx-1, rt.Records[idx-1])}.URL()
	}
	// next
	if idx < len(rt.Records)-1 {
		links["next"] = recordRoute{TypeName: rt.Name, Key: recordID(rt, idx+1, rt.Records[idx+1])}.URL()
	}

	// Relationships for foreign key fields. Every FK field is modelled as a
	// to-many relationship — recutils foreign-key semantics allow multivalued
	// %type: _ rec _ fields, and we cannot distinguish a single-value instance
	// from a to-many with one entry at the schema level.
	rels := map[string]Relationship{}
	for fieldName, targetType := range rt.ForeignKeys {
		vals := rec.FieldValues(fieldName)
		linkages := make([]RelationshipLinkage, 0, len(vals))
		for _, v := range vals {
			linkages = append(linkages, RelationshipLinkage{
				Type: TypeRecord,
				ID:   fmt.Sprintf("%s/%s", targetType, v),
			})
		}
		rel := ToMany(linkages)
		rel.Links = Links{"related": typeRoute{TypeName: targetType}.URL()}
		rels[fieldName] = rel
	}

	res := AnyResource{
		Type:       TypeRecord,
		ID:         id,
		Attributes: json.RawMessage(attrsRaw),
		Links:      links,
	}
	if len(rels) > 0 {
		res.Relationships = rels
	}
	return res, nil
}

// handleHome handles GET /.
func (s *srv) handleHome(w http.ResponseWriter, r *http.Request) error {
	n := negotiate(r)
	if n == negotiatedNotAcceptable {
		return errNotAcceptable
	}
	types := *s.types.Load()
	if n == negotiatedHTML {
		return s.renderHome(w, r, types)
	}
	if s.setCommonHeaders(w, r) {
		return nil
	}

	// Build relationship linkages and included resources for each browseable type.
	linkages := make([]RelationshipLinkage, 0, len(types))
	included := make([]AnyResource, 0, len(types))
	for _, rt := range types {
		if !s.cfg.UIConfig[rt.Name].Browseable() {
			continue
		}
		linkages = append(linkages, RelationshipLinkage{
			Type: TypeRecordTypeCollection,
			ID:   rt.Name,
		})
		attrsRaw, err := json.Marshal(collectionAttrsFor(rt))
		if err != nil {
			return err
		}
		included = append(included, AnyResource{
			Type:       TypeRecordTypeCollection,
			ID:         rt.Name,
			Attributes: json.RawMessage(attrsRaw),
			Links:      Links{"self": typeRoute{TypeName: rt.Name}.URL()},
		})
	}

	doc := Document[RecfileIndexAttrs]{
		Data: Resource[RecfileIndexAttrs]{
			Type: TypeRecfileIndex,
			ID:   "default",
			Attributes: RecfileIndexAttrs{
				Filename: filepath.Base(s.cfg.RecfilePath),
			},
			Relationships: map[string]Relationship{
				"record-types": ToMany(linkages),
			},
		},
		Included: included,
	}
	return writeJSON(w, contentTypeJSONAPI, http.StatusOK, doc)
}

// handleType handles GET /types/{TypeName}.
func (s *srv) handleType(w http.ResponseWriter, r *http.Request) error {
	n := negotiate(r)
	if n == negotiatedNotAcceptable {
		return errNotAcceptable
	}
	typeName := r.PathValue("TypeName")
	types := *s.types.Load()
	rt, ok := findType(types, typeName)
	if !ok {
		return errNotFound
	}
	if !s.cfg.UIConfig[rt.Name].Browseable() {
		return errNotFound
	}
	if rt.Key == "" {
		s.warnKeyless(rt.Name)
	}
	if n == negotiatedHTML {
		return s.renderType(w, r, rt)
	}

	// Build relationship linkages for items and included record resources.
	// Records inside a browseable collection use the collection URL for "up".
	up := typeRoute{TypeName: rt.Name}.URL()
	itemLinkages := make([]RelationshipLinkage, 0, len(rt.Records))
	included := make([]AnyResource, 0, len(rt.Records))
	for i, rec := range rt.Records {
		id := fmt.Sprintf("%s/%s", rt.Name, recordID(rt, i, rec))
		itemLinkages = append(itemLinkages, RelationshipLinkage{
			Type: TypeRecord,
			ID:   id,
		})
		res, err := buildRecordResource(rt, i, rec, up)
		if err != nil {
			return err
		}
		included = append(included, res)
	}

	doc := Document[RecordTypeCollectionAttrs]{
		Data: Resource[RecordTypeCollectionAttrs]{
			Type:       TypeRecordTypeCollection,
			ID:         rt.Name,
			Attributes: collectionAttrsFor(rt),
			Relationships: map[string]Relationship{
				"items": ToMany(itemLinkages),
			},
			Links: Links{"self": typeRoute{TypeName: rt.Name}.URL()},
		},
		Included: included,
	}
	if s.setCommonHeaders(w, r) {
		return nil
	}
	return writeJSON(w, contentTypeJSONAPI, http.StatusOK, doc)
}

// handleRecord handles GET /types/{TypeName}/{Key}.
func (s *srv) handleRecord(w http.ResponseWriter, r *http.Request) error {
	n := negotiate(r)
	if n == negotiatedNotAcceptable {
		return errNotAcceptable
	}
	typeName := r.PathValue("TypeName")
	keyVal := r.PathValue("Key")
	types := *s.types.Load()
	rt, ok := findType(types, typeName)
	if !ok {
		return errNotFound
	}
	// Browseable() gate: link-only types must 404 on direct record URLs too.
	// Applied before HTML/JSON:API branching so both code paths share the
	// same contract as handleType.
	if !s.cfg.UIConfig[rt.Name].Browseable() {
		return errNotFound
	}
	if rt.Key == "" {
		s.warnKeyless(rt.Name)
	}

	idx, rec, ok := findRecord(rt, keyVal)
	if !ok {
		return errNotFound
	}
	if n == negotiatedHTML {
		return s.renderRecord(w, r, rt, idx, rec)
	}

	res, err := buildRecordResource(rt, idx, rec, s.upURL(rt.Name))
	if err != nil {
		return err
	}

	// Decode attributes back to map[string]any for the primary data document.
	var attrsMap map[string]any
	if err := json.Unmarshal(res.Attributes, &attrsMap); err != nil {
		return err
	}

	// Build included: parent collection + any referenced foreign key records.
	included := []AnyResource{}

	// Include parent collection resource.
	collAttrsRaw, err := json.Marshal(collectionAttrsFor(rt))
	if err != nil {
		return err
	}
	included = append(included, AnyResource{
		Type:       TypeRecordTypeCollection,
		ID:         rt.Name,
		Attributes: json.RawMessage(collAttrsRaw),
		Links:      Links{"self": typeRoute{TypeName: rt.Name}.URL()},
	})

	// Include referenced records for each foreign key field. Each FK target may
	// itself be a non-browseable type, so the up link is computed per target.
	for fieldName, targetTypeName := range rt.ForeignKeys {
		vals := rec.FieldValues(fieldName)
		targetRT, found := findType(types, targetTypeName)
		if !found {
			continue
		}
		targetUp := s.upURL(targetTypeName)
		for _, fkVal := range vals {
			fkIdx, fkRec, found := findRecord(targetRT, fkVal)
			if !found {
				continue
			}
			fkRes, err := buildRecordResource(targetRT, fkIdx, fkRec, targetUp)
			if err != nil {
				return err
			}
			included = append(included, fkRes)
		}
	}

	// Build primary resource with proper type.
	primaryLinks := res.Links
	primaryRels := res.Relationships
	// Add "collection" back-reference to the parent collection. This is a
	// to-one relationship per JSON:API §7.5 — `data` is a single object.
	if primaryRels == nil {
		primaryRels = map[string]Relationship{}
	}
	collectionRel := ToOne(RelationshipLinkage{Type: TypeRecordTypeCollection, ID: rt.Name})
	collectionRel.Links = Links{"related": typeRoute{TypeName: rt.Name}.URL()}
	primaryRels["collection"] = collectionRel

	primaryRes := Resource[map[string]any]{
		Type:          TypeRecord,
		ID:            res.ID,
		Attributes:    attrsMap,
		Relationships: primaryRels,
		Links:         primaryLinks,
	}

	doc := Document[map[string]any]{
		Data:     primaryRes,
		Included: included,
	}
	if s.setCommonHeaders(w, r) {
		return nil
	}
	return writeJSON(w, contentTypeJSONAPI, http.StatusOK, doc)
}

// handleHead handles HEAD /.
func (s *srv) handleHead(w http.ResponseWriter, r *http.Request) error {
	types := *s.types.Load()
	var parts []string
	for _, rt := range types {
		if !s.cfg.UIConfig[rt.Name].Browseable() {
			continue
		}
		url := typeRoute{TypeName: rt.Name}.URL()
		parts = append(parts, fmt.Sprintf(`<%s>; rel="item"; title="%s"`, url, rt.Name))
	}
	if len(parts) > 0 {
		w.Header().Set("Link", strings.Join(parts, ", "))
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

// findType returns the RecordType with the given name, or false if not found.
func findType(types []recfile.RecordType, name string) (recfile.RecordType, bool) {
	for _, rt := range types {
		if rt.Name == name {
			return rt, true
		}
	}
	return recfile.RecordType{}, false
}

// findRecord locates a record by key value (keyed types) or decimal index
// (keyless types). Returns the index, record, and whether it was found.
func findRecord(rt recfile.RecordType, keyVal string) (int, recfile.Record, bool) {
	if rt.Key != "" {
		for i, rec := range rt.Records {
			vals := rec.FieldValues(rt.Key)
			if len(vals) > 0 && vals[0] == keyVal {
				return i, rec, true
			}
		}
		return 0, recfile.Record{}, false
	}
	// Keyless: parse index.
	idx, err := strconv.Atoi(keyVal)
	if err != nil {
		return 0, recfile.Record{}, false
	}
	if idx < 0 || idx >= len(rt.Records) {
		return 0, recfile.Record{}, false
	}
	return idx, rt.Records[idx], true
}
