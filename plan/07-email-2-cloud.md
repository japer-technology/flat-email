# Plan 07 — Email-2-Cloud (Object Storage Backend)

## Goal

Enable `flat-email sync --provider gmail --out s3://my-bucket/email-archive`
(and equivalent GCS / Azure / Backblaze B2 targets) so that the archive is
written directly to cloud object storage — making it durable, multi-device, and
shareable without a dedicated server.

## Prerequisites

- v0.1 archive producer is complete.
- `storage.Backend` interface is stable (all existing code already goes through it).
- At least one connector (Plan 01 or Plan 02) is working.

## Scope

**In scope**

- An `S3Backend` implementation of `storage.Backend` using the AWS SDK v2
  (compatible with AWS S3, Backblaze B2, Cloudflare R2, and MinIO via the
  S3-compatible API).
- A `GCSBackend` for Google Cloud Storage.
- A `AzureBackend` for Azure Blob Storage.
- Detection of the target from the `--out` URL scheme: `s3://`, `gs://`, `az://`.
- Credential handling: the standard SDK credential chains (env vars, config
  files, instance metadata) — no new credential format introduced.
- Efficient incremental writes: check `Exists` before uploading (the idempotency
  invariant from SPEC.md §6 means writing the same bytes twice is always safe,
  but skipping the upload saves cost and bandwidth).
- Content-type metadata set on every uploaded object (`application/json`,
  `text/html`, `message/rfc822`, etc.) so the archive is browsable in the
  cloud console.
- `.flat-email-state/` written to the **local filesystem** only, never to cloud
  storage (sync cursors are machine-local operational state, not archive data).

**Out of scope**

- Client-side encryption at rest (IDEAS.md §1.4 — a separate plan).
- A packing / manifest scheme for reducing per-object cost (IDEAS.md §16 open
  item — deferred).
- Multi-part upload for very large attachments (relevant above ~100 MB; rare
  in personal mail).
- Consumer cloud sync folders (Dropbox, Google Drive, OneDrive) — these use
  filesystem sync tools, not an API; users can point `--out` at the locally
  synced folder instead.

## Architecture

### URL scheme and backend selection

The CLI `--out` flag accepts both a local path and a URL:

| Value | Backend |
|-------|---------|
| `./my-archive` or `/home/user/archive` | `storage.FS` (existing) |
| `s3://bucket/prefix` | `storage.S3Backend` |
| `gs://bucket/prefix` | `storage.GCSBackend` |
| `az://container/prefix` | `storage.AzureBackend` |

Backend selection happens in the CLI layer before `archive.Produce` is called;
the producer and connectors are unchanged.

### New packages

```
internal/storage/
  s3.go     # S3Backend — aws-sdk-go-v2
  gcs.go    # GCSBackend — cloud.google.com/go/storage
  azure.go  # AzureBackend — github.com/Azure/azure-sdk-for-go/sdk/storage/azblob
  cloud.go  # parseBackendURL() factory; content-type inference
```

### `S3Backend` implementation sketch

```go
type S3Backend struct {
    client *s3.Client
    bucket string
    prefix string   // key prefix (archive root within the bucket)
}

func (b *S3Backend) Put(path string, data []byte) error
    // s3.PutObject with ContentType inferred from the file extension
func (b *S3Backend) Exists(path string) (bool, error)
    // s3.HeadObject; treat 404 as false
func (b *S3Backend) Read(path string) ([]byte, error)
    // s3.GetObject
func (b *S3Backend) List(prefix string) ([]string, error)
    // s3.ListObjectsV2 with pagination
```

`GCSBackend` and `AzureBackend` follow the same shape using their respective SDKs.

### Cost optimisation: existence check

The archive producer calls `Exists` before writing a message (the idempotency
invariant). For cloud backends, `HeadObject` is cheap (~0.004 USD per 10 k
calls on S3). On a re-sync with 0 new messages, only `HeadObject` calls are
made — no `PutObject` charges.

