# Flat Email — Implementation Plans

This folder contains one detailed implementation plan per remaining feature.
Plans are ordered by the sequencing recommended in `IDEAS.md §6` and
`SUGGESTIONS.md §7`, building each layer on the one before it.

## Current state (v0.1)

What is already shipping:

- `flat-email import` — reads local `.mbox` / Maildir sources and writes a
  `SPEC.md`-conformant archive to disk.
- `internal/archive/` — deterministic archive producer (all derived files,
  HTML sanitization, golden-file conformance tests).
- `internal/storage/` — `Backend` interface with a local-filesystem
  implementation.
- `internal/model/` — connector-neutral message/account/input model.
- `internal/mailstore/` — mbox and Maildir readers.

## What is not yet implemented

Every item below has its own plan file.

| # | Plan | Unlocks |
|---|------|---------|
| 1 | [Gmail connector](01-gmail-connector.md) | First live network sync |
| 2 | [Generic IMAP connector](02-imap-connector.md) | Self-hosted and non-Google mail |
| 3 | [Full-text search](03-search.md) | `flat-email search` command |
| 4 | [HTTP API + serverless reader](04-http-api.md) | `flat-email serve` + `index.html` |
| 5 | [MCP server](05-mcp-server.md) | `flat-email mcp` for AI assistants |
| 6 | [Email-2-Repo (Git backend)](06-email-2-repo.md) | Versioned, diff-able archive |
| 7 | [Email-2-Cloud (object storage)](07-email-2-cloud.md) | S3 / GCS / cloud-backed archive |

## How to read the plans

Each plan follows the same structure:

1. **Goal** — one-sentence purpose.
2. **Prerequisites** — what must exist first.
3. **Scope** — what is in and out.
4. **Architecture** — packages, interfaces, and data flow.
5. **Phases** — ordered work items.
6. **Testing strategy** — how to verify correctness.
7. **Risks** — known hazards and mitigations.

## Guiding constraints (from SPEC.md and SUGGESTIONS.md)

- The archive format is the product. Every connector and delivery mode is a
  function that produces or consumes the same SPEC.md-defined on-disk layout.
- `message.eml` is the only authoritative file. All other files are derived
  and regenerable from it plus the spec version.
- Credentials never appear in the archive. OAuth tokens and IMAP passwords live
  in OS keychains only.
- The `storage.Backend` interface (`internal/storage/`) is the single
  abstraction point. New delivery modes add a new `Backend` implementation;
  the archive producer and all connectors are unchanged.
- Determinism and idempotency are guarantees, not best-effort. Tests must
  assert byte-identical output on repeated runs.
