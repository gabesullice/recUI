package recfile

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// ParseFile reads and parses a recfile from disk.
func ParseFile(path string) ([]RecordType, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseReader(f)
}

// ParseReader parses a recfile from an io.Reader. Pure — no I/O side effects
// beyond reading from r.
func ParseReader(r io.Reader) ([]RecordType, error) {
	scanner := bufio.NewScanner(r)
	// current accumulates the type block being built.
	current := &RecordType{
		ForeignKeys: make(map[string]string),
		FieldTypes:  make(map[string]FieldType),
	}
	// pendingRecord accumulates fields for the record currently being parsed.
	var pendingRecord []Field
	// inSchema is true while we are still in the directive section (before
	// any data record has started in the current type block).
	inSchema := true
	var result []*RecordType
	lineNum := 0

	// flush commits pendingRecord into current if it is non-empty.
	flush := func() {
		if len(pendingRecord) == 0 {
			return
		}
		current.Records = append(current.Records, Record{fields: pendingRecord})
		pendingRecord = nil
		inSchema = false
	}

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		// Blank line: end of a record or schema block boundary.
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}

		// Comment: silently skip.
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Directive line.
		if strings.HasPrefix(line, "%") {
			colon := strings.Index(line, ":")
			if colon == -1 {
				return nil, fmt.Errorf("line %d: malformed directive (no colon): %q", lineNum, line)
			}
			directive := strings.TrimSpace(line[1:colon])
			value := strings.TrimSpace(line[colon+1:])
			// A directive after a data record has started is an error: per
			// the GNU Recutils spec, directives must precede all data records
			// within a type block. %rec: is the sole exception — it begins a
			// new type block and therefore resets the schema section.
			if !inSchema && directive != "rec" {
				return nil, fmt.Errorf("line %d: directive %%%s: not allowed after a data record in the same type block", lineNum, directive)
			}

			switch directive {
			case "rec":
				// Start of a new type block. Flush any pending record into the
				// current type, then save current and start fresh.
				flush()
				result = append(result, current)
				current = &RecordType{
					Name:        value,
					ForeignKeys: make(map[string]string),
					FieldTypes:  make(map[string]FieldType),
				}
				inSchema = true
			case "key":
				// %key: takes a single field name. Compound keys are not
				// defined by the recutils spec (unlike %unique:) — reject
				// whitespace or multi-token values explicitly rather than
				// silently storing a value that matches no field.
				keyFields := strings.Fields(value)
				if len(keyFields) == 0 {
					return nil, fmt.Errorf("line %d: %%key: requires a field name", lineNum)
				}
				if len(keyFields) > 1 {
					return nil, fmt.Errorf("line %d: %%key: requires a single field name, got %d tokens: %q", lineNum, len(keyFields), value)
				}
				current.Key = keyFields[0]
			case "doc":
				current.Doc = value
			case "type":
				ft, fieldName, err := parseTypeDirective(value, lineNum)
				if err != nil {
					return nil, err
				}
				current.FieldTypes[fieldName] = ft
				if ft.Kind == "rec" {
					current.ForeignKeys[fieldName] = ft.RecTarget
				}
			case "mandatory":
				current.Mandatory = append(current.Mandatory, strings.Fields(value)...)
			case "unique":
				current.Unique = append(current.Unique, strings.Fields(value)...)
			case "allowed", "prohibit", "sort", "size", "auto", "confidential", "typedef", "constraint":
				// Recognised but not needed — skip silently.
			default:
				// Unknown directives are silently skipped per the recutils spec.
			}
			continue
		}

		// Continuation line for a multi-line value.
		if strings.HasPrefix(line, "+ ") || line == "+" {
			cont := ""
			if strings.HasPrefix(line, "+ ") {
				cont = strings.TrimSpace(line[2:])
			}
			// Attach to the last field in pendingRecord if present,
			// otherwise this is a %doc: continuation.
			if len(pendingRecord) > 0 {
				last := &pendingRecord[len(pendingRecord)-1]
				if cont != "" {
					last.Value = last.Value + " " + cont
				}
			} else if inSchema {
				// Continuation of %doc: or another schema directive value.
				// We only support %doc: continuation in practice.
				if cont != "" {
					current.Doc = current.Doc + " " + cont
				}
			} else {
				return nil, fmt.Errorf("line %d: unexpected continuation line", lineNum)
			}
			continue
		}

		// Field line: Name: Value
		colon := strings.Index(line, ":")
		if colon == -1 || colon == 0 {
			return nil, fmt.Errorf("line %d: expected 'Name: Value' or blank line, got: %q", lineNum, line)
		}
		name := line[:colon]
		// Validate: field names must not contain spaces.
		if strings.ContainsAny(name, " \t") {
			return nil, fmt.Errorf("line %d: field name contains whitespace: %q", lineNum, name)
		}
		value := strings.TrimSpace(line[colon+1:])
		pendingRecord = append(pendingRecord, Field{Name: name, Value: value})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Flush any record in progress at EOF.
	flush()

	// Append the last type block.
	result = append(result, current)

	// Convert []*RecordType to []RecordType, skipping empty nameless types
	// that arose from a file with no leading default records.
	out := make([]RecordType, 0, len(result))
	for _, rt := range result {
		if rt.Name == "" && len(rt.Records) == 0 {
			continue
		}
		out = append(out, *rt)
	}

	return out, nil
}

// parseTypeDirective parses the value portion of a %type: directive and
// returns the field name and FieldType. lineNum is used in error messages.
func parseTypeDirective(value string, lineNum int) (FieldType, string, error) {
	parts := strings.Fields(value)
	if len(parts) < 2 {
		return FieldType{}, "", fmt.Errorf("line %d: %%type: directive requires at least field name and kind: %q", lineNum, value)
	}
	fieldName := parts[0]
	kind := strings.ToLower(parts[1])
	rest := parts[2:]

	ft := FieldType{Kind: kind}
	switch kind {
	case "rec":
		if len(rest) < 1 {
			return FieldType{}, "", fmt.Errorf("line %d: %%type: rec requires a target type name", lineNum)
		}
		ft.RecTarget = rest[0]
	case "enum":
		ft.EnumVals = rest
	case "range":
		if len(rest) < 2 {
			return FieldType{}, "", fmt.Errorf("line %d: %%type: range requires min and max", lineNum)
		}
		min, err := strconv.Atoi(rest[0])
		if err != nil {
			return FieldType{}, "", fmt.Errorf("line %d: %%type: range min is not an integer: %q", lineNum, rest[0])
		}
		max, err := strconv.Atoi(rest[1])
		if err != nil {
			return FieldType{}, "", fmt.Errorf("line %d: %%type: range max is not an integer: %q", lineNum, rest[1])
		}
		ft.RangeMin = min
		ft.RangeMax = max
	case "regexp":
		// Extract the pattern text between the `/` delimiters. The pattern
		// may contain spaces, so work from the original `value` (not
		// strings.Fields) once we have located the first `/`.
		open := strings.Index(value, "/")
		if open == -1 {
			return FieldType{}, "", fmt.Errorf("line %d: %%type: regexp requires a /pattern/ delimited by slashes: %q", lineNum, value)
		}
		close := strings.LastIndex(value, "/")
		if close == open {
			return FieldType{}, "", fmt.Errorf("line %d: %%type: regexp pattern is missing its closing '/' delimiter: %q", lineNum, value)
		}
		ft.Pattern = value[open+1 : close]
	case "int", "real", "bool", "date", "email", "url", "uuid", "line":
		// no extra arguments
	default:
		// Unknown type kinds are preserved as-is (forward compatibility).
	}

	return ft, fieldName, nil
}
