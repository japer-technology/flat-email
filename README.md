# Flat Email

A portable email extraction and archiving system that pulls messages from Gmail,
Outlook, IMAP, and other providers, then converts them into a flat,
filesystem-native structure. It preserves emails, attachments, metadata, labels,
and threads as ordinary files, making personal mail searchable, ownable,
portable, and accessible through tools, APIs, MCP, or direct file inspection.

> **Project status: design stage.** Flat Email is currently a **specification**,
> not yet a released tool. The on-disk format is defined in [`SPEC.md`](SPEC.md),
> with machine-readable schemas in [`schemas/`](schemas/) and a byte-exact
> conformance fixture in [`tests/golden/`](tests/golden/). The CLI, connectors,
> and readers described below are the **target design**; commands and provider
> support are planned, not shipping yet. See [Project Status](#project-status).

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
- A standalone, self-contained HTML file for every email — open it straight
  from disk and read your mail as a serverless web app, no server required.
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
          email.html           # standalone, self-contained reader page
          attachments/
            invoice.pdf
      threads/
        <thread-id>.json       # ordered list of message ids
        <thread-id>.html       # standalone reader for the whole thread
      labels/
        inbox.json             # message ids carrying this label
  index/                       # search index (regenerable)
  index.html                   # serverless web app entry point
```

Because everything is a file, you can browse the archive in a file manager,
back it up with any sync tool, or process it with scripts.

The layout above is a sketch; the precise, versioned contract — date bucketing,
message naming, store-once labels, attachment collisions, cross-OS portability,
the `metadata.json` record, the archive `catalog.json`, HTML safety rules, and
the determinism guarantees — is defined in [`SPEC.md`](SPEC.md) and the schemas in
[`schemas/`](schemas/).

> **`body.html` vs `email.html`** — `body.html` is the **sanitized** HTML body
> (scripts removed and remote content blocked per [`SPEC.md`](SPEC.md) §13; the
> raw sender bytes are kept verbatim in `message.eml`). `email.html` wraps that
> sanitized body together with the message headers (from, to, subject, date),
> labels, and links to attachments into a complete, styled reader page you can
> open on its own.

## Reading Your Mail (Serverless HTML)

Every synced message is rendered to a standalone `email.html` file, and every
thread to a `<thread-id>.html` file. These pages are fully self-contained: they
embed their own styles and inline the message content, so they open and render
correctly straight from local disk by double-clicking — no server, no build
step, and no internet connection required.

```bash
# Open a single message in your browser
open ./my-archive/accounts/me@example.com/messages/2024/01/15/<message-id>/email.html

# Open a whole thread
open ./my-archive/accounts/me@example.com/threads/<thread-id>.html
```

At the root of the archive, `index.html` ties every email together into a
single serverless web app. Open it from disk (`file://`) and you get a familiar
mailbox experience — browse folders and labels, follow threads, search, and
click through to individual messages and attachments — all powered by the flat
files beside it, with nothing running in the background. Because browsers cannot
list a folder over `file://`, the reader loads a generated archive index
(`catalog.js` / `catalog.json`, see [`SPEC.md`](SPEC.md) §11) instead of trying
to discover files on its own.

```bash
# Launch the offline reader — your mail as a local web app
open ./my-archive/index.html
```

Imagine your preferred way to read your own email being a folder you fully own:
copy it to a USB stick, a backup drive, or another machine, double-click
`index.html`, and your entire mailbox is right there, exactly as a webapp,
working completely offline.

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

Connector support is **planned**; the table below is the target matrix, not a
statement of what ships today (see [Project Status](#project-status)).

| Provider              | Auth      | Status |
| --------------------- | --------- | ------ |
| Gmail                 | OAuth 2.0 | Planned |
| Outlook / Microsoft 365 | OAuth 2.0 | Planned |
| Generic IMAP          | Password / App password | Planned |
| Local `.mbox` / Maildir import | none | Planned (first milestone) |

## Project Status

Flat Email is at the **design/specification** stage. What exists today:

- [`MISSION.md`](MISSION.md) — why the project exists.
- [`SPEC.md`](SPEC.md) — the versioned, byte-level on-disk format contract.
- [`schemas/`](schemas/) — JSON Schemas for every JSON file the format defines.
- [`tests/golden/`](tests/golden/) — a tiny byte-exact conformance fixture
  (raw `.eml` inputs paired with the archive they must produce).
- [`IDEAS.md`](IDEAS.md) / [`SUGGESTIONS.md`](SUGGESTIONS.md) — execution modes
  and design guidance.

The `flat-email` CLI, the provider connectors, the HTTP API, the MCP server, and
the serverless readers are **not yet implemented**. The commands in this README
document the intended interface so the format can be designed against real usage.

## Contributing

Contributions are welcome! Please open an issue to discuss substantial changes
before submitting a pull request.

## License

Released under the MIT License. See [LICENSE](LICENSE) for details.
