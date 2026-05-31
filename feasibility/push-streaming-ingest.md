# Feasibility Report: Push / Streaming Ingest

## Verdict

Lower feasibility for early versions; valuable later.

## Rationale

Near-real-time ingest through SMTP sinks, Gmail watch, or Microsoft Graph subscriptions can keep archives current, but it introduces always-on infrastructure, webhook security, provider-specific renewal, and more complex state handling.

## Dependencies

- Mature incremental sync engine.
- Secure webhook or local listener deployment model.
- Provider subscription renewal and backfill logic.
- Durable queueing so missed events do not lose messages.

## Key risks

- Push notifications are often hints, not full message payloads, so polling/backfill remains necessary.
- Public webhooks increase security exposure.
- SMTP sink operation can blur the line between archive and mail server.
- Missed events can silently desynchronise the archive without reconciliation.

## Recommendation

Defer until polling sync is reliable. When added, use push only as a trigger for normal incremental sync, not as the sole source of truth.
