package recfile

// RecordType represents a typed group of records in a recfile, defined by a
// %rec: directive. The zero value (Name == "") represents the nameless default
// type for records that appear before any %rec: directive.
type RecordType struct {
	Name        string
	Key         string            // from %key:; empty if not declared
	Doc         string            // from %doc:; empty if not declared
	Mandatory   []string          // field names from %mandatory: directives
	Unique      []string          // field names from %unique: directives
	ForeignKeys map[string]string // field name → target record type name
	FieldTypes  map[string]FieldType
	Records     []Record
}

// FieldType captures the parsed type metadata from a %type: directive.
type FieldType struct {
	Kind      string   // "int", "real", "bool", "rec", "date", "email", "url",
	// "enum", "uuid", "range", "regexp", "line"
	RecTarget string   // populated when Kind == "rec"
	EnumVals  []string // populated when Kind == "enum"
	RangeMin  int      // populated when Kind == "range"
	RangeMax  int      // populated when Kind == "range"
	Pattern   string   // populated when Kind == "regexp"; the pattern text
	// between the `/` delimiters, not compiled or validated here.
}

// Record holds an ordered list of fields. Use the accessor methods to query
// field values; the fields slice is unexported to prevent external mutation.
type Record struct {
	fields []Field
}

// Field is a single name/value pair from a recfile record.
type Field struct {
	Name  string
	Value string
}

// FieldValues returns all values for the given field name in declaration order.
func (r Record) FieldValues(name string) []string {
	var vals []string
	for _, f := range r.fields {
		if f.Name == name {
			vals = append(vals, f.Value)
		}
	}
	return vals
}

// FieldNames returns unique field names in first-occurrence order.
func (r Record) FieldNames() []string {
	seen := make(map[string]bool)
	var names []string
	for _, f := range r.fields {
		if !seen[f.Name] {
			seen[f.Name] = true
			names = append(names, f.Name)
		}
	}
	return names
}

// FirstField returns the first field in the record. Returns a zero Field if
// the record has no fields.
func (r Record) FirstField() Field {
	if len(r.fields) == 0 {
		return Field{}
	}
	return r.fields[0]
}
