# tpix machine-readable output schema

`tpix` can emit machine-readable output so external tools can consume structured data 
instead of parsing human text.

```bash
tpix search cetz              # default, human-readable
tpix search cetz --json       # exactly one JSON document on stdout
```

`--json` is a global (persistent) boolean flag and can appear before or after
the subcommand: both `tpix --json search cetz` and `tpix search cetz --json`
work. 

## Design principles

1. **`--json` emits exactly one JSON document on stdout**: the terminal `result` or `error`. Nothing else is ever written to stdout.
2. **Progress and warnings stay off stdout.** The SDK's human-readable progress output is redirected to stderr in JSON mode. 

The current `schemaVersion` is **1**.

## Envelope

Every document is an object with these fields:

| Field | Type | Description |
|---|---|---|
| `schemaVersion` | integer | Schema version. Currently `1`. |
| `type` | string | `result` or `error`. |
| `command` | string | Command path without the leading `tpix`, e.g. `search`, `zotero list`. |

## Terminal result

```json
{ "schemaVersion": 1, "type": "result", "command": "search", "data": { ... } }
```

## Terminal error

```json
{
  "schemaVersion": 1,
  "type": "error",
  "command": "info",
  "error": {
    "code": 4,
    "httpStatus": 404,
    "message": "package not found",
    "description": "..."
  }
}
```

| Field | Type | Description |
|---|---|---|
| `code` | integer | Stable process exit code (see below). |
| `httpStatus` | integer | Present when the failure came from the server. |
| `message` | string | Short error message. |
| `description` | string | Optional longer description from the server. |

### Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Generic / unexpected error |
| 2 | Usage error (bad flags/args, or command not yet supported in JSON) |
| 3 | Authentication or configuration error |
| 4 | Not found (package, version, export) |
| 5 | Network or server error |
| 6 | Validation failure (e.g. upload rejected) |

## Per-command `data` payloads

### `version`
```json
{ "version": "v0.1.0", "schemaVersion": 1, "hasUpdate": false, "latest": "v0.2.0" }
```
`schemaVersion` is also present at the envelope level; it is repeated here so a
consumer can cheaply verify compatibility with `tpix version --json`.

### `search`
Reuses the server's search response.
```json
{ "query": "cetz", "total": 1, "count": 1, "results": [ /* search results */ ] }
```

### `info`
Reuses the server's package response (`PackageResponse`).

### `list`
```json
{
  "cachePath": "/home/u/.cache/typst/packages",
  "packages": [
    { "namespace": "preview", "name": "cetz", "version": "0.3.0", "path": "..." }
  ],
  "total": 1
}
```

### `get`
```json
{
  "requested": "@preview/cetz",
  "packages": [
    { "namespace": "preview", "name": "cetz", "version": "0.3.0", "cached": false }
  ],
  "resolved": 1
}
```
`cached` is `true` when the package was already in the local cache and `false` when it was downloaded.

### `pull`
```json
{
  "projectDir": "/abs/project",
  "dryRun": false,
  "direct": [
    { "namespace": "preview", "name": "cetz", "version": "0.3.0", "cached": true }
  ],
  "packages": [
    { "namespace": "preview", "name": "cetz", "version": "0.3.0", "cached": true }
  ],
  "resolved": 1
}
```
`direct` lists the imports discovered in the project with their cache status
at scan time. `packages` is the full resolved set (direct + transitive) and is
empty in `--dry-run`.

### `deps`
```json
{
  "root": {
    "package": { "namespace": "preview", "name": "cetz", "version": "0.5.2" },
    "cached": true,
    "children": [
      {
        "package": { "namespace": "preview", "name": "oxifmt", "version": "1.0.0" },
        "cached": true
      }
    ]
  },
  "packages": [
    { "namespace": "preview", "name": "cetz", "version": "0.5.2", "cached": true },
    { "namespace": "preview", "name": "oxifmt", "version": "1.0.0", "cached": true }
  ]
}
```
`deps` resolves the full transitive dependency graph without downloading any
package. `root` is a recursive tree (shared packages appear once and are then
shown as leaves to avoid cycles); `packages` is the flattened, de-duplicated
list.

### `push`
```json
{
  "namespace": "acme",
  "package": "mypkg",
  "version": "1.0.0",
  "sha256": "...",
  "size": 1234,
  "success": true,
  "report": []
}
```
`success` is `false` (and `report` non-empty) when the server rejected the package during validation.

### `remove`
```json
{ "removed": { "namespace": "preview", "name": "cetz", "version": "0.3.0" } }
```

### `bundle`
```json
{ "sourceDir": "./mypkg", "outputPath": "/abs/mypkg/mypkg.tar.gz" }
```

### `login`
```json
{ "success": true }
```

### `logout`
```json
{ "success": true }
```

### `whoami`
Reuses the server's user profile response.
```json
{
  "username": "alice",
  "email": "alice@example.com",
  "created_at": "2024-01-01T00:00:00Z",
  "namespaces": [ { "name": "acme", "permission": "write" } ]
}
```

### `cache-path`
```json
{ "path": "/home/u/.cache/typst/packages", "source": "config", "previous": "..." }
```
`source` is `config` or `env` (the `TYPST_PACKAGE_CACHE_PATH` override).
`previous` is only present when setting a new path.

### `zotero list`
```json
{ "libraries": [ /* ZoteroLibrary objects */ ] }
```

### `zotero delete`
```json
{ "deleted": "exp_123" }
```

### `zotero export`
```json
{
  "exportId": "exp_123",
  "format": "biblatex",
  "outputPath": "references.bib",
  "content": "..."
}
```
In JSON mode `--library` is required (no interactive prompt). `outputPath` is
present when `-o/--output` was given; otherwise `content` holds the exported
text.


## Stability

`schemaVersion` tracks backwards-incompatible changes. Additive fields do **not** bump it. Consumers should ignore unknown fields.
