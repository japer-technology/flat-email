# Feasibility Report: Daemon / Scheduled Sync Service

## Verdict

Feasible after incremental sync and locking are reliable.

## Rationale

Scheduled sync is a natural extension of a CLI sync command and unlocks set-and-forget archiving. It should not be implemented until idempotency, sync state, and failure recovery are well-defined.

## Dependencies

- Incremental sync cursors and retryable provider operations.
- Archive locking to prevent concurrent writes.
- Backoff, logging, health status, and error reporting.
- OS-specific schedulers or a portable long-running process.

## Key risks

- Corruption from overlapping sync runs.
- Hidden failures if background jobs cannot notify users.
- Provider rate limits and auth refresh failures.
- Confusing semantics for deletions and label changes over time.

## Recommendation

Begin with documented scheduler integrations around the CLI. Add a long-running daemon only after the sync engine can recover safely from interruption.
