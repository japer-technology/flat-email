# Plan 02 — Generic IMAP Connector

## Goal

Enable `flat-email sync --provider imap --host imap.example.com --user me@example.com --out ./my-archive`
to sync any standard IMAP mailbox into a `SPEC.md`-conformant archive, with
incremental re-runs fetching only new or changed messages.

## Prerequisites

- `internal/connector/connector.go` interface exists (from Plan 01).
- `internal/syncstate/` package exists (from Plan 01).
- `storage.Backend` interface is stable.

## Scope

**In scope**

- IMAP4rev1 (RFC 3501) and IMAP4rev2 (RFC 9051) connection and auth.
  - Auth methods: SASL `PLAIN` and `LOGIN` (only ever over an encrypted
    connection).
  - TLS (port 993) and STARTTLS (port 143).
  - App-password flows (Gmail IMAP, Outlook IMAP with Modern Auth disabled).
- **Strictly read-only fetching.** All body fetches use `BODY.PEEK[]` (never
  `RFC822`/`BODY[]`, which implicitly set `\Seen` on the server). This is what
  upholds the project-wide "never modifies your source mailboxes" guarantee.
- Full initial sync per folder/mailbox.
- Incremental sync using `UIDVALIDITY` + `UIDNEXT` cursors (SPEC.md §14).
- Mapping IMAP flags (`\Seen`, `\Answered`, `\Flagged`, `\Deleted`, `\Draft`,
  `\Recent`) to `model.Message.Flags`.
- Mapping IMAP folder names to labels (each folder → one label; a message
  in multiple folders via IMAP COPY semantics is stored once, referenced by both
  labels — the standard store-once rule from SPEC.md §4.3).
- Credential storage in the OS keychain (same backend as Plan 01).
- `.flat-email-state/<account>.json` cursor per folder: `uidvalidity`, `uidnext`.

**Out of scope**

- IMAP IDLE push notifications (polling only for now).
- IMAP NOTIFY / QRESYNC extensions (safe to add later as an optimisation).
- Sending or modifying mail.
- Outlook-specific OAuth Modern Auth (covered by a future Outlook connector plan).

## Architecture

### New packages

```
internal/connector/imap/
  auth.go     # Credential prompt + keychain storage
  client.go   # IMAP session: connect, SELECT, UID FETCH, UID SEARCH, LOGOUT
  sync.go     # Full and incremental sync → []model.Message
  mapper.go   # IMAP envelope/flags/body → model.Message
```

The `imap.Connector` implements the same `connector.Connector` interface defined
in Plan 01, so the CLI wiring is identical.

### Sync state file

`.flat-email-state/<account>.json` (per folder, keyed by folder name):

```json
{
  "provider": "imap",
  "account": "me@example.com",
  "folders": {
    "INBOX": { "uidvalidity": 1234567890, "uidnext": 4001, "syncTime": "2024-01-15T09:30:00Z" },
    "Sent":  { "uidvalidity": 1234567891, "uidnext": 201,  "syncTime": "2024-01-15T09:30:00Z" }
  }
}
```

### Data flow

```
flat-email sync --provider imap
  → imap.Auth (keychain ↔ password/app-password)
  → syncstate.Load
  → imap.Connector.Messages (full or incremental per folder)
  → model.Input
  → archive.Produce
  → syncstate.Save
```

## Phases

### Phase 1 — Connection and credential storage

1. Add an IMAP client library to `go.mod` (e.g. `github.com/emersion/go-imap/v2`).
2. Implement `imap/auth.go`:
   - Prompt for password on first run, store in OS keychain keyed by
     `flat-email:imap:<host>:<user>`.
   - Retrieve stored credential on subsequent runs.
3. Implement `imap/client.go`:
   - `Connect(host, port, tls bool, user, pass string) (*Session, error)` — TLS
     dial, CAPABILITY check, SELECT folder.
   - `ListFolders` — `LIST "" "*"` returns all subscribable folders.
   - `FetchMessages(folder, uidset)` — `UID FETCH <set> (UID FLAGS INTERNALDATE BODY.PEEK[])`.
     `BODY.PEEK[]` (not `RFC822`) so fetching never sets `\Seen` upstream.
4. Add `flat-email auth imap --host ... --user ...` CLI sub-command.

### Phase 2 — Full initial sync

