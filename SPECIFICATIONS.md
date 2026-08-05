# Flat Email Product Specifications

**Status:** Draft  
**Scope:** Product behavior and implementation contracts  
**Archive format:** [`SPEC.md`](SPEC.md), currently `specVersion: 1`

## 1. Purpose and authority

Flat Email turns a mailbox into an ownable collection of ordinary files. The
product promise is:

> Given the same authoritative mail and source facts, produce a predictable,
> self-describing archive that remains readable, searchable, and portable
> without the source provider, a database, or a network connection.

This document consolidates the product requirements implied by
[`MISSION.md`](MISSION.md), [`SUGGESTIONS.md`](SUGGESTIONS.md), the implementation
plans in [`plan/`](plan/), the feasibility reports in
[`feasibility/`](feasibility/), and the repository as it exists today. It defines
the contracts between connectors, synchronization, archive production, storage,
query surfaces, and delivery modes.

The documents have distinct roles:

| Document or artifact | Authority |
| --- | --- |
| `SPECIFICATIONS.md` | Product behavior, component boundaries, security requirements, and acceptance criteria. |
| [`SPEC.md`](SPEC.md) | Exact on-disk archive paths, bytes, and format-version rules. |
| [`schemas/`](schemas/) | Machine-readable shape of archive JSON documents. |
| [`tests/golden/`](tests/golden/) | Byte-exact archive conformance fixture. |
| [`plan/`](plan/) | Non-normative implementation sequencing and design input. |
| [`README.md`](README.md) | User-facing status and usage; it MUST distinguish shipping behavior from planned behavior. |

This document does not silently change archive version 1. If a requirement here
changes archive paths, schemas, or canonical bytes, the corresponding change
MUST first be made in `SPEC.md`, the schemas, and the golden fixture under the
format-version policy in `SPEC.md`.

The terms **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are used
as described by RFC 2119.

## 2. Product scope

### 2.1 Goals

Flat Email MUST be:

- **Ownable:** an archive is usable without a provider account or Flat Email
  service.
- **Preservation-first:** source message bytes are never discarded merely
  because they are malformed or inconvenient to parse.
- **Deterministic:** meaningful source changes create meaningful archive
  changes; clocks, map iteration, hostnames, and randomness do not create churn.
- **Idempotent:** retrying an import or sync is safe.
- **Portable:** generated paths work across Linux, macOS, Windows, Git, and
  object storage.
- **Offline-first:** individual messages and the archive's static browsing
  surface remain useful without a server.
- **Read-only at the source:** connectors do not send, delete, move, label, flag,
  or mark messages read.
- **Secure by default:** untrusted mail cannot execute script, trigger automatic
  remote requests, escape the archive root, or expose provider credentials.
- **Extensible:** connectors, storage destinations, APIs, and readers share one
  archive and one set of domain contracts.

### 2.2 Current baseline and target capabilities

| Capability | Status in this repository | Product requirement |
| --- | --- | --- |
| Local `.mbox` and Maildir import | Implemented; end-to-end coverage is incomplete | Retain and harden as the offline reference connector. |
| Archive producer, schemas, and golden fixture | Implemented | Remains the conformance baseline for every destination; CLI repeat behavior still requires hardening. |
| Per-message and per-thread HTML | Partially implemented | Bring into conformance with the reader and safety requirements below. |
| Gmail sync | Planned | First live provider connector. |
| Generic IMAP sync | Planned | Second live connector, sharing the connector and sync contracts. |
| Full-text search | Planned | Regenerable local index and CLI query surface. |
| Root reader and local HTTP API | Planned | Static small-archive browsing plus a secure scalable local service. |
| MCP server | Planned | Read-only, local, bounded access through the shared query layer. |
| Git destination | Planned | Filesystem archive followed by deterministic commits. |
| Object storage | Planned | S3-compatible first; GCS and Azure follow the same capability contract. |

### 2.3 Non-goals for the core product

The core product:

- is not a mail client and does not send, reply, forward, delete, archive, or
  mutate provider state;
- does not require a database to read an archive;
- does not promise logical deduplication when two sources return different bytes
  for the same human-visible message;
- does not make plaintext archives confidential merely because they are stored
  in Git or cloud storage;
- does not expose an unauthenticated remote mail API;
- does not require decryption or signature verification for S/MIME or PGP/MIME;
  and
- does not promise that a monolithic `file://` reader scales to every mailbox
  size.

## 3. System model

The product consists of the following layers:

```text
source mailbox or local store
  -> read-only connector
  -> connector-neutral account/message changes
  -> sync coordinator
  -> deterministic archive producer
  -> storage backend
  -> static readers / query layer / search index
  -> CLI, HTTP, and MCP adapters
```

Each layer has one responsibility:

- A **connector** translates provider or file-store concepts into source-neutral
  records. It does not write archive files.
- The **sync coordinator** owns cursors, retries, locking, update ordering, and
  failure recovery.
- The **archive producer** validates authoritative inputs and writes the
  `SPEC.md` layout. It does not authenticate to providers.
- A **storage backend** stores archive-root-relative objects and implements the
  publication guarantees required by the coordinator.
- The **query layer** reads a published archive through a read-only storage
  interface. HTTP and MCP are adapters over this layer, not independent archive
  parsers.
- A **search index** is an optional acceleration structure. It is never the
  source of truth.

## 4. Data authority and lifecycle

