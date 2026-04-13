# recUI

_recUI_ is a read-only web UI for exploring
[GNU recutils](https://www.gnu.org/software/recutils/) recfiles in a browser.
Point it at any valid recfile and it renders a navigable hyperdocument — a home
page listing record types, type pages listing records, and record pages with
field values and prev/next navigation. It can run as a local dev server or
generate a static site for any host.

## Building

Requires **Go 1.22+**, **make**, and **curl**.

```sh
make vendor   # download the DM Mono font (once)
make build    # compile the binary
```

## Usage

### `serve` — local dev server

```sh
./recui serve path/to/file.rec
```

Binds to `127.0.0.1:8080`. The recfile is polled for changes and re-parsed
automatically.

| Flag | Short | Default | Description |
| --- | --- | --- | --- |
| `--port` | `-p` | `8080` | TCP port to listen on. |
| `--config` | `-c` | _(none)_ | Path to a TOML [display config](docs/configuration.md) file. |
| `--web-dir` | `-w` | _(embedded)_ | Serve static assets from disk instead of the embedded FS. See [development](docs/development.md). |

### `generate` — static site

```sh
./recui generate path/to/file.rec
./recui generate -o public path/to/file.rec
```

Writes a self-contained site to the output directory, ready to deploy to
GitHub Pages, Netlify, or any static host. All internal links use relative
URLs, so the output works at any base path without extra configuration.

| Flag | Short | Default | Description |
| --- | --- | --- | --- |
| `--output` | `-o` | `site` | Output directory. Created if it does not exist. |
| `--config` | `-c` | _(none)_ | Path to a TOML [display config](docs/configuration.md) file. |

## Further reading

- [Configuration reference](docs/configuration.md) — customizing titles, field
  layout, and type visibility.
- [Development](docs/development.md) — contributing, architecture, and hot
  reload.
