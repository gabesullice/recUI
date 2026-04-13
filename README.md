# recUI

_recUI_ is a read-only, local web UI for exploring
[GNU recutils](https://www.gnu.org/software/recutils/) recfiles in a browser.
Point it at any valid recfile and it renders a navigable hyperdocument with
three page types: a home page listing record types, a type page listing records,
and a record page showing field values with prev/next navigation.

The tool is generic — it works with any recfile.

## Prerequisites

- **Go 1.22+**
- **make** and **curl** — used by `make vendor` to download the DM Mono font.

## Vendored Assets

The only external runtime asset is the DM Mono font, which is fetched into
`web/vendor/` (gitignored) by `make vendor`. The Makefile pins an exact
version; updating the font means bumping the version variable in the Makefile
and re-running `make vendor`. Run `make vendor` once before the first build.
All page rendering is performed server-side by Go `html/template` — there is
no JavaScript bundle, module graph, or importmap to vendor.

## Building

```sh
make vendor   # download the DM Mono font (once, or after a version bump)
make build    # compile the binary
```

This produces a `recui` binary in the tool's root directory.

## Running

After building, serve any recfile:

```sh
./recui serve path/to/file.rec
```

The server binds to `127.0.0.1:8080` and logs a ready line to stderr:

```
recui ready  addr=http://127.0.0.1:8080  recfile=./data.rec  types=3  records=47
```

Open the logged URL in a browser.

### CLI Flags

| Flag | Short | Default | Description |
| --- | --- | --- | --- |
| `--port` | `-p` | `8080` | TCP port to listen on. |
| `--web-dir` | `-w` | _(embedded)_ | Serve static assets from a disk path instead of the embedded filesystem. Development-only — lets you edit CSS and HTML templates and refresh without rebuilding. |
| `--config` | `-c` | _(none)_ | Path to a TOML display config file. Without this flag the tool uses built-in defaults for every type and field. See [Configuration](#configuration). |

## Pages

**Home** — lists every record type in the file with its record count. If a type
has a `%doc:` descriptor, it appears below the type name.

**Type** — lists all records of a type. Each record's title (its first field
value) is shown as a link.

**Record** — displays all fields of a single record. Foreign key fields
(`%type: ... rec ...`) render as links to the referenced record. A footer
provides prev/next navigation between records in the same type.

## Hot Reload

The server polls the recfile's mtime every two seconds. When the file changes
on disk, recui re-parses it and serves the updated data on the next request. If
re-parsing fails, the previous state is retained and a warning is logged. There
is no `--watch` flag — hot reload is always on. The TOML config file is loaded
once at startup and is not hot-reloaded; restart the server to pick up config
changes.

## Architecture at a Glance

```
recui/
├── cmd/recui/        — Cobra CLI entry point (flags, slog setup, signal handling)
├── pkg/recfile/      — recfile parser: tokens, records, types, field metadata
├── pkg/config/       — TOML display-config loader and strict validator
├── pkg/server/       — HTTP server, routing, handlers, rendering, JSON:API
└── web/              — CSS, HTML templates, and vendored font (served by server)
```

The server renders every page with Go `html/template` — there is no
client-side framework. Hot reload is a simple two-second mtime poll: a
per-request snapshot reference is swapped out under a mutex when the file
changes on disk, so handlers always see a consistent view. A JSON:API
representation is served alongside HTML via `Accept`-header content
negotiation, letting the same URL answer either `text/html` or
`application/vnd.api+json`.

## Configuration

Without `--config`, recui renders every type and field using built-in
defaults. A TOML config file customizes what appears on each page, how record
titles are rendered, and which types are reachable via navigation.

### File format

The config file is a TOML document with one section per record type and
optional subsections per field:

```toml
[type.Book]
title = "Title"
field_order = ["Title", "Author", "Genre", "Notes"]

[type.Book.fields]
Author.label = "inline"
Genre.list_format = "inline"
Genre.sep = ", "

[type.Tag]
browse = false
```

In this example:

- `Book` records use the `Title` field as the record title, pin key fields
  to the top of the detail page, render `Author` inline (label and value on
  one line), and join multiple `Genre` values with commas instead of a list.
- `Tag` records are reachable by direct link but do not appear on the home
  page, do not get a collection page, and do not participate in prev/next
  navigation.

Unknown keys are rejected at load time, so typos and stale options surface as
an immediate startup error rather than as silent no-ops.

### Per-type options

Set under `[type.TypeName]`.

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `browse` | bool | `true` | When `false`, the type is link-only: no home listing, no collection page, no prev/next navigation. Records are still reachable via direct URL from other records that point at them. |
| `title` | string | _(none)_ | Either a plain field name (e.g. `"Name"`) whose value becomes the record title, or a Go `text/template` expression (e.g. `"{{.Id}}. {{.Name}}"`). Templates are parsed at config-load time — a malformed template is a startup error, not a silent runtime fallback. |
| `field_order` | `[]string` | _(none)_ | Field names that should appear first on the record page, in this order. Fields not listed here follow in their original declaration order. |

### Per-field options

Set under `[type.TypeName.fields.FieldName]` (or inline as
`[type.TypeName.fields]` with `FieldName.option = value`).

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `exclude` | bool | `false` | When `true`, the field is omitted from the record detail page entirely. Useful for fields already surfaced in the title. |
| `label` | `"h3"` \| `"inline"` \| `"omit"` | `"h3"` | How the field name is presented. `h3` is a heading above the value; `inline` puts the label and value on one line; `omit` renders the value with no label at all. |
| `list_format` | `"ul"` \| `"ol"` \| `"inline"` | `"ul"` | For multivalued fields, how the values are laid out — bulleted list, numbered list, or a single run joined by `sep`. |
| `sep` | string | `", "` | Separator used when `list_format = "inline"`. Setting `sep` without `list_format = "inline"` is a config error. |

### Title templates

The `title` option accepts Go
[`text/template`](https://pkg.go.dev/text/template) syntax. The template is
executed against a `map[string]string` of the record's field values (first
value per field), so field names become dot-prefixed keys:

```toml
[type.Author]
title = "{{.Name}} ({{.Born}})"
```

Templates are parsed once by `config.LoadConfig` at startup, so a malformed
template (e.g. `"{{.Id}."`) fails fast with a clear error rather than
silently falling back at render time. If execution of a valid template fails
for a specific record (e.g. the record is missing a referenced field), recui
falls through to the record-title fallback chain described below.

**Escape contract.** Template output flows into fields that are rendered by
`html/template` downstream, which auto-escapes string values. That is why it
is safe to use `text/template` here even though the input is
user-controlled via the TOML config.

### Record-title fallback chain

When `title` is absent from the config — or is a plain field name that the
record does not contain, or is a template whose execution errors — recui
picks a title using the first rule that matches:

1. Well-known field names, in order: `Title`, `Label`, `Name`, `ID`.
2. The first field that is both `%unique:` and `%mandatory:` (in record
   field order).
3. The first `%unique:` field (in record field order).
4. The first declared field of the record.

## Development

To iterate on CSS and HTML templates without rebuilding the Go binary, point
`--web-dir` at the source tree:

```sh
./recui serve --web-dir ./web ./testdata/sample.rec
```

Edits to `web/style.css` and `web/*.html` are picked up on the next request.
Go code changes still require a rebuild (`make build`). All page markup is
rendered server-side by Go `html/template`; there is no JavaScript bundle to
reload.