The statement that every file except `message.eml` is derivable from raw bytes is
not sufficient for live connectors. Labels, flags, provider IDs, provider
received dates, and provider thread IDs do not generally exist in the RFC 5322
message. The product therefore recognizes these data classes:

| Class | Examples | Required treatment |
| --- | --- | --- |
| Authoritative message content | The logical message bytes exposed by the source adapter | Stored as `message.eml`; immutable after successful publication. |
| Authoritative source facts | Internal received date, source references, label/folder membership, flags, provider IDs, provider thread ID | Preserved in archive metadata because a copied archive must not require the provider to retain them. |
| Stable archive facts | First archive observation, archive/account identity, format version | Set once or changed only by an explicit migration. |
| Derived archive data | Parsed headers, body files, decoded attachments, readers, thread and label indexes, catalog | Rebuildable from message content plus preserved source and archive facts. |
| Operational state | Provider cursors, last successful poll, retries, locks, staging data, known remote UID sets | Kept outside the archive data model; loss causes a safe full reconciliation. |
| Secrets | OAuth tokens, passwords, client secrets, cloud credentials | Never written into archive files or sync-state files. |

`metadata.json` is consequently a **mixed record**: some fields are byte-derived
and some are authoritative source or archive facts. A format migration MUST
preserve the latter while regenerating the former. `SPEC.md` and its schema MUST
identify the class and regeneration inputs of every field before live connector
work is considered conformant.

### 4.1 Message and source lifecycle

- Raw message content is append-only.
- Identical raw bytes within one account map to one message record. Multiple
  labels or IMAP occurrences reference that record.
- The same bytes in two accounts are two account-scoped records because labels,
  flags, source references, and retention state can differ.
- Different bytes always produce different records, even if their `Message-ID`
  headers or visible contents match.
- Label and flag state mirrors the latest authoritative provider state.
- A confirmed upstream deletion does not remove `message.eml`. It changes the
  preserved source-presence fact once; repeated observations of the same state
  MUST NOT cause further archive churn.
- Poll timestamps and “observed again” timestamps belong in operational sync
  state, not in otherwise unchanged per-message files.

## 5. Cross-cutting invariants

### CORE-001: Preservation

`message.eml` MUST contain the exact logical message bytes returned by the source
adapter. A source adapter MAY remove container framing, such as an mbox `From `
separator, and reverse container escaping, such as mbox `>From ` quoting. It MUST
NOT normalize line endings, repair headers, or reserialize MIME merely to make a
message valid. Malformed messages remain archivable.

### CORE-002: Determinism

Given the same:

1. authoritative message bytes;
2. authoritative source facts;
3. stable archive facts; and
4. archive format version,

a conformant producer MUST emit the same canonical archive bytes. The same
human-visible email fetched through different connectors is not the same input
when bytes or source facts differ.

Archive-creation facts such as `createdAt` are explicit stable inputs. A
repeated sync preserves them; a newly created archive may legitimately have
different creation facts.

### CORE-003: Idempotency

A successful sync followed by a sync that observes no authoritative changes
MUST produce zero archive-byte changes, zero Git commit, and no uploads of
already-identical objects. Actual changes to labels, flags, source presence, or
format version MAY update the affected metadata and derived indexes. Raw message
bytes MUST NOT be rewritten.

### CORE-004: Read-only sources

Provider access MUST use the least-privileged read-only scope or protocol
operation available. Tests MUST prove that a connector does not issue provider
mutation operations or trigger implicit `\Seen` changes.

### CORE-005: Published consistency

Only one writer may update an archive at a time. Cursors advance only after all
authoritative and required derived archive changes are durably published.
Readers and indexes consume the last published catalog, never in-progress
connector output.

### CORE-006: Safe paths

Every storage operation MUST receive a normalized, non-empty,
archive-root-relative POSIX path. Absolute paths, `.` or `..` components, NULs,
backslashes as separators, and paths that escape through symlinks MUST be
rejected at the storage boundary even when an upstream sanitizer has already
run.

### CORE-007: Account-scoped identity

The public identity of a message is `(accountKey, messageKey)`. A digest alone is
not a complete locator in a multi-account archive. Threads and labels are also
account-scoped. Search results, HTTP resources, MCP results, and sync changes
MUST carry `accountKey`.

### CORE-008: No hidden network dependency

Opening static archive files MUST NOT require or automatically contact a
provider, CDN, telemetry service, or localhost service. Network-backed
enhancements require an explicit user action.

## 6. Archive-contract hardening gates

`SPEC.md` remains the byte-level authority, but the following ambiguities and
contradictions MUST be resolved there, in the schemas, and in the golden fixture
before the affected feature ships:

