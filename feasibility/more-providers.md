# Feasibility Report: More Providers

## Verdict

Moderately feasible, but should follow a proven connector abstraction.

## Rationale

Yahoo, Proton via Bridge, Fastmail, POP3, and JMAP expand coverage, but each provider has different auth, folder, label, rate-limit, and message identity semantics. The flat archive can absorb those differences only if the connector interface is disciplined.

## Dependencies

- Provider-neutral message, label, thread, and deletion model.
- Read-only connector contract.
- Auth storage outside the archive.
- Test fixtures for provider-specific edge cases.

## Key risks

- Provider APIs and policies change over time.
- Proton support may depend on a local bridge rather than direct API access.
- POP3 lacks rich labels and stable sync semantics.
- JMAP support depends on server adoption and capability variation.

## Recommendation

Add providers one at a time after one offline import path and one network provider are working. Prefer standards-based IMAP/JMAP before highly custom APIs unless demand is clear.
