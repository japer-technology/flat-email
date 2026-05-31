# Feasibility Report: Container / Self-hosted Server Image

## Verdict

Feasible and valuable for NAS/home-lab users once the CLI exists.

## Rationale

A container can package sync, local API, MCP, and reader serving in a repeatable deployment. It fits server and NAS use cases, but it depends on clear volume, secrets, and update practices.

## Dependencies

- Headless CLI operation.
- Environment or mounted-file configuration format.
- Volume layout for archive data and non-archive sync state.
- Secret injection that avoids storing tokens in image layers or archives.

## Key risks

- Users may expose private mail APIs publicly.
- File ownership and permissions can be difficult across host platforms.
- Token persistence and refresh flows are harder in headless containers.

## Recommendation

Ship after local sync and serve commands work. Default to loopback/private binding, document reverse-proxy risks, and keep secrets outside mounted archive paths.
