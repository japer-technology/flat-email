# Flat Email

A portable email extraction and archiving system that pulls messages from Gmail,
Outlook, IMAP, and other providers, then converts them into a flat,
filesystem-native structure. It preserves emails, attachments, metadata, labels,
and threads as ordinary files, making personal mail searchable, ownable,
portable, and accessible through tools, APIs, MCP, or direct file inspection.

## Why Flat Email

Email is one of the most valuable personal data stores, yet it is usually locked
inside proprietary services and opaque formats. Flat Email gives you full
ownership of your mail by turning it into plain files you can read, search,
back up, and process with any tool you already use.

- **Ownable** — your mail lives on disk as ordinary files, not in a vendor silo.
- **Portable** — move your archive between machines, drives, or clouds.
- **Searchable** — grep, ripgrep, or full-text indexers work out of the box.
- **Durable** — no database required to read your data; it is just files.
- **Accessible** — inspect directly, or query via the CLI, API, or MCP server.

## Features

- Connectors for Gmail, Outlook/Microsoft 365, and generic IMAP providers.
- Incremental, resumable syncing that only fetches what changed.
- Preserves attachments, headers, metadata, labels/folders, and thread
  relationships.
- Deterministic, human-readable on-disk layout (see [Layout](#on-disk-layout)).
- Full-text search across your entire archive.
- A local HTTP API and an MCP server for integration with tools and assistants.
- Read-only by default — your source mailboxes are never modified.

## Installation

```bash
# Install from source
git clone https://github.com/japer-technology/flat-email.git
cd flat-email
make install
```

## Quick Start

```bash
# Authenticate with a provider
flat-email auth gmail

# Sync a mailbox into a local archive
flat-email sync --provider gmail --out ./my-archive

# Search your archive
flat-email search "invoice 2024" --archive ./my-archive
```

## On-disk Layout

Each message and its associated data are stored as plain files in a predictable
structure:

```
my-archive/
  accounts/
    me@example.com/
      messages/
        2024/01/15/<message-id>/
          message.eml          # raw RFC 822 source
          metadata.json        # headers, labels, thread id, flags
          body.txt             # plain-text body
          body.html            # html body (if present)
          attachments/
            invoice.pdf
      threads/
        <thread-id>.json       # ordered list of message ids
      labels/
        inbox.json             # message ids carrying this label
  index/                       # search index (regenerable)
```

Because everything is a file, you can browse the archive in a file manager,
back it up with any sync tool, or process it with scripts.

## Usage

### Syncing

```bash
flat-email sync --provider imap \
  --host imap.example.com \
  --user me@example.com \
  --out ./my-archive
```

Syncs are incremental and resumable: re-running the command fetches only new or
changed messages.

### Searching

```bash
flat-email search "from:alice subject:report" --archive ./my-archive
```

### API and MCP

```bash
# Start the local HTTP API
flat-email serve --archive ./my-archive --port 8080

# Start the MCP server for use with assistants and tools
flat-email mcp --archive ./my-archive
```

## Supported Providers

| Provider              | Auth      | Status |
| --------------------- | --------- | ------ |
| Gmail                 | OAuth 2.0 | ✅     |
| Outlook / Microsoft 365 | OAuth 2.0 | ✅     |
| Generic IMAP          | Password / App password | ✅ |

## Contributing

Contributions are welcome! Please open an issue to discuss substantial changes
before submitting a pull request.

## License

Released under the MIT License. See [LICENSE](LICENSE) for details.