| Area | Required resolution |
| --- | --- |
| Metadata authority | Classify fields individually. Preserve connector facts during regeneration instead of claiming `metadata.json` is derivable from raw bytes alone. |
| Observation timestamps | Preserve an immutable first-observed archive fact if desired; move unchanged-sync observation time out of per-message files. A stable source-presence transition may remain in archive metadata. |
| Raw validity | Replace the requirement that all raw messages be valid RFC 5322 with preservation-first language. Parsing failure must not authorize rewriting source bytes. |
| Determinism scope | Require byte identity for the same complete authoritative inputs, not for merely equivalent messages from different connectors. |
| Store-once scope | State explicitly that store-once applies within an account and is byte-level, not logical-email or cross-account deduplication. |
| Account identity | Define a portable `accountKey`, preserve the original address and aliases, reject collisions, and define account rename behavior. Raw account strings must not become paths without validation. |
| MIME part indexes | Define a zero-based or one-based scheme, traversal order, container treatment, malformed-subtree behavior, and placeholder width. |
| Body selection | Define a total algorithm for nested `multipart/alternative`, `multipart/related`, `message/rfc822`, calendar, signed, and encrypted parts. |
| Cryptographic mail | Preserve signatures and encrypted payloads; do not require keys or silently discard cryptographic parts. |
| Date parsing | Define the parsed portion of `Received`, timezone requirements, UTC comparison, plausible bounds, and out-of-range behavior. |
| Filename truncation | Pin the hash algorithm, input bytes, encoding, length, separator, extension placement, and final byte budget. |
| Portable names | Define Unicode case folding, Windows device-name handling, Unicode-normalization collisions, and a total generated-path budget. |
| Label lifecycle | Define deterministic collision order and whether renamed, removed, and empty labels remove, retain, or replace manifest and membership entries. |
| Attachment integrity | State that size and SHA-256 cover the transfer-decoded bytes written to disk. |
| Empty outputs | Define when `attachments.json`, label files, and other empty manifests exist; empty directories alone must never carry meaning. |
| Key promotion | Define locking, migration, reference rewriting, crash recovery, and the operational blast radius of archive-wide 16-to-32-byte key promotion. |
| Catalog scale | Define a compatible sharding/versioning path before static-reader and object-store assumptions make a monolithic catalog permanent. |
| HTML canonicalization | Publish a fixed sanitizer algorithm or allow-list and expand the golden fixture to cover every automatic-fetch and active-content class. A library choice alone is not a portable specification. |

Any correction that changes canonical paths, required fields, or bytes requires
the format-version treatment defined by `SPEC.md`.

## 7. Domain and component contracts

### 7.1 Domain records

The connector-neutral model MUST represent:

- an account key, original primary address, aliases, provider kind, and stable
  provider account identifier when available;
- raw message bytes;
- zero or more stable source references, because one IMAP message may occur in
  several folders with different UIDs;
- provider internal date;
- original label definitions and current memberships;
- normalized flags and preserved unknown provider flags;
- provider message and conversation identifiers when available; and
- whether a record is a full snapshot item, an addition, a metadata change, or a
  confirmed source removal.

The model MUST distinguish “unknown” from an explicitly empty value. It MUST NOT
use provider iteration order as canonical order.

### 7.2 Connector contract

A connector MUST support a full reconciliation when no valid cursor exists and
MAY support an incremental change feed. Its result contains:

- account facts and label definitions;
- complete message records for new content;
- metadata/source-reference changes for known content;
- confirmed source removals where the provider can prove them;
- an opaque candidate next cursor; and
- statistics and non-fatal per-item diagnostics.

The candidate cursor is not persisted by the connector. The sync coordinator
persists it only after archive publication succeeds.

Connector errors MUST identify whether they are:

- authentication or authorization failures;
- expired/invalid cursor failures requiring full reconciliation;
- retryable rate-limit or transient transport failures;
- permanent per-message parsing/fetch failures; or
- user/configuration failures.

Retryable operations use bounded exponential backoff with jitter and honor
provider retry hints. Cancellation MUST stop new work promptly. A sync that
cannot account for every source change MUST report incomplete status and MUST
NOT advance beyond the unresolved change.

### 7.3 Storage contract

The existing `Put`, `Exists`, `Read`, and `List` interface is sufficient for the
initial whole-archive filesystem producer but not for mirrored labels,
incremental publication, key migration, or remote concurrency. The target
storage contract MUST provide or explicitly emulate:

- read/stat and paginated or streaming list;
- immutable create with collision verification;
- atomic replacement of a single mutable object;
- deletion of obsolete derived objects;
- durable completion before a write reports success;
- writer locking or a conditional lease;
- conditional publication based on a known generation/ETag;
- staging plus publish/abort semantics, or an equivalent backend-specific
  protocol; and
- a flush barrier for bounded concurrent writes.

Backends MUST expose capabilities rather than silently weakening guarantees.
The producer MUST not assume that `List` is cheap, that a path is a native
filesystem path, or that an asynchronous `Put` is complete when it returns.

The local filesystem backend MUST use atomic same-filesystem replacement for
mutable files, reject symlink escapes, and create private archives by default.
Object stores MUST use conditional requests or generation IDs for the commit
point.

### 7.4 Incremental update and publication

An update follows this order:

1. Resolve the destination and acquire the archive writer lock.
2. Validate the manifest, archive version, account identity, and sync state.
3. Fetch a full snapshot or delta without mutating the source.
4. Validate and stage new raw messages and preserved source facts.
5. Recompute every affected per-message, label, thread, and catalog artifact.
6. Validate staged JSON, references, path containment, and key collisions.
7. Publish immutable content, then atomically replace mutable derived files.
8. Publish the new catalog/manifest generation last.
9. Persist the candidate provider cursor atomically.
10. Update or rebuild the optional search index from the published generation.
11. Release the lock and report statistics.

If any step before cursor persistence fails, the old cursor remains active.
Already-written immutable content may remain unreachable until the retry; it
must be verified and reused. The next run MUST reconcile and repair partial
derived output. A corrupt derived file is rebuilt; a conflict in authoritative
content fails loudly.

Static direct file browsing during an active sync cannot be a multi-file atomic
snapshot under the current layout. The CLI and services MUST prevent concurrent
serving/writing where consistency cannot otherwise be guaranteed, and this
limitation MUST be documented.

