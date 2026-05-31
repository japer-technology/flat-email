# Feasibility Report: Library / SDK

## Verdict

Feasible after the archive format stabilises.

## Rationale

An SDK can let other tools produce and consume Flat Email archives, but publishing it too early risks freezing unstable abstractions. The layout spec should lead; the SDK should encode it.

## Dependencies

- Stable `SPEC.md` and schema versions.
- Conformance fixtures and compatibility tests.
- Clear separation between archive writer, parser, connector, and reader concerns.
- Versioning policy for breaking format changes.

## Key risks

- Premature public APIs can slow necessary format changes.
- Multiple implementations may diverge without strong conformance tests.
- Language choice may limit ecosystem adoption.

## Recommendation

Create internal library boundaries first, then publish an SDK once v1 archive semantics and conformance tests are stable.
