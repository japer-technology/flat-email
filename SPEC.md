# Flat Email Archive Layout Specification

**Spec version:** `1` &nbsp;·&nbsp; **Status:** Draft &nbsp;·&nbsp; **Stability:** unstable until v1.0

This document is the canonical contract for the Flat Email on-disk format. It
exists because — as argued in [`SUGGESTIONS.md` §2](SUGGESTIONS.md) — **the layout
is the product**. Every execution mode in [`IDEAS.md`](IDEAS.md) (Git backend,
cloud storage, desktop apps, MCP, connectors) is a delivery variation on this one
primitive, so the format must be nailed down *before* connectors are written.

The README sketch shows the shape of an archive; this spec pins down the
load-bearing details the sketch leaves open: how messages are dated and named,
how labels and attachments are handled, how the format behaves across operating
systems, and how it is versioned. It also states the determinism and
source-of-truth guarantees that make the mission words **portable**, **durable**,
and **ownable** literally true.

This spec is an **implementation contract**, not a description: two conformant
implementations, given the same input messages, MUST produce byte-identical
derived files. Where a rule below could otherwise be read two ways, the binding
interpretation is the one that removes ambiguity. The key words **MUST**,
**MUST NOT**, **SHOULD**, and **MAY** are used as in RFC 2119.

Machine-readable schemas for the JSON files defined here live in
[`schemas/`](schemas/) and are versioned alongside this document; a tiny
conformance fixture lives in [`tests/golden/`](tests/golden/) (§9).

---

## 1. Goals and non-goals of the format

**Goals**

- **Deterministic** — the same input message always produces the same files and
  bytes, regardless of when, where, or how many times sync runs.
- **Self-describing** — an archive declares its own format version and can be
  read with no software beyond a file browser and a text editor.
- **Portable** — an archive copies losslessly between Linux, macOS, and Windows
  filesystems, and between local disk, Git, and object storage.
- **Store-once** — message bodies and attachments are never duplicated, no matter
  how many labels, folders, or threads reference them.
- **Regenerable** — every file except the raw message is rebuildable from the raw
  message plus this spec.

**Non-goals**

- Not a mail client; the format does not send, reply to, or modify mail.
- Not a database; readers must never *require* an index to read the data.
- Not a wire protocol; this describes files at rest, not an API.

---

## 2. Format versioning

Every archive root contains a `flat-email.json` manifest declaring the layout
version:

```json
{
  "format": "flat-email",
  "specVersion": 1,
  "messageKeyBytes": 16,
  "createdBy": "flat-email/<tool-version>",
  "createdAt": "2024-01-15T00:00:00Z"
}
```

- `specVersion` is an **integer** that increments only on a breaking layout
  change. Readers MUST refuse (or explicitly downgrade-handle) an archive whose
  `specVersion` they do not understand.
- `messageKeyBytes` is the number of SHA-256 prefix bytes used for
  `<message-key>` and `<thread-key>` digests (default `16`; promoted to `32` on
  collision, §4.2). Readers use it to know the key length without guessing.
- `createdBy`/`createdAt` are informational and are the **only** place a
  generation timestamp is allowed to appear (see §6 — they live in the manifest,
  never inside per-message derived files, so they cannot pollute diffs).
- A format upgrade is defined as: bump `specVersion`, then re-render all derived
  files from the authoritative `message.eml` bytes (§5). Raw messages are never
  rewritten by an upgrade.

---

## 3. Directory layout

```
my-archive/
  flat-email.json                  # archive manifest (format + specVersion)
  catalog.json                     # derived: machine-readable archive index (§11)
  catalog.js                       # derived: catalog.json wrapped for file:// readers (§11)
  accounts/
    me@example.com/
      messages/
        2024/01/15/<message-key>/
          message.eml              # authoritative: raw RFC 5322 bytes
          metadata.json            # derived: headers, labels, thread id, flags (§10)
          body.txt                 # derived: plain-text body (§12)
          body.html                # derived: sanitized HTML body (§12, §13)
          email.html               # derived: standalone reader page (§13)
          attachments/
            <sanitized-name>       # derived: decoded attachment payloads
            attachments.json       # derived: attachment manifest (§4.4)
      threads/
        <thread-key>.json          # derived: ordered message keys in thread
        <thread-key>.html          # derived: standalone thread reader
      labels/
        labels.json                # derived: label manifest (§4.7)
        <sanitized-label>.json     # derived: message keys carrying this label
  index/                           # derived: regenerable search index
  index.html                       # derived: serverless web-app entry point
  .flat-email-state/               # NON-archive: regenerable sync state (§14)
```

