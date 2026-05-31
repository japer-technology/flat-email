# Feasibility Report: Local Email MCP Server

## Verdict

Highly feasible after archive query primitives exist.

## Rationale

MCP can expose search, message retrieval, thread lookup, labels, and attachments over the local archive without changing the archive format. It is a strong fit for Flat Email's automation and ownership goals.

## Dependencies

- Efficient archive search and metadata lookup.
- Read-only tool definitions by default.
- Permission and redaction model for sensitive mail.
- Clear handling of attachment access and large result sets.

## Key risks

- Overexposure of sensitive mail to connected assistants.
- Poor ranking or pagination could make search tools noisy.
- Action tools such as reply/delete would undermine the read-only guarantee if added too early.

## Recommendation

Implement as read-only after local search works. Require explicit archive paths and keep tool outputs scoped, paginated, and privacy-conscious.
