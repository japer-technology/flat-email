# Feasibility Report: Email 2 Repo

## Verdict

Feasible after the filesystem baseline is deterministic.

## Rationale

A Git-backed archive is mostly orchestration around the flat filesystem output. It becomes valuable only if repeat syncs produce meaningful diffs, so it depends directly on stable paths, sorted metadata, and deterministic derived files.

## Dependencies

- Completed local filesystem sync mode.
- Clear policy for derived files and regenerable indexes in `.gitignore`.
- Safe handling for large attachments, likely through Git LFS or content-addressed storage.
- Credential and token storage outside the archive and repository.

## Key risks

- Huge repository growth from attachments and generated HTML.
- Privacy leaks if archives are pushed to remotes without clear warnings.
- Noisy commits if derived files are not byte-stable.
- Ambiguous commit semantics for label changes, deletions, and re-renders.

## Recommendation

Prototype as a thin wrapper around filesystem sync with local-only commits first. Add optional remote push only after size, ignore rules, and safety prompts are defined.
