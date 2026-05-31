# Feasibility Report: Local HTTP API + Serverless Reader

## Verdict

Highly feasible and central to the product experience.

## Rationale

The API and reader are already part of the target design. The per-message HTML reader is especially aligned with the flat-file mission, while the local HTTP API can cover browser limitations and large-archive performance.

## Dependencies

- Catalog and metadata generation from the archive.
- Safe HTML rendering and remote-content blocking.
- Local-only server defaults with explicit host binding.
- API routes for messages, threads, labels, attachments, and search.

## Key risks

- `file://` limitations may constrain the serverless root reader.
- XSS and tracking risks from untrusted email HTML.
- Serving private mail over a network interface by accident.

## Recommendation

Prototype the reader early against real browsers. Keep per-message `email.html` self-contained, and use the HTTP API as the scalable path for full-archive browsing and search.