### 7.5 Sync-state contract

Sync state is versioned, provider-specific operational JSON containing:

- `stateVersion`;
- provider and account identity;
- destination/archive identity;
- the last successfully published archive generation;
- opaque provider cursor data;
- source-reference data required to detect changes or removals; and
- retry/reconciliation metadata that contains no message content or secrets.

For local archives, state MAY live at `.flat-email-state/` under the archive
root but is excluded from the archive manifest and export semantics. For remote
destinations it lives in the platform application-state directory, keyed by
destination and account. It MUST be written with private permissions and atomic
replacement.

Unknown state versions, corruption, or loss trigger a warning, quarantine of the
bad state, and a full reconciliation. They never authorize deletion of archived
messages.

### 7.6 Shared query contract

The transport-neutral query layer MUST offer read-only operations for:

- listing accounts;
- listing and retrieving account-scoped messages;
- retrieving threads and labels;
- retrieving body and attachment metadata;
- resolving safe attachment paths; and
- executing search with stable pagination.

It consumes catalog/schema versions explicitly and returns typed not-found,
invalid-argument, unsupported-version, and corruption errors. Its public message
locator always includes `accountKey`. HTTP and MCP MUST use this layer rather
than joining user input directly onto filesystem paths.

## 8. Input and connector requirements

### 8.1 Local `.mbox` and Maildir import

Local import is the reference offline connector.

- Input files and directories MUST be opened read-only.
- Mbox envelope separators are removed and mbox quoting is reversed; the
  remaining message bytes are preserved, including their line endings.
- Maildir message contents are preserved exactly. Filename flags and folder
  location become source facts, not edits to the message.
- Multiple input paths in one invocation are allowed.
- Identical raw messages within an account merge deterministically, including
  the union of source references and labels.
- A malformed individual message is preserved when its logical bytes can be
  isolated. An incomplete import returns a non-zero result with item diagnostics.
- End-to-end tests MUST exercise the CLI, mbox framing and quoting, Maildir
  flags, labels, filesystem writes, schemas, and repeat import behavior. Producer
  tests that construct `model.Input` directly are not sufficient.

The unused newline-normalization behavior currently described in
`internal/mailstore/mbox.go` is not part of this contract; normalization would
change message identity and must not be introduced without a format decision.

### 8.2 Gmail

The Gmail connector MUST:

- use OAuth with the least-privileged Gmail read-only scope;
- keep refresh tokens and client secrets in an OS credential store or an
  explicitly configured external secret provider;
- fetch the raw message representation and base64url-decode it without MIME
  reserialization;
- fetch label definitions and map system and user labels without losing original
  names or provider IDs;
- preserve Gmail message ID, thread ID, internal date, labels, and relevant
  normalized/unknown flags as source facts;
- page all list/history calls and apply bounded concurrency and quota-aware
  backoff;
- use Gmail History for deltas only after a successful full snapshot;
- fall back to full reconciliation when a history cursor expires; and
- retain raw content when Gmail reports deletion while applying a stable
  source-presence and label-state transition.

OAuth callback listeners bind loopback only, validate state/PKCE, have a bounded
lifetime, and do not log authorization codes or tokens.

### 8.3 IMAP

The IMAP connector MUST:

- require certificate-verified TLS by default;
- use `EXAMINE` rather than writable `SELECT` when supported and use
  `BODY.PEEK[]` for body retrieval;
- never issue `STORE`, `EXPUNGE`, `COPY`, `MOVE`, or another mutation command;
- use UID, UIDVALIDITY, and folder identity as source references;
- use CONDSTORE/QRESYNC and MODSEQ when available, with a full flags/UID
  reconciliation fallback when unavailable;
- not treat `UIDNEXT` alone as proof that flags and removals are unchanged;
- discard folder cursor state and reconcile that folder when UIDVALIDITY changes;
- map SPECIAL-USE folders and arbitrary folders to preserved label definitions;
- merge identical bytes found in several folders within the account while
  retaining every folder/UID source reference; and
- treat differing bytes as different messages even when `Message-ID` matches.

An insecure TLS override, if provided for diagnostics, MUST be conspicuously
named, require explicit invocation on every run, and emit a warning. It MUST
never become persisted default configuration.

## 9. Search and index

The index is derived acceleration state under `index/`; archive correctness does
not depend on it.

### SEARCH-001: Indexed content

The index includes account key, message key, date, sender and recipients,
subject, original and sanitized labels, flags, thread key, attachment presence,
and searchable body text. `body.txt` is preferred. When it is absent, the
indexer MUST derive plain searchable text from sanitized `body.html` without
changing archive source files. Attachment contents are not indexed in the core
release.

### SEARCH-002: Query behavior

The initial query language supports:

- bare terms, combined with AND by default;
- quoted phrases;
- `from:`, `to:`, `subject:`, and `label:`;
- `has:attachment`;
- `after:` inclusive and `before:` exclusive UTC date bounds; and
- `is:unread` and `is:starred`.

Malformed syntax or unknown operators produce a validation error rather than a
silently different query. Results are ordered by relevance, then date
descending, then account key and message key as total tie-breakers.

Search spans all accounts unless an account filter is supplied. Every result
contains `accountKey`, `messageKey`, date, sender, subject, labels, attachment
count, a plain-text snippet, and score/order information.

### SEARCH-003: Index lifecycle

