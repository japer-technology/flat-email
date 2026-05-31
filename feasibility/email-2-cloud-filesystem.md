# Feasibility Report: Email 2 Cloud Filesystem

## Verdict

Moderately feasible, best implemented behind a storage backend abstraction.

## Rationale

Object stores and consumer sync folders are natural destinations for a portable archive, but they differ sharply from local filesystems in consistency, latency, listing semantics, and credential handling.

## Dependencies

- Storage interface that can write, read, list, and atomically publish archive paths where possible.
- Manifest or catalog design that avoids excessive small-object reads.
- Provider-specific credential storage and configuration.
- Clear guidance for sync-folder providers versus direct object-store APIs.

## Key risks

- High cost or poor performance from many small files in object storage.
- Eventual consistency causing partial archive reads during sync.
- Remote credential exposure or accidental public buckets.
- Conflicts when multiple machines sync to the same destination.

## Recommendation

Support synced local folders first because the filesystem implementation already covers them. Add S3-compatible object storage next, with conservative write ordering and explicit warnings about encryption and public access.
