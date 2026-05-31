# Feasibility Report: Email 2 Database / Index Sink

## Verdict

Feasible as an optional, regenerable acceleration layer.

## Rationale

A database or search index can improve query speed and analytics without compromising the flat-file mission if it is always rebuildable from the archive. This aligns with `SPEC.md`, which already treats indexes as derived.

## Dependencies

- Stable metadata and catalog schema.
- Rebuild command that can recreate the index from `message.eml` and derived metadata.
- Clear separation between archive correctness and index availability.

## Key risks

- Users may start depending on the database as the source of truth.
- Schema migrations add maintenance overhead.
- External search engines complicate installation and portability.

## Recommendation

Begin with an embedded SQLite or Tantivy-style local index after filesystem sync works. Keep all index files under a clearly regenerable path and make archive browsing work without them.
