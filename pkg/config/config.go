package config

import (
	"fmt"
	"strings"
	texttmpl "text/template"

	"github.com/BurntSushi/toml"
)

// UIConfig maps record type names to their display configuration.
type UIConfig map[string]TypeConfig

type TypeConfig struct {
	Title      string                 `toml:"title"`
	FieldOrder []string               `toml:"field_order"`
	Browse     *bool                  `toml:"browse"` // nil = browseable (default); false = link-only (no collection page, no home listing, no nav)
	Fields     map[string]FieldConfig `toml:"fields"`
	// titleTmpl is the parsed Go text/template for Title when Title contains
	// template syntax ("{{"). It is populated by LoadConfig and is nil when
	// Title is absent or is a plain field-name reference. Unexported so TOML
	// decoding cannot touch it.
	titleTmpl *texttmpl.Template
}

// Browseable reports whether this type should have a collection page and appear
// in the home listing and navigation. Returns true when Browse is nil (default).
func (tc TypeConfig) Browseable() bool {
	return tc.Browse == nil || *tc.Browse
}

// TitleTemplate returns the pre-parsed title template for this type, or nil if
// the configured Title is empty or is a plain field-name reference (i.e. does
// not contain "{{"). The template is parsed once at config-load time by
// LoadConfig; render-time code must not re-parse Title.
func (tc TypeConfig) TitleTemplate() *texttmpl.Template {
	return tc.titleTmpl
}

type FieldConfig struct {
	Exclude    bool   `toml:"exclude"`
	Label      string `toml:"label"`       // "h3" | "inline" | "omit" | "" (default: "h3")
	ListFormat string `toml:"list_format"` // "ul" | "ol" | "inline" | "" (default: "ul")
	Sep        string `toml:"sep"`         // default: ", "
}

type rawFile struct {
	Type map[string]TypeConfig `toml:"type"`
}

// LoadConfig reads a TOML config file and returns a validated UIConfig.
func LoadConfig(path string) (UIConfig, error) {
	var raw rawFile
	meta, err := toml.DecodeFile(path, &raw)
	if err != nil {
		return nil, fmt.Errorf("config: decode %q: %w", path, err)
	}
	// Reject unknown keys so typos in config files surface immediately.
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("config: unknown keys in %q: %s", path, strings.Join(keys, ", "))
	}
	// Validate each type's field configs and pre-parse any title templates.
	// Templates are parsed here (not per-request) so that malformed template
	// syntax is a startup error instead of a silent render-time fallback.
	validLabels := map[string]bool{"": true, "h3": true, "inline": true, "omit": true}
	validListFormats := map[string]bool{"": true, "ul": true, "ol": true, "inline": true}
	for typeName, tc := range raw.Type {
		for fieldName, fc := range tc.Fields {
			if !validLabels[fc.Label] {
				return nil, fmt.Errorf("config: type %q field %q: invalid label %q (must be h3, inline, omit, or empty)", typeName, fieldName, fc.Label)
			}
			if !validListFormats[fc.ListFormat] {
				return nil, fmt.Errorf("config: type %q field %q: invalid list_format %q (must be ul, ol, inline, or empty)", typeName, fieldName, fc.ListFormat)
			}
			if fc.Sep != "" && fc.ListFormat != "inline" {
				return nil, fmt.Errorf("config: type %q field %q: sep requires list_format = \"inline\"", typeName, fieldName)
			}
		}
		if strings.Contains(tc.Title, "{{") {
			tmpl, err := texttmpl.New(typeName + "/title").Parse(tc.Title)
			if err != nil {
				return nil, fmt.Errorf("config: type %q: parse title template: %w", typeName, err)
			}
			tc.titleTmpl = tmpl
			raw.Type[typeName] = tc
		}
	}
	return UIConfig(raw.Type), nil
}
