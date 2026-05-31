# Flat Email JSON schemas

These [JSON Schema](https://json-schema.org/) (draft 2020-12) documents are the
machine-readable companions to [`../SPEC.md`](../SPEC.md). They let connectors,
readers, and third-party tools validate the JSON files in a Flat Email archive
without re-implementing the prose rules.

| Schema | Validates | Spec section |
| --- | --- | --- |
| [`manifest.schema.json`](manifest.schema.json) | `flat-email.json` (archive manifest) | §2 |
| [`metadata.schema.json`](metadata.schema.json) | `accounts/<a>/messages/.../metadata.json` | §10 |
| [`catalog.schema.json`](catalog.schema.json) | `catalog.json` (and `catalog.js` payload) | §11 |
| [`labels.schema.json`](labels.schema.json) | `accounts/<a>/labels/labels.json` | §4.7 |
| [`attachments.schema.json`](attachments.schema.json) | `attachments/attachments.json` | §4.4 |

These schemas are versioned alongside the spec: a breaking change to either is a
`specVersion` bump (§2). They constrain shape and types only — the determinism,
ordering, and byte-level guarantees in the spec are additional requirements a
valid-but-non-conformant document could still violate, which is why the golden
fixture in [`../tests/golden/`](../tests/golden/) checks bytes, not just shape.
