# Feasibility Report: Email 2 Filesystem

## Verdict

Highly feasible and foundational.

## Rationale

This is the baseline product shape described by the README and formalised by `SPEC.md`. The repository is currently in design stage, so the main work is implementing the already-documented archive writer rather than inventing a new delivery mode.

## Dependencies

- Stable archive layout and schema conformance.
- Deterministic writer for raw messages, derived metadata, bodies, labels, threads, catalog files, and reader HTML.
- Golden archive tests that verify byte-stable output.

## Key risks

- Email edge cases such as malformed MIME, missing headers, duplicate attachment names, and odd encodings.
- Accidental non-determinism in generated JSON or HTML.
- Blurring the boundary between authoritative `message.eml` and derived files.

## Recommendation

Implement this first. Treat it as the acceptance target for every other idea: if a mode cannot preserve this layout and its determinism guarantees, it should wait.
