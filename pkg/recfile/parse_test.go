package recfile

import (
	"strings"
	"testing"
)

// loadSample parses testdata/sample.rec and returns the result. Fails the
// test on any error so callers can focus on assertions.
func loadSample(t *testing.T) []RecordType {
	t.Helper()
	types, err := ParseFile("../../testdata/sample.rec")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	return types
}

// findType returns the RecordType with the given name, or fails the test.
func findType(t *testing.T, types []RecordType, name string) RecordType {
	t.Helper()
	for _, rt := range types {
		if rt.Name == name {
			return rt
		}
	}
	t.Fatalf("record type %q not found; types present: %v", name, typeNames(types))
	return RecordType{}
}

func typeNames(types []RecordType) []string {
	names := make([]string, len(types))
	for i, rt := range types {
		names[i] = rt.Name
	}
	return names
}

func TestParseSample_TypeCount(t *testing.T) {
	types := loadSample(t)
	if len(types) != 3 {
		t.Errorf("expected 3 record types, got %d: %v", len(types), typeNames(types))
	}
}

func TestParseSample_TypeNames(t *testing.T) {
	types := loadSample(t)
	names := typeNames(types)
	want := []string{"Author", "Book", "Tag"}
	for i, w := range want {
		if i >= len(names) || names[i] != w {
			t.Errorf("type[%d]: want %q, got %q", i, w, names[i])
		}
	}
}

func TestParseSample_AuthorRecordCount(t *testing.T) {
	types := loadSample(t)
	author := findType(t, types, "Author")
	if len(author.Records) != 3 {
		t.Errorf("Author: expected 3 records, got %d", len(author.Records))
	}
}

func TestParseSample_BookRecordCount(t *testing.T) {
	types := loadSample(t)
	book := findType(t, types, "Book")
	if len(book.Records) != 4 {
		t.Errorf("Book: expected 4 records, got %d", len(book.Records))
	}
}

func TestParseSample_KeyField(t *testing.T) {
	types := loadSample(t)
	author := findType(t, types, "Author")
	if author.Key != "AuthorId" {
		t.Errorf("Author.Key: want %q, got %q", "AuthorId", author.Key)
	}
	book := findType(t, types, "Book")
	if book.Key != "ISBN" {
		t.Errorf("Book.Key: want %q, got %q", "ISBN", book.Key)
	}
}

func TestParseSample_KeylessType(t *testing.T) {
	types := loadSample(t)
	tag := findType(t, types, "Tag")
	if tag.Key != "" {
		t.Errorf("Tag.Key: expected empty (keyless), got %q", tag.Key)
	}
}

func TestParseSample_DocField(t *testing.T) {
	types := loadSample(t)
	author := findType(t, types, "Author")
	if author.Doc == "" {
		t.Error("Author.Doc: expected non-empty")
	}
}

func TestParseSample_DocContinuation(t *testing.T) {
	types := loadSample(t)
	book := findType(t, types, "Book")
	// The %doc: for Book spans two lines joined by a continuation.
	if !strings.Contains(book.Doc, "foreign") {
		t.Errorf("Book.Doc continuation not joined: %q", book.Doc)
	}
	if strings.Contains(book.Doc, "\n") {
		t.Errorf("Book.Doc should not contain newline after continuation join: %q", book.Doc)
	}
}

func TestParseSample_MultivaluedField(t *testing.T) {
	types := loadSample(t)
	book := findType(t, types, "Book")
	// GEB has three Genre values.
	geb := book.Records[0]
	genres := geb.FieldValues("Genre")
	if len(genres) != 3 {
		t.Errorf("GEB Genre count: want 3, got %d: %v", len(genres), genres)
	}
	wantGenres := []string{"non-fiction", "mathematics", "philosophy"}
	for i, w := range wantGenres {
		if i >= len(genres) || genres[i] != w {
			t.Errorf("Genre[%d]: want %q, got %q", i, w, genres[i])
		}
	}
}

func TestParseSample_FieldNames_UniqueInOrder(t *testing.T) {
	types := loadSample(t)
	book := findType(t, types, "Book")
	// GEB record: ISBN, Title, Author, Genre (×3), Added, Notes
	geb := book.Records[0]
	names := geb.FieldNames()
	// Genre appears three times but FieldNames must deduplicate it.
	seen := make(map[string]int)
	for _, n := range names {
		seen[n]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("FieldNames: %q appears %d times, want 1", name, count)
		}
	}
	// Genre must appear before Added.
	genreIdx, addedIdx := -1, -1
	for i, n := range names {
		if n == "Genre" {
			genreIdx = i
		}
		if n == "Added" {
			addedIdx = i
		}
	}
	if genreIdx == -1 || addedIdx == -1 {
		t.Fatalf("expected Genre and Added in FieldNames, got %v", names)
	}
	if genreIdx >= addedIdx {
		t.Errorf("Genre (idx %d) should precede Added (idx %d)", genreIdx, addedIdx)
	}
}