- `flat-email index` builds a new index from one published archive generation.
- Sync updates the index only after archive publication.
- Missing, incompatible, or corrupt indexes are rebuilt automatically or on
  command and never make the archive unreadable.
- Rebuilding from the same archive MUST produce equivalent query results;
  byte-identical SQLite pages are not required.
- SQLite FTS5 is the planned initial implementation, not part of the archive
  interoperability contract.

## 10. Static readers and HTML safety

### 10.1 Per-message reader

`email.html` MUST be a valid, self-contained document containing:

- From, To, Cc, Bcc when present, Reply-To, Subject, and resolved date;
- current labels and normalized flags;
- the sanitized message body;
- attachment names, sizes, content types, and relative download links; and
- a visible indication when remote content was blocked.

The sanitized body inserted into `email.html` MUST be a fragment, not a nested
`html` document. Values derived from headers, labels, and filenames are escaped
as text or safe URL components.

### 10.2 Thread reader

A thread reader MUST show members in canonical thread order with sender, date,
subject, and a link to each message. If bodies are included, they use the same
sanitized fragments as message readers. A list of unlinked subjects alone does
not satisfy the reader contract.

### 10.3 Sanitization

Email HTML is hostile input. Canonical sanitization MUST:

- remove scripts, event handlers, forms, embedded browsing contexts, plugins,
  refresh directives, active URL schemes, and unsafe SVG/MathML features;
- remove or safely parse `<style>` elements and `style` attributes so `url()`,
  `@import`, and equivalent fetches cannot remain;
- neutralize automatic-fetch attributes including `src`, `srcset`, `poster`,
  `background`, media sources, CSS resources, and namespaced URL attributes;
- rewrite resolvable `cid:` references only to validated local attachment paths;
- prevent unresolved CID and remote resources from fetching;
- preserve external hyperlinks only as explicit user actions with safe
  opener/referrer behavior;
- emit no external script, stylesheet, font, media, or telemetry reference; and
- produce canonical output covered by byte-exact fixtures.

A restrictive Content Security Policy SHOULD provide defense in depth while
permitting only the inline reader style and validated local attachments needed
offline. Golden and browser tests MUST cover CSS tracking, `srcset`, `poster`,
legacy background attributes, SVG links, malformed markup, active schemes, and
CID handling, not only `<img src>`.

### 10.4 Root reader

The root `index.html`:

- loads the generated catalog through `catalog.js` when opened using `file://`;
- uses no external dependencies and makes no automatic network request;
- provides account, label, thread, and message-list navigation from catalog data;
- performs metadata-only filtering when no local service is running;
- opens per-message readers rather than injecting sender HTML into the mailbox
  shell; and
- shows a clear scale notice when the static catalog exceeds the supported
  threshold.

The initial static target is approximately 10,000 messages. This is a documented
product limit, not a data-loss limit. Larger archives remain directly readable
and use `flat-email serve` for pagination and full-text search.

A page opened from `file://` MUST NOT probe localhost automatically. When the
user explicitly runs `flat-email serve`, the server serves the reader from its
own origin and enables service-backed features there.

## 11. Local HTTP API

### 11.1 Security boundary

Mail is private even on localhost. The HTTP server MUST:

- bind loopback only in the core release;
- reject unexpected `Host` values and cross-origin requests;
- require an unguessable per-run authorization mechanism for archive data;
- avoid `Access-Control-Allow-Origin: *`;
- serve the enhanced reader from the same origin;
- set a restrictive Content Security Policy, `X-Content-Type-Options: nosniff`,
  and anti-framing policy;
- force attachments to download with a safe filename and content type; and
- disable raw-message retrieval unless the user explicitly enables it.

Read-only endpoints do not make wildcard CORS safe: an arbitrary web page must
not be able to read a user's mailbox through localhost. Non-loopback serving,
remote authentication, and TLS are outside the core API; the command MUST reject
such binding until those controls are specified and implemented.

### 11.2 Versioned resources

The first API is rooted at `/v1` and provides:

| Resource | Behavior |
| --- | --- |
| `GET /v1/health` | Process health only; does not reveal archive content. |
| `GET /v1/catalog` | Published catalog generation. |
| `GET /v1/accounts` | Account keys, display addresses, and counts. |
| `GET /v1/messages` | Stable paginated list; optional account, label, and date filters. |
| `GET /v1/accounts/{accountKey}/messages/{messageKey}` | Metadata and plain/sanitized body summary. |
| `GET .../raw` | Raw RFC message when explicitly enabled. |
| `GET .../html` | Sanitized body fragment. |
| `GET .../attachments/{onDiskName}` | Validated attachment download resolved through its manifest. |
| `GET /v1/accounts/{accountKey}/threads/{threadKey}` | Ordered thread members. |
| `GET /v1/accounts/{accountKey}/labels` | Label manifest and counts. |
| `GET /v1/accounts/{accountKey}/labels/{labelKey}/messages` | Stable paginated membership. |
| `GET /v1/search` | Search query with optional account scope. |

Message resources require both account and message keys. No handler may turn a
raw route parameter into a storage path.

List responses contain `data` and pagination metadata (`total`, `limit`,
`offset`). Default and maximum limits are enforced server-side. Errors use a
stable JSON object with a machine code and safe message and map to appropriate
HTTP status codes. Internal paths, message content, tokens, and stack traces are
not included in errors.

## 12. MCP server

`flat-email mcp` is a local stdio server over the shared query layer. It does not
depend on the HTTP server and has no network listener.

The core tools are:

