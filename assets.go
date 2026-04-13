// Package assets embeds the web/ directory for use by the server package.
package assets

import "embed"

// FS is the embedded web/ directory. Files are accessible as "web/style.css",
// "web/home.html", etc.
//
//go:embed all:web
var FS embed.FS