All paths under `accounts/<account>/` use the account's normalised primary
address as the directory name (lowercased; see §7 on case).

The `.flat-email-state/` directory is **not part of the archive format**: it
holds regenerable per-account sync cursors only (§14), never message data and
never credentials. Readers MUST ignore it; backup/Git/cloud modes MAY exclude
it.

---

## 4. Resolving the load-bearing questions

These are the open questions raised in `SUGGESTIONS.md` §2, each now given a
binding rule.

### 4.1 Which date buckets a message?

The path `messages/YYYY/MM/DD/` is derived from a single, well-defined
**bucket date**, chosen in this priority order:

1. The provider's **internal received date** when the connector exposes one
   (Gmail `internalDate`, IMAP `INTERNALDATE`, Microsoft Graph
   `receivedDateTime`). This is stable and not sender-controlled.
2. Otherwise the earliest valid `Received:` header timestamp.
3. Otherwise the `Date:` header.
4. Otherwise the sentinel bucket **`unknown-date/`** (a literal path segment in
   place of `YYYY/MM/DD`).

The chosen date is **normalised to UTC** before bucketing, so the same message
buckets identically on any machine in any timezone. The resolved value and its
source are recorded in `metadata.json` (`date`, `dateSource`). The bucket date
never changes for a message once written, even if a later, "better" source
becomes available — this preserves idempotency (§6).

### 4.2 How is `<message-key>` derived?

The RFC `Message-ID` header is **not** used as a filename: it is optional, not
guaranteed unique, and may contain characters illegal in paths. Instead the
message key is a deterministic, filesystem-safe digest of the **authoritative raw
bytes**:

```
message-key = first 16 bytes (32 lowercase hex chars) of SHA-256(message.eml bytes)
```

- Content-addressed, so re-syncing an unchanged message yields the same key →
  sync is idempotent and de-duplicating.
- Pure hex (`[0-9a-f]`), so it is legal on every target filesystem and safe under
  case-insensitive matching (§7).
- The full Message-ID header, when present, is preserved verbatim inside
  `metadata.json`; only the *filename* is the digest.
- Truncation to 16 bytes keeps paths short; collision probability is negligible
  for personal-mailbox scales. Connectors MAY use the full 32-byte digest by
  declaring it in the manifest if a deployment requires it.

**Collision handling.** A collision is two *distinct* `message.eml` byte
sequences that share the same truncated key. It MUST be handled deterministically,
never by silent overwrite (which would be data loss):

- Before writing a message, if its truncated key already names a directory whose
  `message.eml` bytes differ from the incoming bytes, the writer MUST **promote
  the entire archive to full 32-byte (64 hex char) keys** by setting
  `"messageKeyBytes": 32` in `flat-email.json` and re-rendering, then retry.
- If a collision persists even at 32 bytes (cryptographically implausible), the
  writer MUST **fail loudly** with a deterministic error naming both messages
  rather than overwrite or merge them.
- `"messageKeyBytes"` (default `16`) is recorded in `flat-email.json` so a reader
  always knows the key length in use. Two byte-identical messages are the *same*
  message, not a collision: re-writing them is a no-op (§6).

### 4.3 A message in many labels/folders (store-once)

A message is stored **exactly once**, under its date bucket and message key.
Membership is expressed **by reference**, never by copying:

- Gmail labels and IMAP folders are both modelled as **labels**.
- `labels/<sanitized-label>.json` contains a **sorted** array of message keys
  carrying that label (sort order per §6.1).
