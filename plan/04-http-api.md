# Plan 04 — HTTP API + Serverless Reader (`flat-email serve` + `index.html`)

## Goal

Enable `flat-email serve --archive ./my-archive --port 8080` to expose a
local, read-only HTTP API over the archive, and ship a generated `index.html`
at the archive root that works as a serverless offline web app for browsing
the entire mailbox from disk.

## Prerequisites

- v0.1 archive producer is complete (`email.html` per message, `catalog.json`/`catalog.js`).
- `internal/search/` package exists (Plan 03) — the API's search endpoint uses it.

## Scope

**In scope**

- A local HTTP server (`flat-email serve`) — localhost-only by default.
- Read-only REST API endpoints (see Routes below).
- A generated, self-contained `index.html` at the archive root:
  - Loads `catalog.js` (a `<script>` tag, not `fetch()`) to enumerate the archive.
  - Renders a message list, label/thread navigation, and per-message view.
  - Works fully offline from `file://` for archives up to ~10 k messages.
  - For larger archives or search, falls back to the local HTTP API if
    `flat-email serve` is running (detected by a health-check fetch).
- The `index.html` is regenerated on every `flat-email sync` and by
  `flat-email index`.

**Out of scope**

- Authentication or TLS for the local server (it binds `127.0.0.1` only).
- Write endpoints (reply, delete, etc.).
- A cloud-hosted or remotely accessible server.
- A full SPA framework; the reader uses vanilla HTML/CSS/JS to stay
  self-contained and avoid a build step.

## Architecture

### New packages

```
internal/query/
  query.go      # Shared, transport-neutral archive read layer: list/get
                # messages, threads, labels via storage.Backend + catalog.
                # Both the HTTP handlers here and the MCP tools (Plan 05)
                # call this package, so archive access logic exists once.
internal/api/
  server.go     # net/http server setup, middleware, graceful shutdown
  routes.go     # Route registration
  handlers/
    messages.go  # GET /messages, GET /messages/{key}
    threads.go   # GET /threads, GET /threads/{key}
    labels.go    # GET /labels, GET /labels/{name}
    search.go    # GET /search?q=...
    attachments.go # GET /messages/{key}/attachments/{filename}
    health.go    # GET /health
internal/reader/
  generate.go   # Render index.html from a template + catalog
  template.html # The self-contained reader template (compiled into the
                # binary via go:embed, so `flat-email` ships as a single
                # executable with no template files to distribute)
```

### API routes

All routes are read-only (`GET` only). All responses are JSON unless noted.

| Route | Description |
|-------|-------------|
| `GET /health` | `{"ok": true}` — used by `index.html` to detect if the API is running |
| `GET /catalog` | Returns `catalog.json` contents |
| `GET /accounts` | Lists account addresses in the archive |
| `GET /messages?account=me@example.com&label=inbox&after=2024-01-01&before=2024-12-31&limit=50&offset=0` | Paginated message list with filtering (`account` optional; defaults to all) |
| `GET /messages/{messageKey}` | Full message metadata + body preview |
| `GET /messages/{messageKey}/raw` | Raw `message.eml` bytes (`Content-Type: message/rfc822`) |
| `GET /messages/{messageKey}/html` | Sanitized `body.html` (already safe) |
| `GET /messages/{messageKey}/attachments/{filename}` | Decoded attachment file (serves directly from `attachments/`) |
| `GET /threads/{threadKey}?account=...` | Ordered list of message keys in thread |
| `GET /labels?account=...` | Label manifest (per-account `labels/labels.json`) |
| `GET /labels/{sanitizedName}?account=...` | Message keys for one label |
| `GET /search?q=...&limit=20&offset=0` | Full-text search (delegates to `internal/search/`) |

Account scoping: message keys are content digests and therefore unique
archive-wide (SPEC.md §4.2), so message routes need no account parameter —
when the same message exists under several accounts the handler resolves it
via the catalog and may return all locations. Threads and labels, by contrast,
live under `accounts/<account>/` on disk, so their routes take an `account`
query parameter; when the archive holds exactly one account it may be omitted.

All list endpoints include `total`, `limit`, and `offset` in the response
envelope for pagination.

### `index.html` design

`index.html` is a generated, single-file web app. Key design decisions driven
by `SUGGESTIONS.md §5` and SPEC.md §11:

1. **Catalog loaded via `<script>`** — `<script src="catalog.js"></script>`
   works on `file://` in all major browsers. `fetch()` of a sibling `.json`
   is blocked by Chrome's CORS policy on `file://`.
2. **Offline-first, API-optional** — on load, `index.html` checks for the API
   (`GET /health`). If the API responds, it uses it for search and large lists.
   If not, it works from `catalog.js` alone (message list, navigation, opening
   individual `email.html` pages).
3. **No external dependencies** — all styles and scripts are inlined. No CDN
   requests. No fonts fetched from the network.
