# Feasibility Report: Windows App

## Verdict

Feasible after a working CLI/core exists, but not an early milestone.

## Rationale

A Windows GUI, installer, background sync, and Credential Manager integration would broaden adoption, but they wrap the core rather than prove it. The current repository is still at specification stage, so native packaging would be premature.

## Dependencies

- Stable sync core and local archive reader.
- GUI shell or webview strategy.
- Windows Credential Manager integration.
- Installer, code-signing certificate, auto-update, and background scheduling approach.

## Key risks

- Code signing and auto-update operational overhead.
- Background sync reliability across sleep, network changes, and account errors.
- Antivirus false positives for unsigned or uncommon binaries.

## Recommendation

Defer until the CLI is useful. Prefer a shared cross-platform shell before committing to a bespoke Windows app unless Windows-specific distribution becomes the primary goal.
