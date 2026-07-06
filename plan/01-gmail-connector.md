# Plan 01 — Gmail Connector + Incremental Sync

## Goal

Enable `flat-email sync --provider gmail --account me@example.com --out ./my-archive`
to fetch a Gmail mailbox over the Gmail API and write a complete,
`SPEC.md`-conformant archive, with subsequent runs only fetching what changed.

## Prerequisites

- v0.1 archive producer (`internal/archive/`) is complete and golden-tested.
- `storage.Backend` interface is stable.
- `model.Input` / `model.Message` / `model.Account` types are stable.

## Scope

**In scope**

- OAuth 2.0 authentication flow for Gmail (browser-based consent, local
  redirect listener, token refresh).
- Credential storage in the OS keychain (never in the archive or a plain file).
- Full initial sync: fetch all messages → write archive.
- Incremental sync using Gmail History API (delta from last `historyId`).
- Mapping Gmail-specific fields to the `model.Message` type:
  `internalDate`, labels (including system labels INBOX / SENT / SPAM / TRASH),
  `threadId`, `providerMessageId`, `providerThreadId`, flags.
- `.flat-email-state/<account>.json` cursor file for `historyId` and `syncTime`
  (SPEC.md §14).
- `flat-email sync` CLI sub-command wired to the Gmail connector.
- Append-only semantics: messages deleted upstream are retained in the archive
  with `lastSeen` updated; label membership is mirrored (SPEC.md §6).
- Rate-limit handling and exponential-backoff retry.

**Out of scope**

- Outlook / IMAP connectors (separate plans).
- Push (Gmail watch / Pub/Sub) notifications — polling incremental sync only.
- Sending or modifying mail.

## Architecture

### New packages

```
internal/connector/
  connector.go        # Connector interface
  gmail/
    auth.go           # OAuth 2.0 device / redirect flow, keychain storage
    client.go         # Gmail API wrapper (messages.list, messages.get, history.list)
    sync.go           # Full and incremental sync logic → []model.Message
    mapper.go         # Gmail API types → model.Message
internal/syncstate/
  state.go            # Read/write .flat-email-state/<account>.json cursors
```

### Connector interface

```
type Connector interface {
    // Fetch retrieves the account's messages and label definitions. On the
    // first run or when the cursor is absent it does a full scan; on
    // subsequent runs it uses the stored cursor to fetch only changes.
    Fetch(ctx context.Context, account string, state SyncState) (model.Account, SyncState, error)
}
```

The connector returns a `model.Account` (not a bare `[]model.Message`) because
the producer also needs the account's label definitions — the
`map[string]model.Label` with original names, provider IDs, `type`
(system/user), and visibility that feeds the label manifest (SPEC.md §4.7).
This interface is the only contract the CLI and future connectors must satisfy.

### Data flow

```
flat-email sync
  → gmail.Auth (keychain ↔ OAuth)
  → syncstate.Load (read cursor from .flat-email-state/)
  → gmail.Connector.Fetch (full or incremental)
  → model.Input
  → archive.Produce (storage.Backend)
  → syncstate.Save (write new cursor)
```

Note on incremental runs: `archive.Produce` today regenerates the whole
archive from a complete `model.Input`. The incremental path (Phase 3) needs a
partial-update entry point in `internal/archive/` that can update
`metadata.json`, label indexes, thread files, and the root
`catalog.json`/`catalog.js` for a subset of messages — while preserving each
message's `firstSeen` (the producer already reads existing `metadata.json`
for this). This shared machinery is a prerequisite for Plan 02 as well; see
`plan/README.md` "Shared foundations".

### Sync state file

`.flat-email-state/<account>.json`:

```json
{
  "provider": "gmail",
  "account": "me@example.com",
  "historyId": "1234567",
  "syncTime": "2024-01-15T09:30:00Z"
}
```

No message bodies, no credentials.

## Phases

### Phase 1 — Auth and credential storage

1. Add `golang.org/x/oauth2` and the Gmail API client library to `go.mod`.
2. Implement `gmail/auth.go`:
   - `Login(account string)` — opens a browser to Google's consent page, spins
     a local `localhost` redirect listener, exchanges the code for tokens, and
     stores the refresh token in the OS keychain.
   - `Token(account string)` — retrieves the stored refresh token and exchanges
     it for a fresh access token.
   - Keychain backends: macOS Keychain (`security` CLI or `golang-keyring`),
     Windows Credential Manager, freedesktop Secret Service on Linux.
3. Add `flat-email auth gmail --account me@example.com` CLI sub-command.

### Phase 2 — Full initial sync