| Tool | Required behavior |
| --- | --- |
| `list_accounts` | Account keys, display addresses, and counts. |
| `search_mail` | Structured, paginated search; optional account scope. |
| `get_message` | Requires account and message key; returns metadata and optionally bounded plain text. |
| `get_thread` | Requires account and thread key; returns ordered bounded summaries or bodies. |
| `list_labels` | Requires account; returns original names, label keys, and counts. |
| `get_messages_by_label` | Requires account and label key; returns stable pagination. |
| `get_attachment_info` | Requires account and message key; metadata only. |

Tool results MUST be structured JSON with the same account/message identity and
error semantics as the query layer. Default and hard maximum limits are enforced.
Included body text is capped at 4,000 Unicode characters and reports whether it
was truncated.

The MCP process:

- receives one immutable archive path at startup;
- exposes no tool that selects arbitrary paths;
- depends only on a read-only query interface;
- returns neither raw HTML nor attachment bytes;
- writes protocol output only to stdout and diagnostics only to stderr; and
- does not log query text, subjects, addresses, or body content by default.

Documentation MUST tell users that returned mail is disclosed to the assistant
connected to the MCP process. Write/action tools and remote MCP transports are
out of scope.

## 13. Delivery backends

### 13.1 Local filesystem

The local filesystem is the reference backend. It MUST:

- enforce root containment and symlink safety;
- use restrictive directory and file permissions by default;
- atomically replace mutable files;
- preserve POSIX-style archive paths independently of host separators;
- return sorted deterministic listings when ordering is requested; and
- produce the exact golden archive.

Consumer cloud-sync folders are treated as local filesystem destinations. Flat
Email does not integrate with Dropbox, OneDrive, or iCloud APIs merely to write
into their locally synchronized folders.

### 13.2 Git

Git delivery is orchestration around a successfully published local filesystem
archive, whether implemented as a wrapper or post-publish capability.

- `--git` is explicit and may initialize a repository in the selected archive.
- A commit occurs only after a successful archive publication.
- A no-change sync creates no commit.
- Commit content is deterministic; Git commit IDs and timestamps need not be.
- Commit summaries include provider, account, counts, date range, retained
  removals, and tool version without including message bodies or credentials.
- `.flat-email-state/` and `index/` are ignored. Canonical catalog and static
  reader files are tracked unless the user explicitly chooses a documented
  derived-file profile.
- Provider and Git remote credentials remain outside the repository.
- Push is never automatic. It requires an explicit remote option and a warning
  that the repository contains the user's mail and may expose secrets contained
  in the messages themselves.
- Push failure preserves the local commit and returns a detectable partial
  failure.

Per-sync commits are the default. Per-message commits, Git LFS automation, signed
commit configuration, and branch-per-account layouts are later extensions.

### 13.3 Object storage

The first remote implementation targets S3-compatible storage; `s3://`, `gs://`,
and `az://` identify S3, GCS, and Azure backends as they become available.

Cloud backends MUST:

- use standard SDK credential chains and keep credentials outside archive and
  sync-state objects;
- require authenticated TLS;
- refuse or prominently block a known-public destination by default;
- set accurate content types without changing object bytes;
- use bounded concurrency with a durable flush before publication;
- use conditional writes/generation checks for the catalog commit point;
- verify immutable content rather than treating existence alone as equivalence;
- replace mutable metadata and delete obsolete derived indexes when required;
- keep sync state local and key it by canonical destination plus account;
- prevent simultaneous unsupported writers to the same archive; and
- recover from interruption by reusing verified immutable objects and
  republishing a complete catalog.

Skipping uploads is an optimization, not an idempotency definition. Mutable
metadata cannot be skipped merely because an object exists.

Object storage does not provide client-side confidentiality by default. Provider
encryption at rest SHOULD be enabled, and documentation MUST state what the
provider can read. Client-side encrypted vaults and search over encrypted data
remain separate future work.

## 14. CLI contract

The target CLI contains:

| Command | Contract |
| --- | --- |
| `flat-email import` | Import one or more local mbox/Maildir sources into an account. |
| `flat-email auth <provider>` | Establish or replace provider credentials without writing them to the archive. |
| `flat-email sync` | Run full or incremental read-only provider synchronization. |
| `flat-email index` | Build or rebuild the optional local search index. |
| `flat-email search` | Query an index with human or JSON output. |
| `flat-email serve` | Serve the secure loopback reader/API. |
| `flat-email mcp` | Run the local stdio MCP server. |

Common behavior:

- Usage/configuration errors exit with status 2; complete success with 0;
  operational, incomplete, auth, or remote-push failures return non-zero.
- Human progress and diagnostics go to stderr. Machine-readable JSON goes to
  stdout only when requested.
- Secrets, authorization codes, raw bodies, and sensitive query text are not
  printed in normal logs.
- `--dry-run` performs discovery and reports intended changes without writing an
  archive, state, index, Git commit, or cloud object.
- Destructive or insecure overrides are never implied by configuration defaults.
- Commands validate archive versions before reading or writing and refuse an
  unsupported version rather than guessing.

## 15. Security and privacy requirements

The threat model includes hostile message bytes and HTML, malicious filenames and
headers, crafted archive/catalog contents, untrusted route/tool parameters,
compromised or surprising provider responses, malicious websites targeting a
localhost API, local symlink attacks, and accidental publication to Git/cloud.

### SEC-001: Credentials

- OAuth refresh tokens and IMAP passwords live in OS credential stores or an
  explicitly configured external secret provider.