4. **Per-message navigation** — clicking a message opens its `email.html`
   (a new tab or the same tab). No in-page rendering of arbitrary email HTML
   (that is `email.html`'s job, already sanitized and self-contained).
5. **Scale threshold** — the catalog embeds enough metadata per message
   (subject, date, from, labels — see SPEC.md §11) to render a list of up to
   ~10 k messages inline. Above that threshold, `index.html` shows a notice
   and requires the HTTP API for pagination.

### Server security

- Binds `127.0.0.1` only (never `0.0.0.0`) unless `--host` is explicitly set.
- `--host` flag requires a confirmation prompt if not `127.0.0.1`.
- **CORS for the `file://` reader.** A page opened from `file://` sends
  `Origin: null` (or no origin), so without CORS headers the reader's
  `fetch("http://127.0.0.1:8080/health")` fallback silently fails. The server
  responds with `Access-Control-Allow-Origin: *` on its read-only GET routes —
  safe here because every endpoint is read-only, unauthenticated, and
  localhost-bound; there is no credentialed state to leak cross-origin.
- Sets `X-Content-Type-Options: nosniff` and `X-Frame-Options: DENY` on all
  responses.
- Serves attachment files with `Content-Disposition: attachment` (forces
  download, prevents browser execution).
- Does not serve `message.eml` raw bytes by default; requires `--allow-raw` flag.

## Phases

### Phase 1 — HTTP API skeleton

1. Implement `api/server.go`:
   - `NewServer(archiveRoot string, searchDB *sql.DB) *http.ServeMux`.
   - Graceful shutdown on SIGINT/SIGTERM.
   - Request logging (method, path, status, duration) to stderr.
2. Implement `api/handlers/health.go` and `api/handlers/messages.go` (list and
   get endpoints) reading from `catalog.json` and `metadata.json` via the
   `storage.Backend`.
3. Implement `flat-email serve --archive --port` CLI sub-command.
4. Manual smoke test: `flat-email serve` against the golden archive, `curl`
   the routes.

### Phase 2 — Remaining API routes

1. `handlers/threads.go`, `handlers/labels.go`, `handlers/attachments.go`.
2. `handlers/search.go` — delegates to `internal/search/query.go`.
3. `handlers/messages.go#Raw` (behind `--allow-raw`).
4. Pagination for all list endpoints.

### Phase 3 — `index.html` reader

1. Design the minimal reader UI:
   - Left panel: label/folder tree from `catalog.js`.
   - Right panel: message list for the selected label.
   - Detail view: links to individual `email.html` pages.
   - Search bar (calls API if running; otherwise filters catalog in-memory).
2. Implement `reader/template.html` — a single self-contained HTML file with
   all styles and JS inlined.
3. Implement `reader/generate.go` — reads `catalog.json`, templates the file,
   writes `index.html` to the archive root.
4. Call `reader.Generate` at the end of `archive.Produce` (so every sync
   refreshes it).
5. Browser-matrix test: open a generated `index.html` under `file://` in
   Chrome, Firefox, and Safari.

### Phase 4 — Polish and docs

1. `--open` flag on `flat-email serve` to launch the browser automatically.
2. Document the `file://` scale threshold in the README and in `index.html`
   itself (a visible notice for large archives).
3. Add `flat-email serve --generate-only` to regenerate `index.html` without
   starting the server.

## Testing strategy

- **API handler unit tests** using `net/http/httptest` against the golden archive.
- **Pagination correctness test**: request pages of size 1, 2, N and assert
  the union of pages equals the full catalog with no duplicates.
- **`index.html` generation test**: assert the output is valid HTML and that
  `catalog.js` content appears verbatim in the generated `<script>` tag.
- **`file://` smoke test** (manual, browser matrix): Chrome, Firefox, Safari —
  document results in `tests/browser-compat.md`.

## Risks

| Risk | Mitigation |
|------|-----------|
| `file://` `fetch()` blocked in Chrome for `catalog.json` | Use `<script src="catalog.js">` (confirmed to work) instead of `fetch()` |
| Reader on `file://` cannot call the API (`Origin: null` blocked by CORS) | Serve `Access-Control-Allow-Origin: *` on the read-only GET routes; cover with an httptest assertion |
| Path traversal via `{messageKey}` / `{filename}` route parameters | Validate `messageKey` against the `[0-9a-f]{32,64}` key grammar; resolve attachment paths through the catalog, never by joining raw user input onto the filesystem |
| `index.html` grows too large if catalog is embedded inline | Cap at ~10 k messages inline; link to API for the rest |
| XSS via message content in the reader | Per-message content is served from already-sanitized `email.html`; the reader itself never renders raw email HTML inline |
| Port conflicts for `flat-email serve` | Default to `8080`, `--port` flag; detect and report binding errors clearly |