1. Implement `gmail/client.go`:
   - `ListMessages` — pages through `messages.list` to collect all message IDs.
   - `GetMessage` — fetches a single message in `raw` format (base64 RFC 5322
     bytes) plus metadata (`internalDate`, `labelIds`, `threadId`, `id`).
   - Respect Gmail API quota: batch fetching (up to 100 IDs per batch request).
2. Implement `gmail/mapper.go`:
   - Decode the base64url `raw` field → `model.Message.Raw`.
   - Convert `internalDate` (Unix ms) → `*time.Time` for `InternalDate`.
   - Map `labelIds` to `model.Message.Labels` (system labels kept as-is;
     user label names resolved via `users.labels.list`).
   - Build the `model.Account.Labels` map from `users.labels.list`: original
     name, provider label id, `type` (`system` for Gmail system labels, else
     `user`), and visibility (`labelListVisibility`) — this feeds the label
     manifest (SPEC.md §4.7).
   - Map `UNREAD` absence → `seen` flag; `STARRED` → `starred`; `DRAFT` →
     `draft`; `IMPORTANT` → `important`. Keep unmapped label-flags verbatim in
     `ProviderFlags`.
   - Populate `ProviderMessageID` and `ProviderThreadID`.
3. Implement `gmail/sync.go` — full path: calls `ListMessages` + `GetMessage`
   for each, returns a `model.Account` and a `SyncState{HistoryId: latestId}`.
4. Implement `syncstate` package — JSON read/write under `.flat-email-state/`.
5. Wire `flat-email sync --provider gmail` to call `archive.Produce`.
6. Add integration test with recorded HTTP fixtures (no live network required).

### Phase 3 — Incremental sync

1. Implement `gmail/client.go#ListHistory`:
   - Pages through `history.list` from the stored `historyId`.
   - Collects `messagesAdded`, `messagesDeleted`, `labelsAdded`, `labelsRemoved`
     events.
2. Implement `gmail/sync.go` — incremental path (uses the incremental archive
   update entry point, see `plan/README.md` "Shared foundations"):
   - Fetch full raw bytes only for newly added messages.
   - For label changes on existing messages: read the existing `metadata.json`,
     update the `labels` / `flags` fields, re-derive `email.html`, and
     re-render the affected label index files. Do **not** rewrite
     `message.eml`, and preserve `firstSeen`.
   - For deleted messages: update `lastSeen` in `metadata.json`; do not delete
     the message from the archive (append-only per SPEC.md §6).
   - After any change, regenerate the global derived files: the root
     `catalog.json` / `catalog.js`, affected `threads/<thread-key>.*`, and
     `labels/labels.json` — they must always reflect the archive exactly.
3. Update `syncstate.Save` with the new `historyId` only after all writes
   succeed (write-last to avoid partial-state corruption).

### Phase 4 — CLI polish and error handling

1. Progress output: messages fetched/written counts, elapsed time.
2. `--dry-run` flag: print what would be fetched without writing.
3. `--since <date>` flag: initial sync limited to messages after a date.
4. Graceful handling of Gmail API errors (401 → re-auth prompt, 429 → backoff,
   5xx → retry with jitter).
5. Lock file under `.flat-email-state/` to prevent concurrent sync runs on the
   same archive.

## Testing strategy

- **Unit tests** for `mapper.go` with hard-coded Gmail API response JSON;
  assert `model.Message` fields exactly.
- **Golden test** — a small, fixed set of Gmail API responses (recorded once,
  committed as JSON fixtures) produces a byte-identical archive. Extends the
  existing `tests/golden/` infrastructure.
- **Incremental round-trip test** — run full sync, assert archive A; run
  incremental sync with a few label changes and a deletion, assert archive B
  matches the expected diff exactly.
- **Determinism test** — two full syncs from the same fixtures produce
  identical archives.

## Risks

| Risk | Mitigation |
|------|-----------|
| Gmail API quota (per-project daily units; the binding limit in practice is the **250 units/user/second** rate, and `messages.get` costs 5 units) | Batch fetching; client-side rate limiter; expose `--throttle` flag |
| OAuth consent screen requires Google verification for production use | Document the "unverified app" path for personal use; provide instructions for users to create their own OAuth credentials |
| `internalDate` absent on draft / sent messages | Fall back to SPEC.md §4.1 chain (`Received:` then `Date:`) |
| Keychain unavailable in headless/CI environments | `--token-file` escape hatch (documented as insecure, for CI only) |
| History API gaps (`historyId` typically expires after ~a week of no sync) | Detect the 404/`INVALID_HISTORY_ID` error, fall back to full re-scan which remains idempotent |
| Whole-mailbox `model.Input` held in memory on large initial syncs | Acceptable for v1 (matches the current producer's contract); track a chunked/streaming produce path as a follow-up shared with Plan 02 |