- Cloud and Git credentials use their standard external credential mechanisms.
- Archive files, indexes, catalogs, state, logs, commit messages, and diagnostics
  MUST NOT contain credentials introduced by Flat Email.
- Automated tests use injected fake credentials and never require live secrets.

### SEC-002: Local data protection

- Newly created local archives and state use owner-only permissions by default,
  subject to an explicit user override.
- Temporary and staged files receive the same protection and are cleaned after
  recovery.
- Storage follows no untrusted symlink that can escape the selected root.

### SEC-003: Untrusted content

- MIME and HTML parsing use explicit limits for message size, header count/size,
  MIME depth and part count, decoded attachment size, and aggregate work.
- Exceeding a derivation limit preserves raw content, records a safe diagnostic,
  and does not execute or guess at content.
- Attachments are never executed or rendered inline based solely on sender MIME
  type.
- HTML follows §10 and is tested with adversarial fixtures and fuzzing.

### SEC-004: Network behavior

- Connectors contact only their configured provider endpoints and documented
  OAuth endpoints.
- Static readers make no automatic outbound request.
- The local API follows §11 and is not remotely exposed.
- The product has no telemetry by default.

### SEC-005: Logging and publication

- Logs use stable IDs and counts where possible, not subjects, addresses, label
  names, bodies, tokens, or attachment contents.
- Git/cloud commands warn that the archive itself is sensitive even when Flat
  Email credentials are absent.
- Remote push/upload targets are explicit and never inferred from an email
  address or provider account.

## 16. Reliability, portability, and scale

### 16.1 Recovery

- Loss of sync state causes a full, idempotent reconciliation.
- Loss or corruption of a derived file causes deterministic regeneration.
- Loss or conflict of authoritative raw content is a hard integrity error and is
  never repaired by silently fetching different bytes over it.
- Index corruption never blocks direct archive access.
- Cursor persistence and catalog publication are write-last operations.
- A cancelled or interrupted run returns non-success and is safe to retry.

### 16.2 Concurrency

Only one writer is supported per archive. A daemon, CLI, Git wrapper, and cloud
writer all share the same lock/lease. Concurrent read services either consume a
stable published generation or pause during a local update. Multi-writer merge
is out of scope.

### 16.3 Memory and throughput

- Initial and incremental provider sync MUST support bounded message-fetch and
  write concurrency.
- A large mailbox MUST NOT require all raw message bodies to be held in memory at
  once.
- Backend listings and search indexing are streamed or paginated.
- Rate and concurrency settings have safe defaults and bounded maxima.
- Performance optimizations must not weaken deterministic output, path safety,
  read-only behavior, or publication ordering.

### 16.4 Portability

Generated archives MUST be tested on case-sensitive and case-insensitive
filesystems and against Windows reserved names and path limits. Paths in JSON use
`/` regardless of host OS. Readers MUST reject malformed catalog paths rather
than normalizing them into a different resource.

The writer MUST define and test the consequences of macOS filename normalization.
If a target filesystem cannot round-trip a generated name safely, production
fails before publication rather than silently changing the path.

## 17. Verification and acceptance

### 17.1 Existing conformance base

The byte-exact fixture and JSON-schema tests remain mandatory. Every archive
format correction adds the smallest fixture that demonstrates the rule.
Canonical archive output is compared by path and byte, not by parsed semantic
equivalence.

### 17.2 Required test layers

| Area | Minimum acceptance tests |
| --- | --- |
| Archive producer | Golden bytes, schemas, repeat production, key collision/migration, malformed input, deterministic ordering. |
| Local import | End-to-end CLI tests for mbox, Maildir, flags, quoting, duplicate bytes, malformed messages, and no source modification. |
| Paths/storage | Traversal, absolute paths, backslashes, Unicode/case collisions, Windows names, symlink escape, atomic replacement, interrupted publication. |
| HTML/readers | CSS and attribute tracking vectors, active content, malformed HTML, CID images, valid document structure, labels, attachments, and thread links. |
| Gmail | Recorded HTTP fixtures for full/delta sync, labels, quota retry, cursor expiry, deletion, auth failure, and zero mutation calls. |
| IMAP | In-process server tests for `EXAMINE`, `BODY.PEEK`, flags, duplicate folders, UIDVALIDITY reset, reconciliation, and absence of mutation commands. |
| Sync | No-change zero diff, label/flag transition, retained deletion, crash before/after publish, corrupt/lost state, concurrent writer rejection. |
| Search | Query parser, account scope, stable ranking/pagination, HTML-only body text, corrupt/missing rebuild, equivalent results after rebuild. |
| HTTP | Auth, Host/Origin rejection, pagination, account-scoped lookup, traversal attempts, raw opt-in, attachment download headers, no wildcard CORS. |
| MCP | Tool schemas, limits, account scope, truncation, fixed archive root, no write calls, no raw HTML/attachment content. |
| Git | Initial commit, no-change no commit, exact diff, ignored state/index, push failure, no Flat Email credentials. |
| Cloud | Backend contract against emulators, conditional publish, interrupted upload, stale object recovery, private-target checks, no-op upload behavior. |
| Browsers | `file://` static reader in supported Chrome, Firefox, and Safari versions; served-reader same-origin mode. |

Fuzzing SHOULD cover MIME parsing, address/header decoding, filename
sanitization, catalog parsing, and API path validation.

### 17.3 Feature definition of done

A feature is complete only when:

