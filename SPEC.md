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
  "createdBy": "flat-email/<tool-version>",
  "createdAt": "2024-01-15T00:00:00Z"
}
```

- `specVersion` is an **integer** that increments only on a breaking layout
  change. Readers MUST refuse (or explicitly downgrade-handle) an archive whose
  `specVersion` they do not understand.
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
  accounts/
    me@example.com/
      messages/
        2024/01/15/<message-key>/
          message.eml              # authoritative: raw RFC 5322 bytes
          metadata.json            # derived: headers, labels, thread id, flags
          body.txt                 # derived: plain-text body
          body.html                # derived: raw HTML body (if present)
          email.html               # derived: standalone reader page
          attachments/
            <sanitized-name>       # derived: decoded attachment payloads
            attachments.json       # derived: attachment manifest (see §4.4)
      threads/
        <thread-key>.json          # derived: ordered message keys in thread
        <thread-key>.html          # derived: standalone thread reader
      labels/
        <sanitized-label>.json     # derived: message keys carrying this label
  index/                           # derived: regenerable search index
  index.html                       # derived: serverless web-app entry point
```

All paths under `accounts/<account>/` use the account's normalised primary
address as the directory name (lowercased; see §7 on case).

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

### 4.3 A message in many labels/folders (store-once)

A message is stored **exactly once**, under its date bucket and message key.
Membership is expressed **by reference**, never by copying:

- Gmail labels and IMAP folders are both modelled as **labels**.
- `labels/<sanitized-label>.json` contains a **sorted** array of message keys
  carrying that label.
- `metadata.json` for the message lists its labels (also sorted).

This makes "one message, many folders" (the Gmail model) free of duplication:
bodies and attachments exist once; labels are just indexes pointing at them.

### 4.4 Attachment naming and collisions

Attachment filenames from the wire are untrusted and may collide, contain path
separators, exceed length limits, or use non-portable characters. Rules:

- The on-disk filename is **sanitized** (§7) from the declared filename.
- On collision within a single message's `attachments/` directory, append
  ` (n)` before the extension in deterministic order of the MIME parts
  (`invoice.pdf`, `invoice (1).pdf`, …). Part order is the message's own part
  order, so the result is stable across re-syncs.
- An `attachments.json` manifest records, for each attachment, the **original**
  declared filename, the **on-disk** filename, the content type, the byte size,
  and the SHA-256 of the payload — so the original name is never lost and
  integrity is verifiable.

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
| `labels/*` | Derived | the set of `metadata.json` files |
| `index/`, `index.html` | Derived | the whole archive |

Consequences:

- The **only** byte sequence that must be preserved forever is `message.eml`,
  which MUST be valid RFC 5322 and round-trippable to/from `.mbox`/Maildir.
- A corrupted or deleted derived file is never a data-loss event: re-render it.
- A `specVersion` upgrade re-renders derived files only (§2).

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
- `threads/<thread-key>.json` contains a **sorted-by-bucket-date** array of the
  member message keys, so thread order is deterministic.

---

## 9. Conformance and testing

An implementation conforms to this spec if, for the committed **golden archive**
fixture, it reproduces every path and every byte of every derived file. Suggested
test strategy (per `SUGGESTIONS.md` §8):

- Hand-build a tiny golden archive from a small corpus of raw `.eml` files
  (including multipart, nested attachments, weird encodings, missing/garbage
  dates, duplicate attachment names, and non-UTF-8 headers) and commit it.
- Each connector's job becomes "reproduce the golden archive shape"; tests are
  diffs against the fixture.
- A determinism test runs `sync` twice and asserts zero byte changes.

---

## 10. Open items for a future spec version

Tracked here so they are not silently forgotten; none block v0.1:

- Whether to offer a full 32-byte message key mode by default at large scale.
- A packing/manifest scheme for object storage to avoid many-small-files cost
  (`IDEAS.md` §1.3).
- The encrypted-vault on-disk shape and encrypted-index strategy (`IDEAS.md`
  §1.4).
- The exact `metadata.json` JSON schema, to be published as a versioned schema
  file alongside this document.
