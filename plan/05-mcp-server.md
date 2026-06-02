# Plan 05 — MCP Server (`flat-email mcp`)

## Goal

Enable `flat-email mcp --archive ./my-archive` to start a local MCP
(Model Context Protocol) server that exposes the archive to AI assistants and
agents as a set of read-only, well-scoped tools — so that assistants can search,
read, and reason over personal mail entirely on-device without the mail leaving
the local machine.

## Prerequisites

- `internal/search/` package exists and search works (Plan 03).
- `internal/api/` HTTP API works (Plan 04) — the MCP server reuses the same
  handler logic rather than re-implementing archive access.

## Scope

**In scope**

- MCP server communicating over stdio (the standard MCP transport for local
  tools).
- Read-only tool definitions (no reply, no delete, no label mutation).
- The following MCP tools:
  - `search_mail` — full-text and structured query, returns ranked message summaries.
  - `get_message` — fetch one message's metadata, subject, from, date, body preview.
  - `get_thread` — fetch an ordered thread with all member message summaries.
  - `list_labels` — enumerate all labels and their message counts.
  - `get_messages_by_label` — paginated message list for a given label.
  - `get_attachment_info` — metadata for a message's attachments (name, size, type).
- Tool outputs are scoped and paginated to prevent leaking an entire mailbox
  in one call.
- The MCP server is a standalone process; it does not depend on the HTTP API
  server being up.

**Out of scope**

- Write tools (reply, archive, delete, move) — explicitly deferred to maintain
  the read-only guarantee.
- MCP resources or prompts (tools only for v1).
- Remote / network-accessible MCP server (stdio only; no TCP/WebSocket transport).
- Multi-archive or multi-account selection within one MCP session.

## Architecture

### MCP protocol

MCP communicates over stdio using JSON-RPC 2.0. The server reads requests from
stdin and writes responses to stdout. Error output goes to stderr.

A pure-Go MCP SDK (e.g. `github.com/mark3labs/mcp-go`) handles the JSON-RPC
framing, capability negotiation, and tool dispatch. The tool handler functions
are the only application code required.

### New packages

```
internal/mcp/
  server.go      # MCP server setup, tool registration, stdio transport
  tools/
    search.go    # search_mail tool handler
    message.go   # get_message tool handler
    thread.go    # get_thread tool handler
    labels.go    # list_labels + get_messages_by_label tool handlers
    attachment.go # get_attachment_info tool handler
```

### Tool definitions

Each tool is defined with a JSON Schema for its input parameters and returns
a structured result. Descriptions are written for the assistant, not for users.

**`search_mail`**

```json
{
  "name": "search_mail",
  "description": "Search the local email archive. Returns ranked message summaries. Use structured fields (from, subject, label, after, before) where possible for precision.",
  "inputSchema": {
    "query": "string (required) — e.g. 'from:alice invoice after:2024-01-01'",
    "limit": "integer (default 10, max 50)",
    "offset": "integer (default 0)"
  }
}
```

**`get_message`**

```json
{
  "name": "get_message",
  "description": "Fetch the full metadata and body text of a single message by its message key.",
  "inputSchema": {
    "messageKey": "string (required)",
    "includeBody": "boolean (default true) — set false to get headers only"
  }
}
```

**`get_thread`**

```json
{
  "name": "get_thread",
  "description": "Fetch all messages in a thread in chronological order.",
  "inputSchema": {
    "threadKey": "string (required)",
    "includeBody": "boolean (default false) — set true to include body text for each message"
  }
}
```

**`list_labels`**

```json
{
  "name": "list_labels",
  "description": "List all labels/folders in the archive with message counts.",
  "inputSchema": {}
}
```

**`get_messages_by_label`**

```json
{
  "name": "get_messages_by_label",
  "description": "Fetch a paginated list of message summaries for a given label.",
  "inputSchema": {
    "label": "string (required) — sanitized label name as returned by list_labels",
    "limit": "integer (default 20, max 100)",
    "offset": "integer (default 0)"
  }
}
```

**`get_attachment_info`**

```json
{
  "name": "get_attachment_info",
  "description": "List the attachments of a message: name, content type, size in bytes.",
  "inputSchema": {
    "messageKey": "string (required)"
  }
}
```

### Output format

Tool results are plain-text or compact JSON — not HTML. Each message summary
returned by search/list includes: `messageKey`, `date`, `from`, `subject`,
`labels`, `snippet` (first 200 characters of `body.txt`), `attachmentCount`.
Full body text (when `includeBody: true`) is truncated at 4 000 characters with
a `[truncated]` marker to prevent context-window flooding.

### Privacy guardrails

- The MCP server only accesses the archive path passed on the command line.
  There is no mechanism for an assistant to request a different archive or path.
- Tool outputs never include raw email HTML (only sanitized plain-text body).
- Attachment *content* is never returned by any tool (only metadata).
  Assistants that need file content must ask the user to open the file directly.
- All tool handlers are read-only at the `storage.Backend` level (no `Put`).

## Phases

### Phase 1 — MCP server skeleton and `search_mail`

1. Add an MCP SDK to `go.mod`.
2. Implement `mcp/server.go`:
   - `NewServer(archiveRoot string, searchDB *sql.DB) *mcp.Server`.
   - Register tool definitions and handlers.
   - `Serve(ctx context.Context)` — read from stdin, write to stdout.
3. Implement `mcp/tools/search.go` — call `internal/search/query.Execute`.
4. Implement `flat-email mcp --archive` CLI sub-command.
5. Manual test with Claude Desktop or a local MCP client.

### Phase 2 — Remaining tools

1. `mcp/tools/message.go` — read `metadata.json` + `body.txt`.
2. `mcp/tools/thread.go` — read `threads/<key>.json` then resolve each member.
3. `mcp/tools/labels.go` — read `labels/labels.json` + per-label JSON files.
4. `mcp/tools/attachment.go` — read `attachments/attachments.json`.

### Phase 3 — Guardrails and polish

1. Add result-size caps to all list tools (`limit` max enforced server-side).
2. Truncate body text at a fixed character limit; include a `truncated: true`
   field so the assistant knows more content exists.
3. Add a `--readonly-check` assertion on startup: verify the `storage.Backend`
   is not writable (defense in depth).
4. Log every tool call to stderr (tool name, account, query) for auditability.
5. Document the server in the README and provide a Claude Desktop config snippet.

## Testing strategy

- **Unit tests** for each tool handler using an in-process MCP test client
  against the golden archive.
- **Input validation tests**: missing required fields, out-of-range `limit`,
  unknown `messageKey` — assert structured error responses.
- **Output cap test**: a tool called against a large fixture returns no more
  than `limit` results.
- **Privacy test**: assert no tool handler ever calls `backend.Put`.

## Risks

| Risk | Mitigation |
|------|-----------|
| MCP SDK API instability (protocol is still evolving) | Pin SDK version; abstract the framing behind a thin wrapper so swapping it is a one-file change |
| Assistant floods context with large mail bodies | Hard truncation at 4 000 chars; `truncated` flag; document this in tool descriptions |
| Sensitive mail exposed to a connected assistant | Document clearly: the assistant receives whatever the tool returns; users should only connect MCP to trusted assistants |
| stdin/stdout conflicts if the user runs `flat-email mcp` interactively | Detect a TTY and print a warning: "MCP server expects a JSON-RPC client on stdin" |
