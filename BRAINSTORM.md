# Brainstorm: SPEC.md hard-look review

This document captures a hard-look review of [`SPEC.md`](SPEC.md): what is
already strong, where the current draft contradicts itself, and what should be
made clearer before the format becomes a long-lived contract.

The spec is directionally strong. It correctly treats the archive layout as the
product, makes determinism a first-class property, separates raw mail from
derived files, and anchors future execution modes on one portable on-disk
primitive. The remaining risks are not about vision. They are about making sure
the written contract is precise enough that two implementations, storage modes,
and future connectors do not accidentally diverge.

## Priority 1: resolve internal contradictions

These issues undercut the headline guarantees of determinism, idempotency, and
regenerability. They should be fixed before the spec is treated as stable.

### 1. `metadata.json` is not fully derived from `message.eml`

**Current tension**

`SPEC.md` classifies `metadata.json` as derived from `message.eml` plus the spec,
and says derived files can be re-rendered from the raw message. But
`metadata.json` contains values that are not present in the raw RFC 5322 bytes:

- provider internal dates (`internalDate`, IMAP `INTERNALDATE`, Graph
  `receivedDateTime`)
- provider message and thread IDs
- provider flags
- label membership
- connector-selected thread keys
- `firstSeen` and `lastSeen`

That means a format upgrade cannot actually re-render a complete `metadata.json`
from only `message.eml`.

**Why it matters**

This breaks the authoritative-vs-derived model. It also makes it unclear what is
preserved if a user copies only the archive files, deletes sync state, or imports
the archive with a different implementation.

**Action**

Choose one of these models explicitly:

1. Split `metadata.json` into:
   - byte-derived metadata, rebuildable from `message.eml`
   - provider/archive metadata, preserved as authoritative archive state
     
ANSWERED!!! 2. Keep one `metadata.json`, but stop describing it as purely byte-derived.
   
4. Add a separate authoritative sidecar for connector facts, and derive
   `metadata.json` from `message.eml` plus that sidecar.

The spec should say exactly which files are authoritative and which inputs are
required to regenerate each derived file.

### 2. `lastSeen` conflicts with idempotency and Git-friendly output

**Current tension**

The spec says a message already present at its `<message-key>` path is never
rewritten unless `specVersion` changes. But `lastSeen` is defined as the most
recent sync timestamp that observed the message upstream.

If `lastSeen` is updated on every sync, `metadata.json` changes on every sync. If
it is not updated, the field does not mean what it says.

**Why it matters**

This creates noisy diffs, especially for the Git-backed archive mode, where the
spec is trying to ensure commits show real mail changes rather than sync churn.

**Action**

ANSWERED!!!

Move volatile sync-observation fields out of per-message derived files, or weaken
the idempotency guarantee to explicitly allow sync-marker churn. The cleaner
model is:

- keep stable archive metadata in the archive
- keep volatile sync observations in `.flat-email-state/`
- make per-message files stable unless message data, labels, or the format
  version actually change

### 3. Raw-byte preservation conflicts with "`message.eml` MUST be valid RFC 5322"

**Current tension**

The spec treats `message.eml` as authoritative raw bytes and derives
`message-key` from those bytes. It also says `message.eml` must be valid RFC
5322.

Real-world mail is often malformed: bare LF, invalid encodings, overlong lines,
obsolete syntax, or provider-specific quirks. Repairing a message to make it
valid changes the bytes. Storing it verbatim may preserve invalid RFC 5322.

**Why it matters**

The archive needs a clear data-preservation policy. If raw bytes are the source
of truth, validity cannot be guaranteed for every imported message.

**Action**

ANSWERED!!!

Prefer an explicit preservation rule:

- `message.eml` stores the exact bytes received from the source whenever possible.
- Derived files must tolerate malformed input.
- Valid RFC 5322 is a goal or expectation, not a requirement that permits
  rewriting source bytes.
- If any connector canonicalizes or repairs raw mail, it must document that
  behavior because it changes message keys.

### 4. Cross-implementation byte identity is overstated

**Current tension**

The spec says two conformant implementations given the same input messages must
produce byte-identical derived files. But connector-specific fields and provider
metadata mean two connectors may have different data for the same logical
message.

For example, Gmail, IMAP, mbox, and Graph may disagree on internal date, provider
IDs, flags, labels, thread IDs, and even raw message bytes.

**Why it matters**

The byte-identity guarantee is valuable, but only if its scope is precise.

**Action**

Clarify the guarantee:

- byte-identical output is required for the same authoritative inputs
- connector-supplied facts are part of those inputs
- the same logical email fetched through different connectors may produce
  different archive records

## Priority 2: tighten determinism holes

