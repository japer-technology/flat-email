# Suggestions for Flat Email

A close reading of `MISSION.md`, `README.md`, and `IDEAS.md` shows a clear and
compelling vision: **turn email into plain, ownable files** so mail becomes
portable, searchable, durable, and independent of proprietary platforms. The
documents are well-written and the design space is mapped thoughtfully. What is
missing is not more vision — it is the small number of decisions and guardrails
that turn this vision into something buildable without painting yourself into a
corner.

This document offers suggestions in that spirit: what to nail down first, the
traps hiding in the current design, and how to sequence the work so the project
stays true to its mission.

---

## 1. What you are really trying to achieve

Reading between the lines of the three documents, the core promise is:

> **Given any mailbox, produce a deterministic, self-describing folder of plain
> files that anyone can read, search, back up, and reason over — forever,
> offline, with no vendor and no database.**

Everything else in `IDEAS.md` (Git backend, cloud storage, desktop apps, MCP,
connectors) is a *delivery variation* on that one primitive. That means the
single most important asset in the entire project is **the on-disk layout**, and
the single most important property is **determinism**. If those two are right,
every other mode becomes "just another backend" exactly as `IDEAS.md` §5 hopes.
If they are wrong, every mode inherits the same flaws.

So the central suggestion is: **treat the file format as the product, and the
sync engine as a function that produces it.** Build outward from there.

---

## 2. Nail the layout spec before writing connectors

`IDEAS.md` §6 already lists this first — strongly agree. Before any provider code
exists, write `SPEC.md` (or `docs/layout.md`) that pins down the format as a
versioned contract. The README sketch is a great start but leaves load-bearing
questions unanswered:

- **Which date buckets the message?** The path `messages/2024/01/15/` needs a
  defined source: the `Date:` header (sender-controlled, can be wrong/missing),
  the `Received:` time, or the provider's internal date. Pick one, document it,
  and have a fallback for missing/garbage dates.
- **How is `<message-id>` derived?** RFC `Message-ID` headers are not guaranteed
  unique, not guaranteed present, and can contain characters illegal in
  filenames (`/`, `<>`, very long values). Define a deterministic, filesystem-
  safe derivation — e.g. a hash of the raw message, or a sanitized + hashed
  Message-ID. This is what makes sync idempotent.
- **A message in many labels/folders.** Gmail labels mean one message belongs to
  several "folders". The README stores the message once and lists IDs under
  `labels/inbox.json` — make this explicitly the rule (store-once, reference-by-
  id) so you never duplicate bodies or attachments.
- **Attachment naming collisions.** Two attachments named `invoice.pdf`, or names
  with path separators / unicode / 255-byte limits. Define sanitization and a
  collision strategy (suffixing, or content-addressed names + a manifest).
- **Case-insensitive / case-preserving filesystems.** macOS (APFS default) and
  Windows treat `A.eml` and `a.eml` as the same file; Linux does not. A format
  that is meant to be portable across machines must not rely on case to
  distinguish files.
- **Format versioning.** Put a `flat-email.json` / `VERSION` at the archive root
  declaring the layout version, so future readers and migrations have an anchor.

Recommendation: write the spec, then generate a tiny **golden archive** by hand
and commit it as a fixture. Every connector's job becomes "reproduce the golden
archive shape," and tests become diffs against fixtures.

---

## 3. Determinism and idempotency are features, not details

The mission words "durable" and "portable" only hold if **re-syncing the same
mailbox produces byte-identical (or at least diff-stable) output.** Suggestions:

- Make every generated file a pure function of the message content (and the
  documented format version) — no embedded timestamps-of-generation, no
  nondeterministic ordering of JSON keys or label lists. Sort everything.
- This is what makes the **Email 2 Repo** idea (§1.2) actually useful: commits
  will show *real* mail changes, not noise. Non-determinism would make Git diffs
  worthless.
- Define how deletions and label changes on the source are reflected. Is the
  archive append-only (never lose a message even if deleted upstream) or a
  mirror? These have very different trust properties — append-only is the more
  defensible default for an *archive* whose job is durability.

Consider writing down an explicit **idempotency invariant** in the spec:
"`sync` is safe to run any number of times; a message already present is never
rewritten unless the format version changes."

---

## 4. Separate the regenerable from the authoritative

`IDEAS.md` §5 rightly insists the search index stays regenerable. Push that
discipline further and classify *every* output:

- **Authoritative / source of truth:** `message.eml` (the raw RFC 5322 bytes).
  Keep this; everything else can be rebuilt from it.
- **Derived / regenerable:** `body.txt`, `body.html`, `email.html`,
  `metadata.json`, thread/label JSON, the `index/`, and `index.html`.

If the raw `.eml` is the single source of truth, then a format upgrade is just
"re-render derived files," and a corrupted derived file is never a data-loss
event. Document this split explicitly — it is a quiet superpower for longevity,
and it should drive what the Git backend ignores (`.gitignore` the `index/` and
arguably all derived HTML, or commit them deliberately as a choice).

---

## 5. Hard-look at the serverless `index.html` reader

The self-contained reader is the most distinctive promise — and the most
technically constrained. Be clear-eyed about `file://`:

- Browsers heavily restrict `file://` pages. `fetch()` of sibling files, ES
  module imports, and directory listing are commonly **blocked by CORS / same-
  origin rules**, especially in Chrome. A root `index.html` that tries to read
  thousands of sibling JSON files over `file://` will likely not work uniformly
  across browsers.
