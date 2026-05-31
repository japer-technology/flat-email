# Golden archive fixture

This is the conformance fixture referenced by [`../../SPEC.md`](../../SPEC.md) §15.
It pairs a tiny corpus of raw input messages with the **exact** archive a
conformant implementation MUST produce from them.

```
tests/golden/
  input/      raw RFC 5322 .eml inputs (the only thing a connector is given)
  archive/    the byte-exact archive that must be produced from input/
```

An implementation passes if, given `input/`, it reproduces every path and every
byte under `archive/`. Because the format is deterministic (SPEC §6), running the
producer twice MUST also yield zero changes.

## What each input exercises

| Input | Cases covered |
| --- | --- |
| `input/msg-a.eml` | `multipart/related` + `multipart/alternative`; both `text/plain` and `text/html` bodies; an **inline `cid:` image** rewritten to a relative `attachments/` path; **HTML sanitization** — a `<script>`, an `onclick` handler, and a **remote tracking image** are neutralised; a regular PDF attachment; a **non-ASCII RFC 2047 subject** (`Reçu n°1`); membership in **two labels** (`INBOX` + `Receipts`). |
| `input/msg-b.eml` | **Missing `Message-ID`** and **missing `Date`** → `unknown-date/` bucket and `dateSource: "unknown"`; **duplicate attachment filenames** (`report.pdf`, `report (1).pdf`); a **non-ASCII attachment filename** (`rapporté.txt`, RFC 2231 encoded); HTML-less message (`hasBodyHtml: false`). |

## How to check it (no project code required)

The JSON files validate against the schemas in [`../../schemas/`](../../schemas/),
and every message directory is content-addressed, so you can verify the
load-bearing guarantees with stock tooling:

```bash
# message-key == first 16 bytes of SHA-256(message.eml), per SPEC §4.2
find tests/golden/archive -name message.eml -print0 | while IFS= read -r -d '' f; do
  key=$(basename "$(dirname "$f")")
  got=$(sha256sum "$f" | cut -c1-32)
  [ "$key" = "$got" ] && echo "OK   $key" || echo "FAIL $key != $got"
done
```

## Growing the fixture

Add a new `input/*.eml` plus the derived files it must produce whenever the spec
gains a rule worth pinning down (e.g. calendar invites, S/MIME, garbage MIME
boundaries). Keep additions small and self-describing: the fixture is most useful
when each input isolates a specific edge case from SPEC §§4, 10–13.