- `metadata.json` for the message lists its labels (also sorted).
- Every label that appears anywhere MUST also appear in the label manifest
  (§4.7), which is the authority on original name ↔ on-disk filename mapping.

This makes "one message, many folders" (the Gmail model) free of duplication:
bodies and attachments exist once; labels are just indexes pointing at them.

### 4.4 Attachment naming and collisions

Attachment filenames from the wire are untrusted and may collide, contain path
separators, exceed length limits, or use non-portable characters. Rules:

- The on-disk filename is **sanitized** (§7) from the declared filename. An
  attachment with no declared filename is named `part-<NN>` where `<NN>` is its
  zero-padded MIME part index (§12), keeping output deterministic.
- On collision within a single message's `attachments/` directory, append
  ` (n)` before the extension in deterministic order of the MIME parts
  (`invoice.pdf`, `invoice (1).pdf`, …). Part order is the message's own part
  order, so the result is stable across re-syncs.
- An `attachments.json` manifest records, for each attachment, the **original**
  declared filename, the **on-disk** filename, the content type, the byte size,
  the MIME part index, the content disposition (`attachment`/`inline`), the
  `Content-ID` (if any), and the SHA-256 of the payload — so the original name is
  never lost and integrity is verifiable. Entries are ordered by MIME part index.

### 4.5 Case-insensitive / case-preserving filesystems

The format MUST survive a round-trip through case-insensitive,
case-preserving filesystems (APFS default on macOS, NTFS on Windows) as well as
case-sensitive ones (typical Linux). Therefore:

- **No two paths in an archive may differ only by case.** Generated identifiers
  (`<message-key>`, `<thread-key>`) are lowercase hex and so are inherently safe.
- Account directory names are lowercased.
- Sanitized label and attachment names are compared **case-insensitively** when
  detecting collisions, and a case collision is resolved with the same ` (n)`
  suffixing rule as §4.4.

### 4.6 Format versioning

Covered in §2 — the archive root `flat-email.json` declares `specVersion`, and no
other rule in this spec is allowed to depend on an implicit, undocumented format
version.

### 4.7 Label manifest

Sanitization (§7) is lossy and case-folding (§4.5) can map two distinct provider
labels onto one filename. To keep the user's original meaning while paths stay
portable, every account has a **label manifest** at `labels/labels.json`.

- It is a JSON object whose keys are the **sanitized** filenames (without the
  `.json` extension) used under `labels/`, emitted in sorted key order.
- Each value records the provider's **original** label/folder name, the provider
  label id (when the connector exposes one, else `null`), the label `type`
  (`"system"` for provider-defined labels like Inbox/Sent/Spam, else `"user"`),
  and `visibility` (`"visible"` or `"hidden"`; `"visible"` when unknown).
- When two distinct original names sanitize to the same filename, they are
  disambiguated with the same ` (n)` suffix rule as §4.4 and each resulting
  filename gets its own manifest entry, so no label is silently merged.

The manifest is validated by [`schemas/labels.schema.json`](schemas/labels.schema.json).

---

## 5. Authoritative vs. derived files

Following `SUGGESTIONS.md` §4, every output is classified:

| File | Class | Rebuildable from |
| --- | --- | --- |
| `message.eml` | **Authoritative** | — (the source of truth) |
| `metadata.json` | Derived | `message.eml` + this spec |
| `body.txt`, `body.html` | Derived | `message.eml` |
| `email.html` | Derived | `message.eml` + metadata |
| `attachments/*`, `attachments.json` | Derived | `message.eml` |
| `threads/*` | Derived | the set of `message.eml` files |
| `labels/*.json`, `labels/labels.json` | Derived | the set of `metadata.json` files |
| `catalog.json`, `catalog.js` | Derived | the whole archive |
| `index/`, `index.html` | Derived | the whole archive |

Consequences:

- The **only** byte sequence that must be preserved forever is `message.eml`,
  which MUST be valid RFC 5322 and round-trippable to/from `.mbox`/Maildir.
