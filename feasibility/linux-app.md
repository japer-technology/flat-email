# Feasibility Report: Linux App

## Verdict

Feasible, especially as CLI plus service packaging.

## Rationale

Linux aligns well with filesystem-native archives, headless servers, NAS deployments, systemd timers, and plain-file tooling. GUI packaging can come later; the immediate value is robust CLI distribution.

## Dependencies

- Single-binary or simple packageable runtime.
- Systemd unit/timer examples for scheduled sync.
- Secret Service or keyring integration for credentials.
- Package metadata for AppImage, Flatpak, Snap, `.deb`, or `.rpm` channels.

## Key risks

- Packaging fragmentation across distributions.
- Desktop sandbox permissions for Flatpak/Snap.
- Keyring availability on headless systems.

## Recommendation

Support Linux early through CLI binaries and documented systemd timers. Treat GUI app packaging as secondary to reliable server/NAS operation.
