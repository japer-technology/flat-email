# Plan 03 — Full-Text Search

## Goal

Enable `flat-email search "from:alice invoice 2024" --archive ./my-archive` to
return ranked results from the local archive instantly, with no external server
and no round-trip to the email provider.

## Prerequisites

- v0.1 archive producer is complete.
- `catalog.json` and `metadata.json` files are stable and correct.
- At least one connector (local import or Gmail) is working so a real archive
  exists to index.

## Scope

**In scope**

- A regenerable, local-only search index stored under `index/` (already listed
  in the SPEC.md directory layout as a derived, regenerable path).
- Indexing of `metadata.json` fields: `from`, `to`, `cc`, `subject`, `date`,
  `labels`, `flags`, `attachmentCount`, `messageKey`.
- Indexing of `body.txt` full text.
- Query syntax: bare keywords (AND by default), `from:`, `to:`, `subject:`,
  `label:`, `has:attachment`, `after:`, `before:`, `is:unread`/`is:starred`.
- Ranked results returned to stdout as JSON or a human-readable table.
- `flat-email index --archive ./my-archive` command to build/rebuild the index.
- Index is rebuilt automatically on `sync` (after the archive write succeeds).
- Index rebuild is idempotent and produces the same bytes for the same archive
  content.

**Out of scope**

- Encrypted-index search (IDEAS.md §1.4 open item).
- Remote / cloud search engines (ElasticSearch, Typesense, etc.).
- Attachment content indexing (PDF/Word text extraction) — later enhancement.
- Web-based search UI (that is the HTTP API + reader, Plan 04).

## Architecture

### Index technology choices

Two options; the plan uses **embedded SQLite with FTS5** as the primary choice:

| Option | Pros | Cons |
|--------|------|------|
| SQLite FTS5 (via `modernc.org/sqlite`) | Pure Go, zero CGo, single file, proven, good ranking (BM25) | Less advanced ranking than dedicated engines |
| Bleve (`github.com/blevesearch/bleve`) | Pure Go, rich query language, pluggable analysers | Larger binary, more complex |

SQLite FTS5 is preferred because it:
- Produces a single `index/search.db` file, easy to exclude from Git/backup.
- Is already regenerable from the archive with no extra tooling.
- Can answer structured metadata queries (date ranges, label filters) and
  full-text queries in one pass.
- Keeps the binary small and the dependency tree simple.

### New packages

```
internal/search/
  index.go    # Build / update the FTS5 index from the archive
  query.go    # Parse and execute the query DSL; return ranked hits
  schema.go   # SQLite table definitions (virtual FTS5 table + metadata table)
```

### Index location

`index/search.db` — inside the archive's `index/` directory, the same path
already reserved by SPEC.md as regenerable. Excluded from Git archives via
`.gitignore` (`index/`) by convention (documented in IDEAS.md §1.2).

### Schema

```sql
-- Structured metadata (ordinary table for filtered queries)
CREATE TABLE messages (
  message_key   TEXT PRIMARY KEY,
  account       TEXT NOT NULL,
  date          TEXT NOT NULL,  -- ISO 8601 UTC
  subject       TEXT,
  from_name     TEXT,
  from_address  TEXT,
  labels        TEXT,           -- JSON array string
  flags         TEXT,           -- JSON array string
  has_attachment INTEGER,
  thread_key    TEXT
);

-- Full-text index
CREATE VIRTUAL TABLE messages_fts USING fts5(
  message_key UNINDEXED,
  subject,
  from_name,
  from_address,
  to_addresses,
  body_text,
  content='messages',   -- linked for snippet extraction
  tokenize='unicode61'
);
```

### Query parsing

The query DSL is intentionally Gmail-like so users need no new mental model:

| Token | SQL translation |
|-------|----------------|
| `word` | FTS5 match on `subject`, `from_name`, `body_text` |
| `from:alice` | `from_address LIKE '%alice%' OR from_name LIKE '%alice%'` |
| `to:bob` | `to_addresses LIKE '%bob%'` |
| `subject:invoice` | FTS5 on `subject` only |
| `label:inbox` | `labels JSON contains 'inbox'` |
| `has:attachment` | `has_attachment = 1` |
| `after:2024-01-01` | `date >= '2024-01-01T00:00:00Z'` |
| `before:2024-12-31` | `date < '2024-12-31T00:00:00Z'` |
| `is:unread` | `flags NOT LIKE '%"seen"%'` |

Results are ranked by FTS5 BM25 score, then descending date as a tie-breaker.

## Phases

### Phase 1 — Index build

1. Add `modernc.org/sqlite` to `go.mod`.
2. Implement `search/schema.go` — `CreateSchema(db)` creates the tables above.
3. Implement `search/index.go`:
   - `Build(backend storage.Backend, db *sql.DB)` — walks the archive via
     `backend.List("accounts/")`, reads each `metadata.json` and `body.txt`,
     inserts rows.
   - `Update(backend, db, messageKeys []string)` — upserts only changed messages
     (called by sync after new messages are written).
   - Both paths are idempotent (INSERT OR REPLACE).
4. Implement `flat-email index --archive` CLI sub-command calling `search.Build`.
5. Call `search.Update` at the end of `flat-email sync` automatically.

### Phase 2 — Query execution

1. Implement `search/query.go`:
   - `Parse(q string) Query` — tokenizes the query string into FTS and
     structured filter parts.
   - `Execute(db, q Query, limit int) []Result` — builds and runs the SQL,
     returns `Result{MessageKey, Account, Date, Subject, From, Snippet}`.
   - Snippet extraction using SQLite's `snippet()` function.
2. Implement `flat-email search "<query>" --archive` CLI sub-command:
   - Default output: human-readable table (date, from, subject, snippet).
   - `--json` flag: JSON array output for scripting.
   - `--limit N` flag (default 20).
   - `--account <addr>` flag to restrict to one account.

### Phase 3 — Incremental index maintenance

1. On `flat-email sync`, after `archive.Produce` succeeds, call `search.Update`
   with only the newly written message keys.
2. On label changes (incremental sync), update the `labels`/`flags` columns
   for affected messages without re-indexing the body text.
3. Detect a missing or corrupt `index/search.db` and rebuild automatically
   (warn the user but do not fail).

## Testing strategy

- **Unit tests** for `query.go` parser: known query strings → expected SQL AST.
- **Integration test**: build an index from the existing golden archive fixture;
  run a set of known queries; assert the returned message keys match expected
  results exactly (deterministic because the archive is deterministic).
- **Rebuild idempotency test**: build the index twice from the same archive,
  assert the SQLite file is byte-identical (achievable by always `VACUUM` after
  build and using a fixed page size).

## Risks

| Risk | Mitigation |
|------|-----------|
| Large archives (500 k+ messages) make initial index build slow | Stream rows in batches; report progress; allow `--resume` that skips already-indexed keys |
| SQLite WAL file left behind after a crash makes the index "unclean" | Open with `PRAGMA journal_mode=DELETE` for the build phase; WAL mode only for live query use |
| FTS5 ranking not tuned for email | Accept BM25 defaults for now; expose `--bm25` weight flags as a future enhancement |
| `body.txt` absent for HTML-only messages | Index `subject` and `from` only for those messages; note in result that no body snippet is available |