1. its behavior satisfies this document and the applicable archive format;
2. security and failure cases have automated tests;
3. repeated no-change operation produces no archive churn;
4. user documentation shows runnable commands and accurate status;
5. planned examples are not presented as shipping features;
6. schemas and golden fixtures are updated when archive bytes change; and
7. recovery from interruption and lost operational state is demonstrated.

## 18. Delivery sequence and release gates

The plan files remain useful implementation breakdowns, but their shared
foundations must land before provider-specific work.

1. **Archive hardening gate**
   - Resolve §6 in `SPEC.md`, schemas, and golden fixtures.
   - Fix path containment, HTML remote-fetch vectors, reader completeness, and
     local-import end-to-end coverage.
2. **Sync foundation**
   - Finalize account/source-reference model, connector result contract,
     versioned state, locking, incremental update, publication, and statistics.
3. **First live connectors**
   - Gmail full sync, then Gmail delta.
   - IMAP full sync, then reliable flag/removal reconciliation.
4. **Read foundation**
   - Search/index plus the shared query layer.
5. **Reader and integrations**
   - Static root reader and secure local HTTP service.
   - MCP tools over the same query layer.
6. **Delivery modes**
   - Git orchestration over local filesystem output.
   - S3-compatible storage, followed by GCS and Azure implementations.

Plans may proceed in parallel only when their listed contracts are stable. Git
can be prototyped with local import, but remote push does not bypass the archive
hardening and privacy gates.

## 19. Explicitly deferred work

The following ideas are valid but are not commitments of the core sequence:

- Outlook/Microsoft Graph, JMAP, POP3, Yahoo, Proton Bridge, and additional
  provider connectors;
- Apple Mail, Thunderbird profile, PST, and OST import;
- push notifications, webhooks, SMTP ingest, and always-on streaming;
- attachment-content extraction and external search engines;
- catalog/object packing and large-archive sharding beyond the compatibility
  requirements above;
- client-side encrypted vaults, key rotation, and encrypted search;
- remotely accessible HTTP or MCP servers;
- reply, send, delete, move, or label-mutation tools;
- daemon and container products beyond documented scheduler use;
- a public SDK before the archive and internal package contracts are stable;
- cross-platform desktop packaging, bespoke native apps, and mobile sync; and
- multi-writer archive merge.

When push ingest is eventually added, it acts only as a hint to run normal
incremental reconciliation. It is never the sole record of provider changes.

## 20. Traceability

| Source | Requirements carried forward |
| --- | --- |
| [`MISSION.md`](MISSION.md) | Ownership, portability, durability, offline use, and compatibility with trusted tools. |
| [`SUGGESTIONS.md`](SUGGESTIONS.md) | Layout-first design, determinism, source/derived separation, reader constraints, security, narrow milestones, and golden tests. |
| [`SPEC.md`](SPEC.md) | Current archive layout and canonical byte contract. |
| [`BRAINSTORM.md`](BRAINSTORM.md) | Metadata authority, observation churn, malformed raw mail, deterministic edge cases, portability, scale, and lifecycle gaps. |
| [`CRITIQUE.md`](CRITIQUE.md) | Actual implementation status, HTML remote-fetch gaps, incomplete readers, import coverage, path traversal, dead normalization code, and HTML-only searchability. |
| [`plan/README.md`](plan/README.md) | Dependency graph and shared incremental, query, statistics, and scoping foundations. |
| [`plan/01-gmail-connector.md`](plan/01-gmail-connector.md) | Gmail auth, mapping, history, retries, and sync flow. |
| [`plan/02-imap-connector.md`](plan/02-imap-connector.md) | Read-only IMAP operations, UID state, folders, flags, and reconciliation. |
| [`plan/03-search.md`](plan/03-search.md) | Regenerable local index, query language, ranking, and CLI. |
| [`plan/04-http-api.md`](plan/04-http-api.md) | Root reader, query API, routes, pagination, and scale fallback, with strengthened localhost security. |
| [`plan/05-mcp-server.md`](plan/05-mcp-server.md) | Read-only bounded tools over a shared query layer. |
| [`plan/06-email-2-repo.md`](plan/06-email-2-repo.md) | Per-sync Git history, ignore rules, statistics, and explicit push. |
| [`plan/07-email-2-cloud.md`](plan/07-email-2-cloud.md) | Backend URL schemes, cloud SDKs, concurrency, content types, and local state. |
| [`feasibility/`](feasibility/) | Foundational filesystem priority, staged delivery, and explicit deferral of high-risk packaging, encryption, and push modes. |

## 21. Glossary

- **Account key:** Portable archive identifier for one source account; not
  necessarily the display email address.
- **Archive generation:** One successfully published, internally consistent
  catalog view.
- **Authoritative input:** Message bytes or preserved source/archive facts that
  cannot be regenerated without loss.
- **Connector:** Read-only adapter from a provider or local store to neutral
  account/message changes.
- **Derived file:** Archive data reproducible from authoritative inputs and the
  format version.
- **Message key:** Content digest defined by `SPEC.md`; used together with an
  account key for public lookup.
- **Operational state:** Replaceable local state used for efficient sync, never
  the sole copy of mail or credentials.
- **Published catalog:** Commit point readers use to discover the current
  archive.
- **Source reference:** Provider-specific locator for one occurrence of a
  message, such as Gmail ID or IMAP folder/UID/UIDVALIDITY.
- **Storage backend:** Archive-root-relative object store implementing the
  required safety and publication capabilities.