func TestParseSample_FirstField(t *testing.T) {
	types := loadSample(t)
	author := findType(t, types, "Author")
	first := author.Records[0].FirstField()
	if first.Name != "AuthorId" {
		t.Errorf("FirstField.Name: want %q, got %q", "AuthorId", first.Name)
	}
}

func TestParseSample_MultiLineContinuation(t *testing.T) {
	types := loadSample(t)
	book := findType(t, types, "Book")
	// GEB's Notes field spans two lines.
	geb := book.Records[0]
	notes := geb.FieldValues("Notes")
	if len(notes) != 1 {
		t.Fatalf("Notes: expected 1 value, got %d", len(notes))
	}
	if !strings.Contains(notes[0], "formal systems") {
		t.Errorf("Notes continuation not joined: %q", notes[0])
	}
	if strings.Contains(notes[0], "\n") {
		t.Errorf("Notes should not contain newline after join: %q", notes[0])
	}
}

func TestParseSample_ForeignKeyMetadata(t *testing.T) {
	types := loadSample(t)
	book := findType(t, types, "Book")
	target, ok := book.ForeignKeys["Author"]
	if !ok {
		t.Fatal("Book.ForeignKeys: expected 'Author' entry")
	}
	if target != "Author" {
		t.Errorf("Book.ForeignKeys[Author]: want %q, got %q", "Author", target)
	}
	ft, ok := book.FieldTypes["Author"]
	if !ok {
		t.Fatal("Book.FieldTypes: expected 'Author' entry")
	}
	if ft.Kind != "rec" {
		t.Errorf("FieldTypes[Author].Kind: want %q, got %q", "rec", ft.Kind)
	}
	if ft.RecTarget != "Author" {
		t.Errorf("FieldTypes[Author].RecTarget: want %q, got %q", "Author", ft.RecTarget)
	}
}

func TestParseSample_EmailType(t *testing.T) {
	types := loadSample(t)
	author := findType(t, types, "Author")
	ft, ok := author.FieldTypes["Email"]
	if !ok {
		t.Fatal("Author.FieldTypes: expected 'Email' entry")
	}
	if ft.Kind != "email" {
		t.Errorf("FieldTypes[Email].Kind: want %q, got %q", "email", ft.Kind)
	}
}

func TestParseSample_URLType(t *testing.T) {
	types := loadSample(t)
	author := findType(t, types, "Author")
	ft, ok := author.FieldTypes["Website"]
	if !ok {
		t.Fatal("Author.FieldTypes: expected 'Website' entry")
	}
	if ft.Kind != "url" {
		t.Errorf("FieldTypes[Website].Kind: want %q, got %q", "url", ft.Kind)
	}
}

func TestParseSample_EnumType(t *testing.T) {
	types := loadSample(t)
	book := findType(t, types, "Book")
	ft, ok := book.FieldTypes["Genre"]
	if !ok {
		t.Fatal("Book.FieldTypes: expected 'Genre' entry")
	}
	if ft.Kind != "enum" {
		t.Errorf("FieldTypes[Genre].Kind: want %q, got %q", "enum", ft.Kind)
	}
	wantVals := []string{"fiction", "non-fiction", "essay", "philosophy", "mathematics", "science"}
	if len(ft.EnumVals) != len(wantVals) {
		t.Errorf("Genre EnumVals: want %v, got %v", wantVals, ft.EnumVals)
	}
}

func TestParseSample_UUIDType(t *testing.T) {
	types := loadSample(t)
	author := findType(t, types, "Author")
	ft, ok := author.FieldTypes["AuthorId"]
	if !ok {
		t.Fatal("Author.FieldTypes: expected 'AuthorId' entry")
	}
	if ft.Kind != "uuid" {
		t.Errorf("FieldTypes[AuthorId].Kind: want %q, got %q", "uuid", ft.Kind)
	}
}

func TestParseSample_DateType(t *testing.T) {
	types := loadSample(t)
	author := findType(t, types, "Author")
	ft, ok := author.FieldTypes["Born"]
	if !ok {
		t.Fatal("Author.FieldTypes: expected 'Born' entry")
	}
	if ft.Kind != "date" {
		t.Errorf("FieldTypes[Born].Kind: want %q, got %q", "date", ft.Kind)
	}
}

func TestParseReader_MalformedLine_ReturnsError(t *testing.T) {
	input := `%rec: Test

Name: Alice
this line has no colon
`
	_, err := ParseReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for malformed line, got nil")
	}
	// Error must mention a line number.
	if !strings.Contains(err.Error(), "line") {
		t.Errorf("error should mention line number: %v", err)
	}
}

func TestParseReader_EmptyFile(t *testing.T) {
	types, err := ParseReader(strings.NewReader(""))
	if err != nil {
		t.Fatalf("empty file: unexpected error: %v", err)
	}
	if len(types) != 0 {
		t.Errorf("empty file: expected 0 types, got %d", len(types))
	}
}