These issues may let two reasonable implementations produce different outputs
while both appear to follow the spec.

### 5. Define MIME part indexes precisely

**Current gap**

The spec references MIME part indexes for unnamed attachments, attachment
ordering, and manifests, but does not define the numbering scheme.

**Action**

Specify:

- whether indexes are flat counters or hierarchical paths
- traversal order
- whether multipart container nodes receive indexes
- zero-padding width for `part-<NN>`
- how malformed MIME affects numbering

### 6. Define label collision ordering

**Current gap**

Attachment collision handling is deterministic because MIME part order is
defined. Label collision handling is not: if two provider labels sanitize to the
same filename, the spec does not say which gets the base name and which gets
` (1)`.

**Action**

Define a canonical label ordering before assigning collision suffixes. Candidate
tie-breakers:

1. provider label ID, if present
2. original label name after Unicode normalization
3. label type
4. stable source order only if the connector defines it

Avoid relying on provider iteration order unless it is explicitly part of the
connector contract.

### 7. Fully specify filename truncation hashes

**Current gap**

The sanitizer says long names get a short hash of the original, but does not
define the hash algorithm, length, encoding, or exact placement.

**Action**

Specify:

- hash algorithm, likely SHA-256
- number of bytes/chars retained
- separator format
- whether the hash is computed before or after Unicode normalization
- how to preserve file extension readability while staying under 255 bytes

### 8. Define Unicode case folding for collision checks

**Current gap**

The spec says sanitized label and attachment names are compared
case-insensitively, but Unicode case-insensitive comparison has edge cases.

**Action**

Choose and document a specific comparison:

- simple ASCII case-fold only, or
- Unicode simple case folding, or
- Unicode full case folding

For maximum portability, also consider normalizing and comparing against known
filesystem behavior on Windows and macOS.

### 9. Make body-selection precedence total

**Current gap**

"Richest available alternative" is not enough for complex messages containing
`multipart/alternative`, `multipart/related`, calendar parts, signed parts, or
nested messages.

**Action**

Define a total precedence order for body selection, including how to handle:

- `multipart/alternative`
- `multipart/related`
- nested multiparts
- `text/calendar`
- `message/rfc822`
- signed and encrypted multiparts

### 10. Pin down `Received:` date parsing

**Current gap**

The bucket date fallback uses the earliest valid `Received:` timestamp, but
`Received:` headers are messy and can contain invalid or ambiguous date text.

**Action**

Define:

- which part of the header is parsed
- what "valid" means
- whether timestamps without timezone are rejected
- how comments and obsolete syntax are handled
- whether "earliest" means chronological earliest after UTC normalization

## Priority 3: cover real email edge cases

These are common enough in real mailboxes that the spec should address them
before connectors multiply.

### 11. Treat `message/rfc822` attached messages explicitly

**Current gap**

If body extraction walks every MIME leaf depth-first, it may descend into an
attached email and choose the attached message's body as the parent message body.

**Action**

State that `message/rfc822` parts are attachments by default and are not searched
for the parent message body unless a future spec version defines nested-message
extraction.

### 12. Define behavior for S/MIME and PGP/MIME

**Current gap**

Signed and encrypted email formats are common, but the spec does not say how
`multipart/signed`, `multipart/encrypted`, `application/pkcs7-mime`, or PGP/MIME
should be represented.

**Action**

For v0.1, define conservative behavior:

- preserve raw bytes
- treat cryptographic payloads/signatures as attachments unless decoded by an
  explicit, documented feature
- do not silently discard signatures
- do not require private keys to produce a conformant archive

### 13. Clarify malformed MIME recovery

**Current gap**

The spec says broken MIME becomes an opaque `application/octet-stream`
attachment. That is safe, but may be too broad for partially parseable messages.

**Action**

Define whether recovery is all-or-nothing for the whole message or scoped to the
broken subtree. This matters for deterministic extraction from partially valid
messages.

## Priority 4: strengthen portability claims

The spec already handles many path issues, but a few OS and Git edge cases remain.

### 14. Handle Windows reserved device names

**Current gap**

Windows rejects names such as `CON`, `NUL`, `AUX`, `PRN`, `COM1`, and `LPT1`,
including some extension variants.

**Action**

Add these to the sanitizer rules for labels and attachment names.

### 15. Address total path length

**Current gap**

The sanitizer limits individual path components to 255 bytes, but total paths can
still exceed Windows path limits, especially with long account names, deep date
paths, full 64-char keys, and long attachment names.

**Action**

Either:

- define a conservative total path budget for generated paths, or
- explicitly require long-path-aware environments on Windows, or
- shorten some path components and document the trade-off

### 16. Specify empty directory and empty manifest behavior

