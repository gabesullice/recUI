package server

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gabesullice/recui/pkg/config"
	"github.com/gabesullice/recui/pkg/recfile"
)

// GenerateConfig holds the configuration for static site generation.
type GenerateConfig struct {
	RecfilePath string
	UIConfig    config.UIConfig
	OutputDir   string
}

// Generate produces a static website from a recfile. It writes HTML pages for
// the home, type listings, and individual records, plus static assets (CSS,
// fonts), into OutputDir. All internal links use relative URLs so the output
// works on any static host regardless of the base path.
func Generate(cfg GenerateConfig) error {
	types, err := recfile.ParseFile(cfg.RecfilePath)
	if err != nil {
		return fmt.Errorf("parsing recfile: %w", err)
	}
	// Write static assets from the embedded FS.
	if err := copyEmbeddedFile("style.css", filepath.Join(cfg.OutputDir, "style.css")); err != nil {
		return err
	}
	if err := copyEmbeddedFile("vendor/dm-mono.woff2", filepath.Join(cfg.OutputDir, "vendor", "dm-mono.woff2")); err != nil {
		return err
	}
	// Home page (depth 0).
	rw := relativeRewriter("")
	data := buildHomeData(types, cfg.UIConfig, rw)
	if err := writeTemplate(filepath.Join(cfg.OutputDir, "index.html"), "home", data); err != nil {
		return err
	}
	// Type and record pages.
	for _, rt := range types {
		if !cfg.UIConfig[rt.Name].Browseable() {
			continue
		}
		escapedType := url.PathEscape(rt.Name)
		typeDir := filepath.Join(cfg.OutputDir, "types", escapedType)
		// Type page lives at types/{Type}/index.html (depth 2).
		rw = relativeRewriter("types/" + escapedType)
		td := buildTypeData(rt, types, cfg.UIConfig, rw)
		if err := writeTemplate(filepath.Join(typeDir, "index.html"), "type-list", td); err != nil {
			return err
		}
		for i, rec := range rt.Records {
			key := recordID(rt, i, rec)
			escapedKey := url.PathEscape(key)
			recDir := filepath.Join(typeDir, escapedKey)
			// Record page lives at types/{Type}/{Key}/index.html (depth 3).
			rw = relativeRewriter("types/" + escapedType + "/" + escapedKey)
			rd := buildRecordData(rt, i, rec, types, cfg.UIConfig, rw)
			if err := writeTemplate(filepath.Join(recDir, "index.html"), "record", rd); err != nil {
				return err
			}
		}
	}
	return nil
}

// relativeRewriter returns a URL rewriter that converts absolute URL paths
// (e.g. "/types/Book") into relative paths from the page at fromDir
// (e.g. "../../types/Book" when fromDir is "types/Author").
func relativeRewriter(fromDir string) func(string) string {
	return func(absURL string) string {
		rel := strings.TrimPrefix(absURL, "/")
		if fromDir == "" {
			if rel == "" {
				return "./"
			}
			return rel
		}
		depth := strings.Count(fromDir, "/") + 1
		prefix := strings.Repeat("../", depth)
		return prefix + rel
	}
}

// writeTemplate renders the named template with data and writes it to path,
// creating parent directories as needed.
func writeTemplate(path, name string, data any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", path, err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()
	if err := templates.ExecuteTemplate(f, name, data); err != nil {
		return fmt.Errorf("rendering %s: %w", name, err)
	}
	return nil
}

// copyEmbeddedFile copies a file from the embedded web/ FS to a destination
// path on disk, creating parent directories as needed.
func copyEmbeddedFile(name, dest string) error {
	content, err := fs.ReadFile(webFS, name)
	if err != nil {
		return fmt.Errorf("reading embedded %s: %w", name, err)
	}
	return writeFile(dest, content)
}

// writeFile writes content to path, creating parent directories as needed.
func writeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
