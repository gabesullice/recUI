# Development

## Iterating on templates and CSS

To edit CSS and HTML templates without rebuilding the Go binary, point
`--web-dir` at the source tree:

```sh
./recui serve --web-dir ./web ./testdata/sample.rec
```

Edits to `web/style.css` and `web/*.html` are picked up on the next browser
refresh. Go code changes still require a rebuild (`make build`).

## Vendored assets

The only external runtime asset is the DM Mono font, fetched into
`web/vendor/` (gitignored) by `make vendor`. The Makefile pins an exact
version; updating the font means bumping the URL in the Makefile and
re-running `make vendor`.

All page rendering is server-side Go `html/template` — there is no
JavaScript bundle, module graph, or importmap to vendor.

## Hot reload

The `serve` command polls the recfile's mtime every two seconds. When the
file changes on disk, recui re-parses it and serves the updated data on the
next request. If re-parsing fails, the previous state is retained and a
warning is logged. There is no `--watch` flag — hot reload is always on.

The TOML config file is loaded once at startup and is not hot-reloaded;
restart the server to pick up config changes.

## Architecture

```
recui/
├── cmd/recui/        — Cobra CLI entry point (flags, slog setup, signal handling)
├── pkg/recfile/      — recfile parser: tokens, records, types, field metadata
├── pkg/config/       — TOML display-config loader and strict validator
├── pkg/server/       — HTTP server, routing, handlers, rendering, JSON:API,
│                       and static site generation
└── web/              — CSS, HTML templates, and vendored font
```

The server renders every page with Go `html/template` — there is no
client-side framework. Hot reload is a two-second mtime poll: a per-request
snapshot reference is swapped atomically when the file changes on disk, so
handlers always see a consistent view. A JSON:API representation is served
alongside HTML via `Accept`-header content negotiation, letting the same URL
answer either `text/html` or `application/vnd.api+json`.

The `generate` command reuses the same template data-building functions as
`serve`, writing each page to an `index.html` file under the corresponding
URL path.