For large initial syncs (10 k+ messages), the initial `Exists` check can be
skipped by passing `--overwrite` flag; the producer writes all objects
unconditionally (still idempotent by content, but uses more bandwidth and
incurs more write costs).

### Parallelism

Cloud object writes are independent; the producer can safely upload multiple
objects concurrently. Each backend implementation uses a bounded worker pool
(default concurrency: 8) to parallelise `Put` calls, controlled by a
`--cloud-concurrency N` flag.

### Sync state locality

`.flat-email-state/<account>.json` is **always written to the local filesystem**
(alongside the archive directory or in `~/.flat-email/state/` when using a
cloud `--out`). It is never uploaded to the cloud bucket — it is operational
state, not archive data (SPEC.md §14).

## Phases

### Phase 1 — S3-compatible backend

1. Add `aws-sdk-go-v2` to `go.mod` (S3 module only).
2. Implement `storage/s3.go` — `Put`, `Exists`, `Read`, `List` using the SDK.
3. Implement `storage/cloud.go#parseBackendURL` — parse `s3://bucket/prefix`;
   return the correct backend.
4. Implement content-type inference (`storage/cloud.go#contentTypeFor`) based
   on file extension (`.json` → `application/json`, `.html` → `text/html`,
   `.eml` → `message/rfc822`, etc.).
5. Wire `--out s3://...` in the CLI.
6. Integration test against a local MinIO instance (started in the test suite
   via `testcontainers-go` or a pre-started fixture).

### Phase 2 — GCS and Azure backends

1. Implement `storage/gcs.go` using `cloud.google.com/go/storage`.
2. Implement `storage/azure.go` using the Azure Blob SDK.
3. Add `gs://` and `az://` URL parsing to the factory.
4. Integration tests against the GCS and Azure emulators (Azurite).

### Phase 3 — Parallelism and performance

1. Add a `ConcurrentBackend` wrapper that wraps any `Backend` and parallelises
   `Put` calls using a bounded goroutine pool.
2. Benchmark `flat-email import` against a 1 000-message fixture: local FS vs.
   S3 (MinIO); document throughput numbers.
3. Add `--cloud-concurrency` flag (default 8, max 32).

### Phase 4 — Docs and credential guide

1. Add a "Cloud Storage" section to the README with setup instructions for
   AWS (IAM role / access key), GCS (service account), and Azure (SAS token /
   managed identity).
2. Document the recommended bucket policy: private, versioning optional,
   lifecycle policy to transition old attachments to cheaper storage tiers.
3. Document `.gitignore` and `.flat-email-state/` locality so users understand
   what is and is not uploaded.

## Testing strategy

- **Unit tests** for `parseBackendURL` (valid and invalid URLs).
- **Unit tests** for `contentTypeFor` (all file extensions in the spec).
- **Integration tests** for `S3Backend` against MinIO; test `Put`, `Exists`,
  `Read`, `List` directly.
- **End-to-end test**: `flat-email import` with `--out s3://...` against MinIO;
  assert the resulting object tree matches the golden archive file list exactly.
- **Idempotency test**: run import twice against MinIO; assert the second run
  makes zero `PutObject` calls (all `Exists` checks return true).

## Risks

| Risk | Mitigation |
|------|-----------|
| Many-small-files cost (one S3 PUT per derived file; ~12 objects per message) | Document cost estimate; defer packing scheme to SPEC.md §16 open item |
| S3-compatible API differences (Backblaze B2 vs. R2 vs. real S3) | Test against each; document known quirks; use only the common subset of the S3 API |
| Cloud credential mistakes (public bucket, wrong region) | Validate bucket accessibility and warn if public before writing; add a `--dry-run` mode |
| `List` pagination performance on 100 k+ object archives | Use continuation-token pagination; cache the list in `catalog.json` to avoid listing on every read |
| Network interruption mid-sync leaves partial archive | The idempotency invariant (SPEC.md §6) makes re-running safe; document this; no special recovery logic needed |
