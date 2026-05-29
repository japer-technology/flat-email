# Ideas: Execution Modes for Flat Email

Flat Email turns mail into plain, ownable files. That single primitive — a
deterministic, filesystem-native archive — can be delivered through many
different execution modes. This document imagines those modes: how mail gets
*in*, where the archive *lives*, and how a user *runs* the tool. Each idea notes
what it unlocks, the rough shape of the work, and open questions.

These are exploratory ideas, not commitments. The goal is to map the design
space so we can pick deliberately.

---

## 1. Delivery / output targets ("Email 2 X")

These modes vary *where the flat archive is written and synchronised to*. The
extraction core stays the same; only the destination backend changes.

### 1.1 Email 2 Filesystem (the baseline)

Write the archive to a local directory, exactly as documented in the README.

- **Unlocks:** the foundation everything else builds on — ownable, offline,
  grep-able files.
- **Shape:** already the default `flat-email sync --out ./my-archive`.
- **Notes:** treat the on-disk layout as the canonical contract that every other
  mode targets or mirrors.

### 1.2 Email 2 Repo (Git-backed archive)

Sync mail into a Git repository, committing each incremental sync as a commit.

- **Unlocks:** versioned history of your mailbox, diff-able changes, free
  off-site backup via any Git host, signed/auditable archive, branch-per-account.
- **Shape:** wrap sync output in `git add/commit`; map a sync run to a commit
  with a structured message (counts, date range, accounts touched). Optional
  `--push` to a remote.
- **Open questions:** large attachments → Git LFS or content-addressed blobs?
  How to keep the regenerable search `index/` out of version control
  (`.gitignore`)? Commit granularity (per-sync vs per-message)? Avoid leaking
  secrets/tokens into history.

### 1.3 Email 2 Cloud Filesystem

Write directly to object storage and cloud drives (S3, GCS, Azure Blob,
Backblaze B2) and consumer sync folders (Dropbox, Google Drive, OneDrive,
iCloud Drive).

- **Unlocks:** durable off-machine archive, multi-device access, near-infinite
  capacity, share a read-only archive via a bucket.
- **Shape:** pluggable storage backend abstraction (an interface with
  local-FS, S3-compatible, and "just a synced folder" implementations). The
  serverless `index.html` reader could even be served statically from a bucket.
- **Open questions:** cost and latency of many small files (consider packing /
  manifest files); client-side encryption before upload; eventual-consistency
  for incremental sync state; credential handling per provider.

### 1.4 Email 2 Encrypted Vault

A storage backend that encrypts every file at rest (age/GPG, or per-archive key)
while preserving the directory structure for selective decryption.

- **Unlocks:** safe archiving of sensitive mail to untrusted/cloud storage.
- **Shape:** transparent encrypt/decrypt layer beneath the storage backend;
  reader tooling that decrypts on demand.
- **Open questions:** how does full-text search work over encrypted content
  (encrypted index vs decrypt-on-search)? Key management and recovery.

### 1.5 Email 2 Database / Index sink

Optionally mirror metadata into an embedded index (SQLite/DuckDB) or push to an
external search engine for very large archives — while files remain the source
of truth.

- **Unlocks:** fast structured queries and analytics at scale.
- **Shape:** the index is always *regenerable from files*, never authoritative.
- **Open questions:** keep it strictly optional so the "no database required"
  promise holds.

---

## 2. Packaged applications (per-platform)

These modes vary *how the tool is installed and run by a person* rather than
where data goes. They wrap the same core in friendlier packaging.

### 2.1 Windows app

A native Windows desktop application (installer/MSIX) with a GUI for auth, sync
configuration, progress, and an embedded reader.

- **Unlocks:** non-technical users on the most common desktop OS; system tray
  background syncing; "Open archive folder" in Explorer.
- **Shape:** GUI shell (e.g. a webview hosting the existing serverless reader)
  around the sync core; signed installer; auto-update.
- **Open questions:** code signing, background scheduling via Task Scheduler,
  credential storage in Windows Credential Manager.

### 2.2 Linux app

Distributed as AppImage, Flatpak, Snap, and native `.deb`/`.rpm`, plus the
existing CLI for power users.

- **Unlocks:** desktop Linux users; integration with systemd timers for
  scheduled syncs; secrets via the freedesktop Secret Service / keyring.
- **Shape:** same GUI shell as Windows where useful; emphasise CLI + service
  unit for headless servers/NAS.

### 2.3 macOS app

A signed/notarised `.app` (and Homebrew cask), menu-bar agent for background
sync, with Keychain-backed credentials.

- **Unlocks:** Mac users; native "Open with" for `email.html`; Spotlight can
  index the plain files for free.