**Current gap**

Git does not preserve empty directories. The spec should avoid requiring empty
directories as meaningful archive state.

**Action**

State whether these exist when empty:

- `attachments/`
- `attachments.json`
- label membership files with no messages
- `threads/`
- `index/`

Prefer making files, not empty directories, carry meaning.

### 17. Account for macOS Unicode filename normalization

**Current gap**

The sanitizer emits NFC, but some macOS filesystems may normalize filenames
differently.

**Action**

Clarify whether readers should treat filename Unicode normalization differences
as equivalent when reconciling manifests to files, or whether archive writers
must verify round-trip filename preservation.

## Priority 5: revisit scale and operational blast radius

The spec is clean for small archives, but some rules may become expensive at
mailbox scale.

### 18. Monolithic `catalog.json` may become a churn hotspot

**Current tension**

`catalog.json` lists every message in the archive. For large archives, every sync
may rewrite a large file even when only a few messages changed.

**Why it matters**

This can hurt Git diffs, cloud sync, and `file://` reader performance.

**Action**

Add catalog sharding or pagination to the future-items list, or define it now
before reader assumptions harden. Possible direction:

- root catalog contains accounts, counts, and shard pointers
- per-account or date-bucket catalogs contain message summaries
- `catalog.js` mirrors the same sharded structure for `file://`

### 19. Key-length promotion rewrites the whole archive

**Current tension**

On truncated key collision, the spec promotes the entire archive from 16-byte to
32-byte keys. This is deterministic and safe, but operationally huge.

**Action**

Document the blast radius:

- every message directory may be renamed
- label and thread references must be rewritten
- catalog and reader links change
- Git/cloud modes may see a massive rename-heavy update
- sync should lock the archive during promotion

Also consider whether full-length keys should be an opt-in mode for large or
multi-source deployments.

## Priority 6: clarify archive semantics and lifecycle

### 20. Be explicit about logical deduplication limits

**Current tension**

The spec says content-addressing is de-duplicating, but that means byte-level
deduplication only. The same logical message can produce different bytes through
different providers or import paths.

**Action**

Clarify that:

- identical bytes produce one message key
- same logical email with different bytes produces different message keys
- cross-account deduplication is not guaranteed unless explicitly added later

### 21. Define label deletion and empty-label semantics

**Current gap**

Labels mirror upstream state, but the spec does not say what happens when a label
is removed upstream or becomes empty.

**Action**

Specify whether the label file and manifest entry are removed, retained as empty,
or retained with a tombstone/history marker.

### 22. Define plausible date bounds

**Current gap**

The fallback rules handle missing or invalid dates, but not valid-looking dates
that are absurd for filesystem organization.

**Action**

Define a supported date range. Dates outside that range can either use
`unknown-date/` or a separate policy such as `out-of-range-date/`.

### 23. Clarify attachment hashing

**Current gap**

The attachment manifest records SHA-256 of the payload, but should explicitly say
whether this is before or after transfer decoding.

**Action**

State that attachment hashes are computed over the decoded bytes written to disk.

### 24. Define account identity and aliases

**Current gap**

Account directories use the normalized primary address, but providers can expose
aliases, delegated mailboxes, shared mailboxes, and renamed accounts.

**Action**

Define:

- what "primary address" means
- how aliases are recorded
- whether account renames migrate paths or create new accounts
- how two accounts with the same primary address from different providers are
  disambiguated

## Suggested immediate edit plan for `SPEC.md`

1. Fix the authoritative/derived classification:
   - separate byte-derived files from provider/archive-state files
   - resolve the `firstSeen`/`lastSeen` contradiction
2. Narrow the byte-identical guarantee to "same authoritative inputs."
3. Replace "`message.eml` MUST be valid RFC 5322" with a preservation-first raw
   byte policy.
4. Add deterministic definitions for:
   - MIME part indexes
   - label collision ordering
   - truncation hashes
   - Unicode/case collision comparison
5. Add explicit handling for:
   - `message/rfc822`
   - S/MIME and PGP/MIME
   - Windows reserved names
   - empty directories/manifests
6. Add scale-focused future items:
   - sharded catalogs
   - full-key mode
   - object-storage packing

## Recommended v0.1 stance

For v0.1, prefer conservative, preservation-first rules:

- raw message bytes are authoritative even when malformed
- provider-supplied facts are preserved explicitly, not pretended to be derived
- volatile sync state does not churn stable per-message files
- malformed, encrypted, signed, or nested content is preserved rather than guessed
- deterministic behavior beats clever extraction

That keeps the core promise intact: an archive that is portable, durable,
inspectable, and safe to regenerate without hidden provider state or
implementation-specific guesswork.
