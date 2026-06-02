# Plan 06 — Email-2-Repo (Git-Backed Archive)

## Goal

Enable `flat-email sync --provider gmail --out ./my-archive --git` to commit
each incremental sync as a Git commit in the archive repository — giving the
mailbox a versioned history where `git diff` shows real mail changes (new
messages, label updates) with zero noise from non-determinism.

## Prerequisites

- v0.1 archive producer is complete and deterministic (SPEC.md §6 guarantees
  hold: same input → byte-identical output).
- At least one connector (Plan 01 or Plan 02) is working.
- `storage.Backend` interface is stable.

## Scope

**In scope**

- A `GitBackend` implementation of `storage.Backend` that writes files to a
  local Git-tracked directory and commits after each sync run.
- Auto-initialising a new repository if `--out` is not already a Git repo.
- A structured, informative commit message per sync run (message count, date
  range, accounts touched).
- A generated `.gitignore` that excludes regenerable/non-archive paths:
  `index/`, `.flat-email-state/`, and optionally `catalog.js`/`index.html`
  (as the user chooses).
- Optional `--push <remote>` flag to push to a remote after committing.
- `--git-commit-granularity per-sync` (default) or `per-message` mode.

**Out of scope**

- Git LFS for large attachments (an open item in IDEAS.md §1.2; deferred).
- Signed commits (GPG/SSH) — documented as a user-configurable option but not
  auto-configured.
- Branch-per-account — deferred; the default is one branch (`main`), all
  accounts in one repo.
- Hosting or pushing to GitHub/GitLab (the `--push` flag covers it; no
  special provider integration).

## Architecture

### `GitBackend`

`GitBackend` wraps the existing `FS` backend and adds a Git commit step.
It does not re-implement file I/O; it delegates all reads and writes to an
inner `FS`, then calls `git add -A && git commit` when `Commit()` is called.

```go
type GitBackend struct {
    fs   *storage.FS        // inner filesystem backend
    repo string             // absolute path to the Git repo root
}

// Commit stages all changes and creates a commit with msg.
func (g *GitBackend) Commit(msg string) error
```

The `archive.Produce` function already calls `storage.Backend` methods; after
`Produce` returns, the CLI calls `backend.Commit(message)` if the backend
supports it. A new optional interface avoids changing the `Backend` contract:

```go
type Committer interface {
    Commit(message string) error
}
```

The CLI checks `if c, ok := backend.(Committer); ok { c.Commit(...) }` after
each sync.

### Commit message format

```
sync: 47 new, 3 label-changes — me@example.com (2024-01-15)

Provider: gmail
Account:  me@example.com
New:      47 messages (2024-01-10 – 2024-01-15)
Updated:  3 label changes
Deleted:  0 (retained in archive)
Tool:     flat-email/0.2.0
```

This is structured enough to parse programmatically and readable as prose.

### `.gitignore` template

Generated once when the repo is initialised:

```
# Flat Email — regenerable paths excluded from version control
index/
.flat-email-state/
```

Whether to include `catalog.js` and `index.html` is left to the user (they
are small and useful to have in history, but they change on every sync).

## Phases

### Phase 1 — `GitBackend` and auto-init

1. Implement `storage/git.go`:
   - `NewGit(dir string) (*GitBackend, error)` — checks for an existing `.git/`;
     runs `git init` if absent; writes `.gitignore` if absent.
   - `Put`, `Exists`, `Read`, `List` — delegate to inner `FS`.
   - `Commit(msg string) error` — `git add -A`, then `git commit -m msg`.
     If there are no staged changes, log "nothing to commit" and return nil.
2. Use `os/exec` to invoke the system `git` binary (no pure-Go Git library
   dependency required for this scope).
3. Add `--git` flag to `flat-email sync` and `flat-email import`; when set,
   swap `storage.NewFS` for `storage.NewGit`.

### Phase 2 — Commit message generation

1. Accumulate sync statistics during `archive.Produce`:
   - New messages written.
   - Messages whose `metadata.json` was updated (label changes).
   - Date range of new messages.
2. Pass statistics to `GitBackend.Commit` to format the structured commit
   message above.
3. `flat-email import` uses a simpler message:
   `import: N messages from <source> — <account> (<date>)`.

### Phase 3 — Remote push

1. Add `--push <remote>` flag (e.g. `--push origin`).
2. After `Commit`, run `git push <remote> HEAD` if the flag is set.
3. Handle push errors (network unavailable, rejected push) gracefully:
   the archive is already committed locally; a failed push is a warning,
   not a fatal error.
4. Document the `--push` flag in the README with a note about token/SSH key
   setup being the user's responsibility.

### Phase 4 — `per-message` granularity mode

1. Add `--git-commit-granularity per-message` flag.
2. In this mode, `GitBackend.Commit` is called once per message written,
   with a message like `add: <message-key> (<date>, <from>, <subject>)`.
3. This produces a one-commit-per-email history, useful for searching with
   `git log --grep` but much slower for large syncs.
4. Default remains `per-sync`.

## Testing strategy

- **`git log` test**: after a sync with `--git`, assert that `git log --oneline`
  produces exactly one commit per sync run with the expected message format.
- **Idempotency test**: run sync twice; the second commit should have no file
  changes (all files already exist with identical bytes). Assert the commit is
  empty (or skipped).
- **Diff test**: add one message to the fixture, run incremental sync, assert
  `git diff HEAD~1 HEAD` adds exactly the expected files and no others.
- **No-credential leak test**: assert no file in the committed tree contains
  any OAuth token or password string from the test credential set.

## Risks

| Risk | Mitigation |
|------|-----------|
| Large attachments bloat the Git object store | Document Git LFS as the mitigation; add `--git-lfs-threshold <bytes>` as a future flag |
| `git` binary not on PATH in some environments | Detect absence at startup and print a clear error; document the requirement |
| Slow `git add -A` on large archives (100 k files) | Use `git update-index --add --remove --stdin` with file list for per-sync efficiency |
| `catalog.js` and `index.html` change on every sync, polluting diffs | Document the `.gitignore` option; provide `--git-ignore-derived` flag |
| Credentials accidentally committed if user mis-places `state/` | The generated `.gitignore` excludes `.flat-email-state/`; add a pre-commit check that fails if any file under the archive path matches known secret patterns |
