# Feasibility Report: Email 2 Encrypted Vault

## Verdict

Feasible but higher risk than plain storage modes.

## Rationale

Encryption at rest strongly matches the sensitivity of email archives, especially for cloud and Git destinations. The hard part is preserving usability, searchability, and recovery while avoiding bespoke cryptography.

## Dependencies

- Storage layer where encryption can be inserted transparently.
- Choice of established encryption format and key-management approach, such as age or GPG.
- Decision on whether paths, filenames, metadata, and indexes are encrypted.
- Recovery and rotation story for archive keys.

## Key risks

- Search becomes difficult if contents and indexes are encrypted.
- Directory structure may leak sensitive metadata even if file contents are encrypted.
- Lost keys mean permanent data loss.
- Cross-platform UX for key management can become complex.

## Recommendation

Defer until the core archive and storage abstraction are stable. Start with whole-file content encryption using a proven tool, document metadata leakage clearly, and avoid custom cryptographic design.