- A corrupted or deleted derived file is never a data-loss event: re-render it.
- A `specVersion` upgrade re-renders derived files only (§2).
- `.flat-email-state/` (§14) is neither authoritative nor part of the archive;
  losing it triggers a full, still-idempotent re-scan, never data loss.

---

## 6. Determinism and idempotency

These are guarantees, not best-effort behaviours.

- **Determinism.** Every generated file is a pure function of the authoritative
  message bytes and this spec's `specVersion`. Concretely: JSON keys are emitted
  in a fixed (lexicographic) order; all arrays (labels, thread members,
  attachment lists) are **sorted**; no generation timestamp or hostname or random
  value is embedded in any per-message file (the one allowed generation timestamp
  lives in `flat-email.json`, §2).
- **Idempotency invariant.** `sync` is safe to run any number of times. A message
  already present at its `<message-key>` path is **never rewritten** unless
  `specVersion` changes. This is what makes the **Email 2 Repo** mode (`IDEAS.md`
  §1.2) produce meaningful commits instead of churn.
- **Deletions and upstream changes — archive semantics.** The archive is
  **append-only with respect to messages**: a message that disappears or is
  deleted upstream is retained locally (durability is the archive's job). Label
  membership, by contrast, is a **mirror** of the current upstream state and may
  be added or removed on re-sync, because labels are cheap derived indexes, not
  data. `metadata.json` records `firstSeen` and `lastSeen` sync markers so that
  upstream deletions are observable without destroying the message.

### 6.1 Canonical sort orders (tie-breakers)

"Sorted" is ambiguous without a total order, and ties produce non-deterministic
output, so every ordered list in the format uses an explicit, total ordering:

- **Message keys / thread keys** sort by ascending lowercase-hex string
  (bytewise). Hex strings of equal length compare unambiguously.
- **Thread members** (`threads/<thread-key>.json`) sort by **bucket date ascending,
  then message `date` ascending, then message key ascending**. The message key is
  the final tie-breaker, so order is total even when timestamps are equal.
- **Label membership** (`labels/<sanitized-label>.json`) sorts by message key
  ascending.
- **Label manifest** (`labels/labels.json`) and all JSON objects emit keys in
  ascending Unicode code-point order.
- **Attachments** (`attachments.json`) sort by MIME part index ascending (the
  message's own part order), which is already total.
- **Header, recipient, and label arrays inside `metadata.json`** preserve the
  semantic order defined in §10 (e.g. `to`/`cc` keep envelope order; `labels` are
  sorted); §10 states which is which per field so there is never a choice to make.

All string comparisons above are bytewise over the UTF-8 encoding of the value
after any normalisation that field already requires, so results do not depend on
locale.

---

## 7. Filename sanitization rules

A single sanitization function is applied to every externally-derived path
segment (labels, attachment names):

1. Unicode-normalise to NFC.
2. Replace the path separators `/` and `\`, control characters, and the
   characters illegal on Windows (`<>:"|?*`) with `_`.
3. Trim trailing dots and spaces (illegal/again-ambiguous on Windows).
4. Collapse to a maximum of 255 **bytes** (not codepoints) so the name fits the
   common per-component limit; if truncation occurs, append a short hash of the
   original to keep it distinct and record the original in the relevant manifest.
5. Treat the empty result as `_`.

Generated identifiers (`<message-key>`, `<thread-key>`) bypass sanitization by
construction because they are already lowercase hex.

---

## 8. Threads

- `<thread-key>` is the provider thread/conversation id when available, otherwise
  a digest of the normalised `References`/`In-Reply-To` root, otherwise the
  message's own key (a single-message thread). It is sanitized/hashed to be
  filesystem-safe by the same rules as §4.2.
- `threads/<thread-key>.json` contains an array of the member message keys,
  ordered per the thread rule in §6.1 (bucket date, then message date, then
  message key), so thread order is deterministic even when timestamps tie.

---

## 9. Reserved

The original §9 (Conformance and testing) has moved to §15 and §10 (Open items)
to §16 so that the load-bearing data formats — metadata, catalog, MIME
extraction, HTML safety, and sync state — can be specified in document order
before conformance is defined. This heading is kept so existing cross-references
to "§9" resolve to a stable anchor.

---

## 10. `metadata.json` — the per-message record

`metadata.json` is the derived, machine-readable view of one message. It is a
**v0.1 blocker**, not a future item: connectors cannot be interchangeable without
it. It is validated by [`schemas/metadata.schema.json`](schemas/metadata.schema.json).

**Encoding & ordering.** UTF-8, no BOM, LF newlines, a single trailing newline.
Object keys are emitted in ascending Unicode code-point order (§6); arrays follow
§6.1. Numbers are integers where counts/sizes are meant. A field that is unknown
is emitted as `null` (it is never silently omitted), except arrays, which are
emitted as `[]` when empty — so the set of keys is identical for every message and
diffs stay clean.

**Dates.** Every timestamp is an RFC 3339 / ISO 8601 string normalised to UTC
with a `Z` suffix and second precision (e.g. `2024-01-15T09:30:00Z`). `date` is
the resolved bucket date (§4.1); `dateSource` is one of `internal`, `received`,
`header`, or `unknown`. When `dateSource` is `unknown` (the `unknown-date/`
bucket), `date` is the sentinel `1970-01-01T00:00:00Z` so the field type stays
constant and sorts stably before any real date.

**Required fields.**

| Field | Type | Meaning |
| --- | --- | --- |
| `specVersion` | integer | Mirrors the archive manifest; lets a lone `metadata.json` be interpreted. |
| `messageKey` | string | The §4.2 digest naming this message's directory. |
| `messageIdHeader` | string \| null | Verbatim RFC `Message-ID` header value, or `null` if absent. |
| `date` | string | Bucket date (§4.1), UTC RFC 3339. |
| `dateSource` | string | `internal` \| `received` \| `header` \| `unknown`. |
| `subject` | string | Decoded (RFC 2047) `Subject`, or `""` if absent. |
| `from` | object \| null | `{ "name": string\|null, "address": string }` for the first `From`. |
| `to`, `cc`, `bcc` | array | Address objects in **envelope order** (not sorted). |
| `replyTo` | array | Address objects in envelope order. |
| `threadKey` | string | The §8 thread key. |
| `labels` | array of string | Sanitized label filenames (without `.json`), sorted (§6.1). |
| `flags` | array of string | Normalised flags (see below), sorted ascending. |
| `attachmentCount` | integer | Number of entries in `attachments.json`. |
| `hasBodyText` | boolean | Whether `body.txt` was produced (§12). |
| `hasBodyHtml` | boolean | Whether `body.html` was produced (§12). |
| `firstSeen` | string | UTC timestamp this message was first written (§6). |
| `lastSeen` | string | UTC timestamp of the most recent sync that observed it upstream. |

**Address objects** are `{ "name": string|null, "address": string }`; `name` is
RFC 2047-decoded and `address` is lowercased in its domain part only (the local
part is case-preserved per RFC 5321). **Flags** are normalised to the lowercase
set `seen`, `answered`, `flagged`, `draft`, `deleted`, `recent` (IMAP) plus
`starred`/`important` (Gmail); unknown provider flags are dropped from this list
but preserved verbatim under the optional `providerFlags` array.

**Optional fields** (emitted only when the connector has the data, and then always
present for that connector so its output stays stable): `providerMessageId`,
`providerThreadId`, `providerFlags`, `inReplyTo`, `references` (array, verbatim
header order). Connectors document which optional fields they populate.

`createdBy`/`createdAt`-style generation timestamps MUST NOT appear here; the only
per-message time markers are the content-independent `firstSeen`/`lastSeen` sync
markers, which live here precisely because they are *not* derivable from the bytes
and would otherwise have no home (they never appear in `body.*`/`email.html`).

---

## 11. `catalog.json` / `catalog.js` — the archive index

The README promises that opening the archive folder gives a browsable, offline
mailbox. Browsers cannot enumerate a directory over `file://`, so the archive
ships a **generated catalog** the reader can load instead of discovering files.

- `catalog.json` is the canonical document, validated by
  [`schemas/catalog.schema.json`](schemas/catalog.schema.json).
- `catalog.js` is the *same JSON* assigned to a global, i.e. exactly
  `window.FLAT_EMAIL_CATALOG = <catalog.json verbatim>;` followed by a newline.
  It exists because `file://` pages may load `<script src>` but often cannot
  `fetch()` a sibling `.json`. Because it is byte-derived from `catalog.json`, it
  is not a second source of truth.

`catalog.json` lists, in deterministic order (§6.1):

- `specVersion` and a `counts` summary (`accounts`, `messages`, `threads`,
  `labels`).
- `accounts[]`: for each account, its address and **relative** paths to its
  messages, threads, and the label manifest, plus per-account counts.
- `messages[]`: for each message, `messageKey`, `account`, `date`, `subject`,
  `from`, `threadKey`, `labels`, `attachmentCount`, and the **relative** path to
  its directory. This is enough for the reader to render a list and link to each
  `email.html` without parsing every `metadata.json`.
- `threads[]` and `labels[]`: keys and relative paths for navigation.

All paths in the catalog are archive-root-relative and use `/` separators on
every OS (they are URL paths, not native paths), so the catalog copies losslessly
between machines.

---

## 12. Canonical MIME and body extraction

`body.txt`, `body.html`, and `attachments/*` are pure functions of `message.eml`;
to keep that function deterministic across implementations the extraction rules
are fixed here.

- **Body selection.** Walk the MIME tree depth-first. The chosen text body is the
  first `text/plain` part not marked `Content-Disposition: attachment`; the chosen
  HTML body is the first such `text/html` part. For `multipart/alternative`,
  prefer the richest *available* alternative for `body.html` and the plain
  alternative for `body.txt`; if only one exists, the other is simply absent
  (`hasBodyText`/`hasBodyHtml` in §10 record which were produced). `body.txt` is
  written **only** when a `text/plain` part exists — it is never synthesised by
  down-converting HTML, so output stays a pure function of the source.
- **Charset decoding.** Decode each part using its declared `charset`; if the
  label is missing or invalid, decode as UTF-8 with replacement, then re-encode
  the on-disk `body.*` as UTF-8 (LF newlines, no BOM). The raw bytes always remain
  intact in `message.eml`, so a mis-labelled charset is never lossy at the source.
- **Header decoding.** RFC 2047 encoded-words in `Subject`, display names, and
  filenames are decoded to Unicode for derived files; the raw headers stay in
  `message.eml`. Non-UTF-8 header bytes that are not valid encoded-words are
  decoded as Latin-1 (their byte values) so the result is total and reproducible.
- **Attachments & inline parts.** Every leaf part that is not the selected text or
  HTML body is an attachment, including `inline` parts and `multipart/related`
  embedded images. Each is decoded from its transfer encoding and written under
  `attachments/` (§4.4); its disposition and `Content-ID` are recorded so the HTML
  body's `cid:` references can be rewritten to relative paths (§13).
- **Malformed MIME.** A part with a broken or missing boundary, a truncated
  body, or an unpar-seable structure is treated as a single opaque
  `application/octet-stream` attachment rather than guessed at, so two
  implementations agree. The condition is noted in `attachments.json` via the
  content type; no data is discarded.
- **Calendar invites** (`text/calendar`) are treated as attachments (named
  `invite.ics` when otherwise unnamed) and are **not** selected as the body.

---

## 13. HTML safety (`body.html` and `email.html`)

An archive holds untrusted HTML authored by arbitrary senders, so rendering it
safely is mission-critical, not cosmetic. `body.html` is the **sanitized** body
(not the raw sender HTML — the raw bytes live in `message.eml`); `email.html`
wraps that sanitized body with headers, labels, and attachment links. Both MUST,
deterministically:

- **Remove all scripting:** strip `<script>`, inline event handlers
  (`on*` attributes), `javascript:`/`vbscript:`/`data:` URLs in active positions,
  and `<meta http-equiv="refresh">`.
- **Block automatic network access by default:** neutralise remote resources so
  opening a message never phones home (no tracking-pixel "email opened" pings).
  Remote `img`/`media`/CSS URLs are disabled (e.g. rewritten to a blocked
  placeholder) unless the user explicitly opts in; there is **no** automatic
  external fetch on load.
- **Strip embedding/active elements:** remove `<iframe>`, `<object>`, `<embed>`,
  `<frame>`/`<frameset>`, `<applet>`, `<form>`, and `<base>`.
- **Rewrite `cid:` references** for inline images to the message's relative
  `attachments/<name>` path (per §12) so embedded images render fully offline.
- **Make links safe:** external `href`s are preserved but rendered inert against
  the local page (e.g. `target="_blank"` with `rel="noopener noreferrer nofollow"`),
  and never auto-navigated.
- **Self-contained `email.html`:** all styles are inlined and no external
  stylesheet, font, or script is referenced, so it renders identically from disk
  on any machine with no network.

The sanitizer is part of the deterministic contract: the same input bytes MUST
produce byte-identical `body.html`/`email.html`, so the allow-list of tags and
attributes, and the placeholder used for blocked remote content, are fixed by the
implementation and exercised by the golden fixture (§15).

---

## 14. Sync state vs. archive data

Incremental sync needs per-account cursors (Gmail `historyId`, IMAP
`UIDVALIDITY`/`UIDNEXT`, Graph delta tokens). These are **operational state, not
archive data**, and are kept strictly separate so the archive stays a clean,
portable, shareable artifact:

- **Credentials never touch the archive.** OAuth/IMAP secrets live in OS keychains
  (Keychain, Credential Manager, Secret Service), never on disk in the archive —
  critical for the Git and cloud modes where the archive is shared (`IDEAS.md`
  §5, `SUGGESTIONS.md` §6).
- **Non-secret cursors** live under `.flat-email-state/<account>.json` (outside
  `accounts/`, §3). They are regenerable: deleting them forces a full re-scan that
  is still idempotent (§6), producing the identical archive. They contain **no**
  message bodies and **no** secrets.
- Backup, Git, and cloud modes MAY exclude `.flat-email-state/` (e.g. via
  `.gitignore`); doing so never risks data loss because it is not archive data.
- Losing or corrupting sync state therefore degrades to "slower next sync", never
  to a damaged archive.

---

## 15. Conformance and testing

An implementation conforms to this spec if, for the committed **golden archive**
fixture in [`tests/golden/`](tests/golden/), it reproduces every path and every
byte of every derived file. Test strategy (per `SUGGESTIONS.md` §8):

- The fixture pairs a small corpus of raw `.eml` inputs with the exact archive
  they must produce. It deliberately includes the gnarly cases: missing/garbage
  dates, a missing `Message-ID`, duplicate attachment names, a message in
  multiple labels, HTML containing `<script>` and a remote tracking image, an
  inline `cid:` image, and non-ASCII header and filename bytes.
- Each connector's job becomes "reproduce the golden archive shape"; tests are
  byte diffs against the fixture, validated additionally against the JSON schemas
  in [`schemas/`](schemas/).
- A determinism test runs `sync` twice and asserts zero byte changes.

See [`tests/golden/README.md`](tests/golden/README.md) for how the fixture is laid
out and intended to grow.

---

## 16. Open items for a future spec version

Tracked here so they are not silently forgotten; none block v0.1:

- Whether to offer a full 32-byte message key mode by default at large scale.
- A packing/manifest scheme for object storage to avoid many-small-files cost
  (`IDEAS.md` §1.3).
- The encrypted-vault on-disk shape and encrypted-index strategy (`IDEAS.md`
  §1.4).
- A formal opt-in mechanism and UI contract for re-enabling blocked remote
  content (§13) without breaking determinism of the at-rest files.
- Publishing the `schemas/` documents under stable URLs for third-party
  validation (`IDEAS.md` §3.5 ecosystem goal).
