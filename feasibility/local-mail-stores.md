# Feasibility Report: Local Mail Stores

## Verdict

Highly feasible and an excellent first connector milestone.

## Rationale

Importing `.mbox`, Maildir, Apple Mail, Outlook archives, and Thunderbird profiles avoids OAuth, network failures, and rate limits. It lets the project validate the archive layout against real messages while preserving read-only behavior.

## Dependencies

- Parsers for one or more local mail formats.
- Mapping from local folders/profile metadata to Flat Email labels and accounts.
- Robust MIME handling and attachment extraction.
- Golden tests with representative local-store fixtures.

## Key risks

- Outlook `.pst`/`.ost` support may require complex or platform-specific parsing.
- Apple Mail and Thunderbird profiles can vary by version.
- Large historical stores may reveal performance issues early.

## Recommendation

Start with Maildir or `.mbox` for v0.1 because they are simpler, offline, and testable. Treat Outlook and app-specific profile imports as later compatibility work.