func TestParseReader_DefaultNamelessType(t *testing.T) {
	// Records before any %rec: directive fall into a nameless default type.
	input := `Name: Alice
Age: 30

Name: Bob
Age: 25
`
	types, err := ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(types) != 1 {
		t.Fatalf("expected 1 type (nameless default), got %d", len(types))
	}
	if types[0].Name != "" {
		t.Errorf("expected nameless type, got name %q", types[0].Name)
	}
	if len(types[0].Records) != 2 {
		t.Errorf("expected 2 records, got %d", len(types[0].Records))
	}
}

func TestParseReader_RangeType(t *testing.T) {
	input := `%rec: Item
%type: Rating range 1 5

Rating: 3
Name: Widget
`
	types, err := ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(types) == 0 {
		t.Fatal("expected at least one type")
	}
	ft, ok := types[0].FieldTypes["Rating"]
	if !ok {
		t.Fatal("expected FieldTypes entry for Rating")
	}
	if ft.Kind != "range" {
		t.Errorf("Kind: want %q, got %q", "range", ft.Kind)
	}
	if ft.RangeMin != 1 || ft.RangeMax != 5 {
		t.Errorf("Range: want [1,5], got [%d,%d]", ft.RangeMin, ft.RangeMax)
	}
}

func TestParseReader_MultipleTypes(t *testing.T) {
	input := `%rec: Foo

X: 1

%rec: Bar

Y: 2
Y: 3
`
	types, err := ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(types) != 2 {
		t.Fatalf("expected 2 types, got %d", len(types))
	}
	if types[0].Name != "Foo" || types[1].Name != "Bar" {
		t.Errorf("type names: want [Foo Bar], got %v", typeNames(types))
	}
}

func TestParseReader_DirectiveAfterDataRecord_Rejected(t *testing.T) {
	// A %key: directive appearing after a data record in the same type block
	// must be rejected per the GNU Recutils spec (directives precede records).
	input := `%rec: Item

Name: Alice

%key: Name
`
	_, err := ParseReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for directive after data record, got nil")
	}
	if !strings.Contains(err.Error(), "line") || !strings.Contains(err.Error(), "key") {
		t.Errorf("error should mention line number and directive name: %v", err)
	}
}

func TestParseReader_DirectiveAfterDataRecord_RecStillAllowed(t *testing.T) {
	// A %rec: directive after a data record is legal — it begins a new type
	// block. This is the sole exception to the "directives precede records"
	// rule enforced by the test above.
	input := `%rec: Foo

Name: Alice

%rec: Bar
%key: Id

Id: 1
`
	types, err := ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(types) != 2 {
		t.Fatalf("expected 2 types, got %d", len(types))
	}
	if types[1].Key != "Id" {
		t.Errorf("second type: expected Key %q, got %q", "Id", types[1].Key)
	}
}

func TestParseReader_CompoundKey_Rejected(t *testing.T) {
	// A compound %key: declaration (multiple tokens) must be rejected; the
	// recutils spec does not define compound primary keys in %key: (only in
	// %unique:), and silently storing "Field1 Field2" as a single key string
	// matches no field.
	input := `%rec: Pair
%key: Field1 Field2

Field1: a
Field2: b
`
	_, err := ParseReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for compound %key:, got nil")
	}
	if !strings.Contains(err.Error(), "key") {
		t.Errorf("error should mention %%key:: %v", err)
	}
}

func TestParseReader_RegexpType_PreservesPattern(t *testing.T) {
	// %type: Field regexp /pat/ must populate FieldType.Pattern with the
	// text between the `/` delimiters.
	input := `%rec: Item
%type: Code regexp /^[A-Z]{3}-[0-9]+$/

Code: ABC-123
`
	types, err := ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(types) == 0 {
		t.Fatal("expected at least one type")
	}
	ft, ok := types[0].FieldTypes["Code"]
	if !ok {
		t.Fatal("expected FieldTypes entry for Code")
	}
	if ft.Kind != "regexp" {
		t.Errorf("Kind: want %q, got %q", "regexp", ft.Kind)
	}
	if ft.Pattern != "^[A-Z]{3}-[0-9]+$" {
		t.Errorf("Pattern: want %q, got %q", "^[A-Z]{3}-[0-9]+$", ft.Pattern)
	}
}

func TestParseReader_RegexpType_MalformedPattern_Rejected(t *testing.T) {
	// A %type: regexp directive with a missing closing `/` must be rejected.
	input := `%rec: Item
%type: Code regexp /unterminated

Code: whatever
`
	_, err := ParseReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for malformed regexp pattern, got nil")
	}
	if !strings.Contains(err.Error(), "regexp") {
		t.Errorf("error should mention regexp: %v", err)
	}
}

func TestRecord_FirstField_EmptyRecord(t *testing.T) {
	r := Record{}
	f := r.FirstField()
	if f.Name != "" || f.Value != "" {
		t.Errorf("FirstField on empty record: expected zero Field, got %+v", f)
	}
}
