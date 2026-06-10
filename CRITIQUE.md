# CRITIQUE.md

A harsh-but-fair review of the Flat Email repository as it actually exists on
disk today — not as the README sells it. Findings are grounded in specific files
and verified against `make test` / `go vet` (both pass) and a read of every Go
source file.

## TL;DR

The Go that exists is clean, well-commented, and disciplined about determinism.
But this is, today, a ~2,000-line local mbox/Maildir-to-files converter wrapped
in ~3,700 lines of documentation describing a product that does not exist. The
one headline feature that *is* implemented and security-critical — HTML
sanitization to stop email tracking — has a hole big enough to drive a tracking
pixel through. Several promised reader features are stubs, the shipping code path
has ~0% test coverage, and the repo ships no LICENSE despite claiming MIT.

---

## 1. The README is a product brochure for vaporware

`README.md` opens by describing connectors for "Gmail, Outlook, IMAP, and other
providers," "incremental, resumable syncing," "full-text search," "a local HTTP
API and an MCP server," and a whole-archive serverless `index.html` web app. The
**Features** section (lines 36–47) lists all of these as bullet points with no
qualifier. Only much further down (lines 191–228) does the reader learn that
*none* of it is implemented — the only working command is `flat-email import`.

Quantitatively: **35 Markdown files / 3,706 lines of docs** vs **1,969 lines of
non-test Go**. The `feasibility/` (15 files) and `plan/` (7 files) directories
describe connectors, an MCP server, cloud sinks, encrypted vaults, mobile
companions, and more — none of which exist. The `Installation` block tells users
to run `make install` and then `flat-email auth gmail` / `flat-email sync`
(lines 60–69) — commands that don't exist and will print `unknown command`.

This is the central problem with the repo: the documentation-to-product ratio is
inverted. Leading with the target design and burying "none of this ships yet" is
misleading, even with the status disclaimer. Quick Start examples should run.

## 2. The anti-tracking sanitizer leaks — and it's the whole point

`SPEC.md` §13 is emphatic: rendering untrusted sender HTML safely is
"mission-critical," and the sanitizer MUST "Block automatic network access by
default… Remote `img`/`media`/CSS URLs are disabled… there is **no** automatic
external fetch on load."

`internal/archive/htmlsafe.go` only inspects two attributes — `src` and `href` —
plus `on*` handlers. It does **not** touch:

- `style="..."` inline CSS — `style="background:url(http://tracker/x.gif)"`
  fetches on open.
- `<style>` element contents — these are preserved verbatim as a text node, so
  `<style>body{background:url(http://tracker)}</style>` or `@import url(...)`
  phones home immediately. `<style>` is not in `htmlRemoveElements` and its text
  is never parsed.
- `srcset` (responsive images), `<img>`/`<video>` `poster`, and the legacy
  `background` attribute — all remote-fetch vectors left untouched.

So the single most important promise of the project — "open a message and it
never phones home, no tracking-pixel pings" — is **false for any sender who uses
CSS instead of a bare `<img src>`**, which is most marketing mail. This is both a
privacy regression and a direct violation of the implementation's own SPEC §13.
The golden fixture only exercises the `<img src>` case, so the gap is invisible
to CI.

## 3. The "readers" are stubs that don't match SPEC or README

The standalone HTML readers are sold hard ("your entire mailbox… exactly as a
webapp," README §"Reading Your Mail"). In reality:

- **`email.html` is missing promised content.** README line 110–112 and SPEC §13
  both say `email.html` wraps the body "together with the message headers, labels,
  and links to attachments." `internal/archive/readers.go:renderEmailHTML` emits
  only From / To / Subject / Date. No labels, no attachment links, no Cc/Bcc. The
  golden `email.html` confirms it (the message has `inbox`/`receipts` labels and
  two attachments, none of which appear).
- **`email.html` is malformed.** It embeds the sanitized body — itself a full
  `<html><head></head><body>…</body></html>` document — *inside* its own
  `<section>` inside its own `<body>`. You can see the nested `<html>` document in
  the committed golden `email.html`. Browsers tolerate it; it's still invalid.
- **The thread reader is a list of strings.** `renderThreadHTML` produces
  `<h1>Thread</h1>` followed by an `<ol>` of bare subjects — no links to the
  member messages, no dates, no senders, no bodies. Calling this a "standalone
  reader for the whole thread" (README line 90, 127) is generous.
- **Blocked remote images have no placeholder.** SPEC §13 says remote content is
  "rewritten to a blocked placeholder." The code merely renames `src` to
  `data-blocked-src`, producing an invisible broken image with zero UI affordance
  telling the user content was blocked or offering to load it.

## 4. The shipping code path is essentially untested

`go test -cover` tells the story:

```
internal/archive     73.5%
internal/mailstore     0.0%   <-- the actual import feature
internal/storage       0.0%
cmd/flat-email         0.0%
```

The golden test (`tests/golden_test.go`) is byte-exact and genuinely nice, but it
**bypasses the connectors entirely** — it hand-builds `model.Input` from `.eml`
files. So the only thing a user can actually run today — parsing an mbox or
Maildir, From-line unescaping, Status/X-Status flag extraction, Maildir `:2,`
info flags — has no test at all. There is no test that runs `flat-email import`
against an mbox and checks the result. The "first end-to-end slice" is never
tested end-to-end.

## 5. Dead code that contradicts a stated invariant

`internal/mailstore/mbox.go` defines `normalizeToCRLF` (lines 157–170) with a
doc comment claiming "the content-addressed key is taken over these normalised
bytes so the same logical message keys identically regardless of the source
store's newline convention." **`normalizeToCRLF` is never called** (verified by
grep — only the definition exists). Consequently the keying invariant it promises
does not hold: the same message imported from an LF-normalized mbox and a CRLF
Maildir will hash to *different* `messageKey`s and land in different directories.
Either the function should be wired in or the comment is a lie. `go vet` doesn't
catch unused package-level functions, so this rots silently.

