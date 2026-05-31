# Feasibility Report: Mobile Companions

## Verdict

Read-only companions are moderately feasible; full mobile sync is low feasibility initially.

## Rationale

Browsing an existing archive on a phone matches the mission, but mobile filesystems, background execution, provider auth, and large local datasets make full sync difficult. A PWA or thin native wrapper around the reader is a better first step.

## Dependencies

- Archive reader that works with constrained local or cloud-backed files.
- Mobile-friendly UI and search experience.
- Integration with Files app, Android storage providers, or cloud storage SDKs.
- Clear offline caching strategy.

## Key risks

- Mobile browsers may restrict local file access even more than desktop browsers.
- Large mail archives can exceed device storage and indexing limits.
- Background sync is heavily constrained by iOS and Android lifecycle rules.

## Recommendation

Start with read-only browsing of an archive stored in a synced folder or cloud provider. Do not attempt mobile-first syncing until desktop/server sync is mature.
