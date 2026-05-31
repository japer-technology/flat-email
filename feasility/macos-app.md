# Feasibility Report: macOS App

## Verdict

Feasible but operationally more complex than CLI distribution.

## Rationale

macOS users benefit from Keychain, launchd, Spotlight indexing, and a polished menu-bar app. However, notarisation, sandboxing, filesystem permissions, and update distribution introduce overhead that should wait for a stable core.

## Dependencies

- Stable CLI or embeddable sync core.
- Keychain integration.
- Notarised and signed `.app` build pipeline.
- Filesystem access design that works with macOS privacy prompts.
- launchd scheduling for background sync.

## Key risks

- Sandboxing may conflict with user-selected archive folders.
- Notarisation failures can block releases.
- Background agent UX needs careful error reporting.

## Recommendation

Offer a CLI/Homebrew path first. Build a macOS app after the cross-platform reader and sync flow are proven.
