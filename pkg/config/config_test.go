package config

import (
	"strings"
	"testing"
)

func TestLoadConfig_Valid(t *testing.T) {
	cfg, err := LoadConfig("testdata/valid.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	book, ok := cfg["Book"]
	if !ok {
		t.Fatal("expected type Book in config")
	}
	if book.Title != "Title" {
		t.Errorf("Title: got %q, want %q", book.Title, "Title")
	}
	if len(book.FieldOrder) != 5 {
		t.Errorf("FieldOrder len: got %d, want 5", len(book.FieldOrder))
	}
	if book.FieldOrder[0] != "Title" {
		t.Errorf("FieldOrder[0]: got %q, want %q", book.FieldOrder[0], "Title")
	}
	if len(book.Fields) != 4 {
		t.Errorf("Fields len: got %d, want 4", len(book.Fields))
	}
	if book.Fields["Author"].Label != "inline" {
		t.Errorf("Author.Label: got %q, want %q", book.Fields["Author"].Label, "inline")
	}
	if book.Fields["Genre"].ListFormat != "inline" {
		t.Errorf("Genre.ListFormat: got %q, want %q", book.Fields["Genre"].ListFormat, "inline")
	}
	if book.Fields["Genre"].Sep != ", " {
		t.Errorf("Genre.Sep: got %q, want %q", book.Fields["Genre"].Sep, ", ")
	}
	if !book.Fields["InternalID"].Exclude {
		t.Error("InternalID.Exclude: got false, want true")
	}
}

func TestLoadConfig_UnknownKey(t *testing.T) {
	_, err := LoadConfig("testdata/unknown_key.toml")
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
}

func TestLoadConfig_BadLabel(t *testing.T) {
	_, err := LoadConfig("testdata/bad_label.toml")
	if err == nil {
		t.Fatal("expected error for invalid label, got nil")
	}
}

func TestLoadConfig_BadListFormat(t *testing.T) {
	_, err := LoadConfig("testdata/bad_list_format.toml")
	if err == nil {
		t.Fatal("expected error for invalid list_format, got nil")
	}
}

func TestLoadConfig_SepNoInline(t *testing.T) {
	_, err := LoadConfig("testdata/sep_no_inline.toml")
	if err == nil {
		t.Fatal("expected error for sep without list_format=inline, got nil")
	}
}

func TestLoadConfig_EmptyPath(t *testing.T) {
	_, err := LoadConfig("")
	if err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
}

func TestLoadConfig_TemplateTitle_Valid(t *testing.T) {
	cfg, err := LoadConfig("testdata/valid_template_title.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	book, ok := cfg["Book"]
	if !ok {
		t.Fatal("expected type Book in config")
	}
	tmpl := book.TitleTemplate()
	if tmpl == nil {
		t.Fatal("TitleTemplate(): got nil, want parsed template")
	}
	// The template should execute against a simple map.
	var buf strings.Builder
	if err := tmpl.Execute(&buf, map[string]string{"Id": "E-001", "Name": "Demo"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got, want := buf.String(), "E-001. Demo"; got != want {
		t.Errorf("rendered title: got %q, want %q", got, want)
	}
}

func TestLoadConfig_TemplateTitle_Malformed(t *testing.T) {
	_, err := LoadConfig("testdata/bad_template_title.toml")
	if err == nil {
		t.Fatal("expected error for malformed title template, got nil")
	}
	if !strings.Contains(err.Error(), "Book") {
		t.Errorf("error should name offending type %q, got: %v", "Book", err)
	}
	if !strings.Contains(err.Error(), "title") {
		t.Errorf("error should mention the title field, got: %v", err)
	}
}

func TestLoadConfig_PlainFieldTitle(t *testing.T) {
	cfg, err := LoadConfig("testdata/plain_field_title.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	book, ok := cfg["Book"]
	if !ok {
		t.Fatal("expected type Book in config")
	}
	if book.Title != "Name" {
		t.Errorf("Title: got %q, want %q", book.Title, "Name")
	}
	if book.TitleTemplate() != nil {
		t.Error("TitleTemplate(): got non-nil, want nil for plain field-name title")
	}
}

func TestLoadConfig_Browse(t *testing.T) {
	cfg, err := LoadConfig("testdata/valid.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg["Book"].Browseable() {
		t.Error("Book: expected Browseable() == true when browse not set")
	}
	junction, ok := cfg["Junction"]
	if !ok {
		t.Fatal("expected type Junction in config")
	}
	if junction.Browseable() {
		t.Error("Junction: expected Browseable() == false when browse = false")
	}
}