1. Implement `imap/mapper.go`:
   - `INTERNALDATE` response → `model.Message.InternalDate` (first priority for
     bucket date, SPEC.md §4.1).
   - `BODY.PEEK[]` response literal → `model.Message.Raw` (verbatim RFC 5322
     bytes).
   - IMAP system flags → `model.Message.Flags` using the normalised vocabulary
     (`\Recent` is deprecated in IMAP4rev2; map it when present, never rely on it).
   - Folder name → `model.Message.Labels` (one entry per folder); build the
     `model.Account.Labels` map from `LIST` results (folder attributes such as
     `\Sent` / `\Junk` from SPECIAL-USE map to `type: "system"`).
   - `UID` and folder → `ProviderMessageID`, encoded as
     `<folder>/<uidvalidity>/<uid>` so the id stays unambiguous across folder
     rebuilds.
2. Implement `imap/sync.go` — full path:
   - `LIST` all folders.
   - For each folder: `SELECT` (read-only via `EXAMINE`), record `UIDVALIDITY`
     and `UIDNEXT`, fetch all messages with `UID FETCH 1:* ...`.
   - Deduplicate messages that appear in multiple folders by the SHA-256 of the
     raw bytes — the same digest that keys the archive (SPEC.md §4.2), so the
     store-once rule falls out for free. Do **not** dedupe on the `Message-ID`
     header: it is optional, not unique, and two different byte sequences are
     two different messages by definition. Each duplicate contributes its
     folder as an additional label on the single stored message.
   - Return a `model.Account` and updated `SyncState`.
3. Wire `flat-email sync --provider imap` to the full sync path.

### Phase 3 — Incremental sync

1. On each run, compare stored `UIDVALIDITY` with server value:
   - If changed: `UIDVALIDITY` reset means the folder was rebuilt; discard
     cursor and fall back to full sync for that folder.
   - If unchanged: fetch only `UID <uidnext>:*` (new messages since last sync).
2. Detect flag changes: `UID FETCH <known-uid-range> FLAGS` (flags-only fetch,
   no body) and update `metadata.json` for any changed messages without
   rewriting `message.eml` — using the same incremental archive update entry
   point as Plan 01 (see `plan/README.md` "Shared foundations"), which also
   regenerates the affected label indexes and the root `catalog.json`/`catalog.js`.
3. Handle `EXPUNGE` responses (messages removed upstream): update `lastSeen` in
   `metadata.json`; retain the message in the archive (append-only).
4. Update cursor after all writes succeed.

### Phase 4 — CLI flags and polish

1. `--folder <name>` flag to sync only a specific IMAP folder (default: all).
2. `--port` and `--starttls` flags.
3. Progress output per folder.
4. Retry on transient connection errors (network drop, temporary 5xx-equivalent
   IMAP NO responses) with exponential backoff.

## Testing strategy

- **Unit tests** for `mapper.go` with hard-coded IMAP server response strings.
- **Mock IMAP server** (in-process, using the same library's test helpers) for
  full and incremental sync paths.
- **Golden test** — a fixed IMAP server state (committed fixture) produces a
  byte-identical archive. Extends `tests/golden/`.
- **UIDVALIDITY-reset test** — simulate a folder rebuild mid-sync; assert the
  connector discards the stale cursor and does a clean full re-scan that produces
  an identical archive.

## Risks

| Risk | Mitigation |
|------|-----------|
| IMAP server dialect variation (some servers misreport `UIDNEXT`) | Test against Dovecot, Gmail IMAP, and Fastmail fixtures; log and skip messages that fail to parse rather than abort |
| TLS certificate issues on self-hosted servers | `--skip-tls-verify` flag (documented as insecure; for trusted private networks only) |
| Very large mailboxes (100 k+ messages) on initial sync | Stream `UID FETCH` in UID-range batches of 1 000; report progress |
| IMAP COPY semantics create duplicate raw bytes for messages in multiple folders | Deduplicate by raw-byte SHA-256 (same key → same archive entry, two label references) |
| Accidentally marking mail read on the server (write side-effect) | Use `EXAMINE` (read-only select) and `BODY.PEEK[]` everywhere; add a test asserting the mock server never receives a `STORE`/flag-setting command |
| The same logical message has *different* bytes in different folders (some servers rewrite headers on COPY) | Accept it: two byte sequences are two archive entries by SPEC.md §4.2; document the behaviour rather than guess at equivalence |