## 6. "Searchable with grep out of the box" — except for HTML-only mail

A core selling point (README lines 32, 27) is that `grep`/`ripgrep` work over the
archive. But `derive.go` only populates `body.txt` from a `text/plain` MIME part;
there is no HTML→text fallback. Modern senders frequently ship **HTML-only**
messages with no plain-text alternative. For those, `body.txt` is never written,
so `grep` over the archive silently misses the entire body. The feature that's
advertised as working "out of the box" quietly fails for a large fraction of real
mail.

## 7. Project hygiene gaps

- **No LICENSE file.** README line 247 says "Released under the MIT License. See
  [LICENSE]" — there is no `LICENSE` in the repo. As shipped, the code is legally
  *unlicensed* (all rights reserved), directly contradicting the README and
  undermining the "ownable" pitch.
- **No CI.** There is no `.github/` directory. For a project whose entire value
  proposition is *byte-exact determinism*, having lint/vet/test run only when a
  human remembers to type `make` is a glaring omission. A single environment
  difference in JSON/HTML serialization would break the format and nobody would
  notice until a user complained.

## 8. Latent path traversal via account address

`producer.go:produceAccount` builds every output path from
`strings.ToLower(acc.Address)` with **no sanitization** (`derive.go:95`,
`producer.go:61`). Labels and attachment names are run through `sanitizeSegment`,
but the account address is not. A crafted address (e.g. `../../etc`) would let the
producer's `Backend.Put` write outside the archive root — `storage.FS.Put` does a
plain `filepath.Join(root, path)` with no containment check. Today the address is
user-supplied via `--account`, so the blast radius is small. But the moment the
"planned" Gmail/IMAP connectors feed externally-influenced addresses in, this
becomes a real traversal sink. Sanitize the account segment now, or enforce
root-containment in `FS.Put`.

## 9. Design smell: all-or-nothing key length

`chooseKeyLen` (producer.go:224) scans the *entire* input and, if **any** two
messages share a 16-byte SHA-256 prefix, switches **every** message in the whole
archive to 32-byte keys — changing every directory name and every path. This is
per SPEC §4.2, but it means one crafted message (a 16-byte prefix collision is
expensive but the mechanism is global) reshuffles the layout of an otherwise
stable archive, defeating the incremental/resumable re-sync story the README
sells. A per-collision local extension would be far less disruptive.

---

## What's genuinely good (credit where due)

- The determinism discipline is real and well-executed: a single canonical JSON
  encoder (`jsonenc.go`, `SetEscapeHTML(false)`, 2-space indent, fixed trailing
  newline), `map[string]any` for sorted-key objects vs a struct for the
  order-significant `attachments.json`, and a byte-exact golden fixture to lock it
  down. This is the strongest part of the codebase.
- MIME handling is careful: depth-first leaf indexing, opaque fallback for broken
  multipart, base64/quoted-printable decode with graceful fallback, and charset
  decoding via `x/text/htmlindex` reused for both bodies and RFC 2047 headers.
- The `storage.Backend` seam is a sensible abstraction for the future backends.
- Code is uniformly `gofmt`-clean, `go vet`-clean, and unusually well-commented
  with SPEC cross-references.

## Priorities if this were my repo

1. Fix the CSS/`<style>`/`srcset` sanitizer leak (§2) and add golden cases for
   each vector — it's a security bug in the one shipped feature.
2. Add a `LICENSE` file and a CI workflow (§7).
3. Test the connectors and add a real end-to-end `import` test (§4).
4. Either wire up `normalizeToCRLF` or delete it and fix the comment (§5).
5. Make `email.html`/thread readers actually match the SPEC, or downgrade the
   SPEC/README claims to what the code does (§3).
6. Trim the README's Features/Quick Start to what runs today; move the rest under
   an explicit "Roadmap" heading (§1).
