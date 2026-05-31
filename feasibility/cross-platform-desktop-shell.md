# Feasibility Report: Cross-platform Desktop Shell

## Verdict

Feasible and likely preferable to three bespoke desktop apps.

## Rationale

A Tauri, Electron, or Wails shell can reuse the serverless reader and call the same sync core across Windows, macOS, and Linux. This reduces duplicated UI work while still enabling native credential storage and background helpers.

## Dependencies

- Stable command or library interface for sync operations.
- Reusable web reader UI.
- Cross-platform credential abstraction.
- Packaging and update strategy for all target operating systems.

## Key risks

- Less native feel than bespoke apps.
- Framework choice affects binary size, security surface, and update complexity.
- OS-specific background scheduling still requires platform code.

## Recommendation

Use this as the default desktop strategy once the CLI and reader are proven. Keep OS-specific apps out of scope unless user demand justifies them.