- **Open questions:** notarisation pipeline, sandboxing vs filesystem access,
  launchd agent for scheduling.

### 2.4 Mobile companions (iOS / Android)

Likely read-only first: browse an existing archive (local or cloud) on a phone.

- **Unlocks:** carry your mail archive in your pocket, fully offline.
- **Shape:** the serverless reader already runs in a browser; a thin native
  shell or PWA could wrap it. Syncing on mobile is a harder, later step.

### 2.5 Cross-platform desktop shell

Rather than three bespoke apps, one cross-platform GUI (Tauri/Electron/Wails)
that wraps the core and reuses the existing self-contained reader.

- **Unlocks:** one codebase → Windows/macOS/Linux; lowest maintenance.
- **Trade-off:** less "native" feel than per-OS apps; pick this *or* §2.1–2.3,
  probably not both.

---

## 3. Service & integration modes

These modes vary *how other software talks to Flat Email*.

### 3.1 Local Email MCP server

Run the archive behind an MCP server so assistants and agents can search, read,
and reason over your mail locally.

- **Unlocks:** "ask your mailbox" workflows; agents that summarise threads,
  find attachments, draft replies grounded in real history — all on-device.
- **Shape:** already sketched as `flat-email mcp --archive`. Define MCP tools:
  search, get message, get thread, list labels, fetch attachment.
- **Open questions:** read-only vs action tools; redaction/scoping of sensitive
  mail; per-tool permission prompts.

### 3.2 Local HTTP API + serverless reader

The documented `flat-email serve` HTTP API alongside the static `index.html`
web app.

- **Unlocks:** custom UIs, scripts, browser extensions, dashboards over the
  archive without re-parsing files.
- **Shape:** already in the README; could become the shared backend for the
  desktop apps in §2.

### 3.3 Daemon / scheduled sync service

A long-running background service (systemd unit, launchd agent, Windows
service, or container) that keeps the archive continuously up to date.

- **Unlocks:** "set and forget" archiving; powers NAS/home-server deployments.
- **Shape:** wrap incremental sync in a scheduler with backoff and locking.

### 3.4 Container / self-hosted server image

An official Docker/OCI image for syncing + serving the API/MCP/reader, ideal for
home labs and NAS devices.

- **Unlocks:** one-command self-hosting; pairs naturally with §1.3 cloud storage
  and §3.3 scheduled sync.

### 3.5 Library / SDK

Expose the extraction and layout core as an importable library so other tools
can produce or consume Flat Email archives.

- **Unlocks:** an ecosystem — third-party connectors, exporters, and readers
  that all speak the same flat-file format.
- **Shape:** stabilise the on-disk layout as a documented spec first.

---

## 4. Connector / input modes ("X 2 Email")

These modes vary *where mail comes from* — the source side of the pipeline.

- **More providers:** Yahoo, Proton (via bridge), Fastmail, generic
  POP3, JMAP-native servers.
- **Local mail stores:** import existing `.mbox`, Maildir, Apple Mail, Outlook
  `.pst`/`.ost`, Thunderbird profiles — archive mail you already have offline.
- **Push/streaming ingest:** act as an SMTP sink or subscribe to provider push
  (Gmail watch / Microsoft Graph subscriptions) to capture mail in near
  real-time rather than polling.
- **Open questions:** keep the read-only-by-default guarantee for source
  mailboxes; normalise wildly different provider models into one flat layout.

---

## 5. Cross-cutting concerns

Themes that apply across most modes above:

- **Storage backend abstraction** — a single interface (local FS, Git, cloud
  object store, encrypted vault) makes §1 modes mostly "just another backend".
- **The layout is the contract** — every mode targets the same documented
  on-disk format; that is what keeps archives portable across modes.
- **Security & secrets** — provider tokens and credentials must never land in
  the archive (especially the Git and cloud modes); prefer OS keychains.
- **Index is always regenerable** — never let any mode make the search index
  authoritative; files remain the source of truth.
- **Offline-first** — the serverless reader is the through-line; most GUI/app
  modes can reuse it instead of building new UI.

---

## 6. Suggested sequencing (rough)

1. Solidify §1.1 filesystem layout as a documented spec (foundation for all).
2. Storage backend abstraction → unlocks §1.2 Git and §1.3 cloud cheaply.
3. Local MCP (§3.1) and HTTP API (§3.2) — high leverage for assistant workflows.
4. Cross-platform desktop shell (§2.5) reusing the serverless reader.
5. Daemon/container (§3.3/§3.4) for self-hosters.
6. Expand connectors (§4) and explore encrypted vault (§1.4) as the archive
   grows in value and sensitivity.
