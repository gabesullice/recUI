# Configuration

Without `--config`, recui renders every type and field using built-in
defaults. A TOML config file customizes what appears on each page, how record
titles are rendered, and which types are reachable via navigation.

Unknown keys are rejected at load time, so typos surface as immediate startup
errors rather than silent no-ops.

## File format

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

## Per-type options

Set under `[type.TypeName]`.

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `browse` | bool | `true` | When `false`, the type is link-only: no home listing, no collection page, no prev/next navigation. Records are still reachable via direct URL from other records that point at them. |
| `title` | string | _(none)_ | Either a plain field name (e.g. `"Name"`) whose value becomes the record title, or a Go `text/template` expression (e.g. `"{{.Id}}. {{.Name}}"`). Templates are parsed at config-load time — a malformed template is a startup error. |
| `field_order` | `[]string` | _(none)_ | Field names that should appear first on the record page, in this order. Fields not listed here follow in their original declaration order. |

## Per-field options

Set under `[type.TypeName.fields.FieldName]` (or inline as
`[type.TypeName.fields]` with `FieldName.option = value`).

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `exclude` | bool | `false` | When `true`, the field is omitted from the record detail page entirely. Useful for fields already surfaced in the title. |
| `label` | `"h3"` \| `"inline"` \| `"omit"` | `"h3"` | How the field name is presented. `h3` is a heading above the value; `inline` puts the label and value on one line; `omit` renders the value with no label at all. |
| `list_format` | `"ul"` \| `"ol"` \| `"inline"` | `"ul"` | For multivalued fields, how the values are laid out — bulleted list, numbered list, or a single run joined by `sep`. |
| `sep` | string | `", "` | Separator used when `list_format = "inline"`. Setting `sep` without `list_format = "inline"` is a config error. |

## Title templates

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

Template output flows into fields rendered by `html/template` downstream,
which auto-escapes string values. This is why it is safe to use
`text/template` here even though the input is user-controlled via the TOML
config.

## Record-title fallback chain

When `title` is absent from the config — or is a plain field name that the
record does not contain, or is a template whose execution errors — recui
picks a title using the first rule that matches:

1. Well-known field names, in order: `Title`, `Label`, `Name`, `ID`.
2. The first field that is both `%unique:` and `%mandatory:` (in record
   field order).
3. The first `%unique:` field (in record field order).
4. The first declared field of the record.