- There is **no directory enumeration** from the browser, so `index.html` cannot
  "discover" messages on its own. It needs a generated **manifest** (one or a
  few JSON/JS files) that enumerates everything, ideally loaded via a `<script>`
  tag (which `file://` allows) rather than `fetch` (often blocked).
- **Scale:** inlining an entire mailbox into one HTML file does not scale to
  100k+ messages. Plan for a manifest + lazy loading, and accept that the
  *whole-archive* reader may need the local HTTP API for large archives while
  *per-message* `email.html` stays truly standalone.

Concrete suggestions:
- Decide the contract: per-message `email.html` is fully inlined and standalone
  (great, keep it); the root `index.html` is a *progressive* app that works fully
  offline but reads a generated manifest, with a documented fallback to
  `flat-email serve` for huge archives.
- Prototype the `file://` reader **early** against a real browser matrix — this
  is the riskiest unproven assumption in the README, and discovering its limits
  now will shape the layout (e.g. emit `manifest.js` not `manifest.json`).

---

## 6. Security and privacy: the mission depends on it

An archive of someone's entire mail life is among the most sensitive artifacts
imaginable. `IDEAS.md` §5 flags secrets; elevate it to a first-class principle:

- **Tokens never touch the archive.** Store OAuth/IMAP credentials in OS
  keychains (Keychain, Credential Manager, Secret Service), never in the output
  folder — critical for the Git and cloud modes where the archive is shared.
- **Read-only by default** for source mailboxes is stated — make it an enforced,
  tested guarantee, not just a convention. It is a key trust differentiator.
- The standalone `email.html` renders **untrusted HTML from arbitrary senders**.
  This is an XSS / tracking-pixel / remote-content vector. Decide and document a
  policy: sanitize HTML, block remote resource loading by default (no leaking
  "email opened" pings), strip/neutralize scripts. The reader's safety story
  should be explicit.
- For the cloud/encrypted-vault modes, prefer client-side encryption *before*
  upload, and address the "search over encrypted data" question early since it
  changes the index design.

---

## 7. Narrow the first milestone ruthlessly

The vision is broad; the risk is breadth. Suggest an explicit, tiny **v0.1** that
proves the core primitive end to end and nothing more:

1. **One input:** `.mbox` / Maildir import (no OAuth, no network, fully testable
   offline). This de-risks the whole format before you fight provider APIs.
2. **One output:** local filesystem in the documented layout.
3. **One reader:** standalone per-message `email.html`.
4. **Determinism tests** against a committed golden archive.

Only after that is solid, add (in roughly `IDEAS.md` §6 order): Gmail connector,
incremental sync state, full-text index, HTTP API, MCP, then the broader modes.
Importing local mail stores first is the fastest path to a trustworthy format
because it removes auth and rate limits from the equation entirely.

---

## 8. Engineering foundations to decide up front

- **Language/runtime.** The README implies a single distributable binary
  (`make install`, `flat-email ...`). Go or Rust fit "single static binary, runs
  on NAS, easy to cross-compile" best; Node/Python ease the HTML reader work but
  complicate distribution. Pick deliberately, because it constrains the desktop/
  daemon/container modes later.
- **The layout abstraction = the storage-backend interface.** Design the writer
  as an interface (`put(path, bytes)`, `exists`, `list`) from day one, even if
  the only implementation is local FS. This is exactly what makes Git/cloud/
  encrypted modes "just another backend" (§1, §5) instead of rewrites.
- **Incremental sync state.** Decide where per-account cursors/history-ids live
  (inside the archive? a sibling state file? a keychain?), how they survive a
  copied/moved archive, and how a lost state file degrades (full re-scan that is
  still idempotent thanks to §3).
- **Testing strategy.** Golden-archive fixtures + a corpus of gnarly real-world
  emails (multipart, nested attachments, weird encodings, missing headers,
  non-UTF-8, calendar invites, S/MIME). Email is a swamp of edge cases; a strong
  fixture corpus is the best defense.

---

## 9. Documentation and ecosystem

- Promote the on-disk layout to a **published, versioned spec** so third parties
  can write connectors/readers (the §3.5 ecosystem goal). The format being open
  and stable is itself part of "returning email to its owner."
- Align with existing standards where free interop is available: ensure
  `message.eml` is valid RFC 5322 and that an archive can round-trip to/from
  `.mbox`/Maildir. Compatibility with tools people already trust reinforces the
  mission better than any new format would.
- Add a short **non-goals** section somewhere (e.g. "not a mail client, does not
  send mail, does not modify source mailboxes"). Clear boundaries keep the broad
  `IDEAS.md` surface from causing scope creep.

---

## 10. Summary — the few things that matter most

1. **The layout is the product.** Write a versioned `SPEC.md` and a golden-archive
   fixture before connectors.
2. **Determinism + idempotency** are load-bearing for portability, durability,
   and the Git mode — design for them explicitly.
3. **Raw `.eml` is the only source of truth;** everything else is regenerable.
4. **Prove the `file://` reader early** — it is the riskiest assumption; plan for
   a manifest and a graceful fallback at scale.
5. **Security is the mission, not a feature:** credentials in OS keychains, read-
   only sources, sanitized/safe HTML rendering.
6. **Ship a ruthlessly small v0.1** (mbox-in → files-out → standalone reader)
   before chasing the (excellent) breadth in `IDEAS.md`.

The vision is strong and coherent. The work now is to convert it into a tight
format spec and a deterministic core, then let the many delivery modes fall out
of that foundation as the documents already anticipate.
