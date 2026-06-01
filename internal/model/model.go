// Package model defines the structured inputs to the Flat Email archive
// producer. Connectors (mbox, Maildir, and future provider connectors)
// translate their source mail into these types; the producer turns them into a
// SPEC.md-conformant on-disk archive.
//
// The split exists because some archive fields are NOT derivable from the raw
// RFC 5322 bytes (labels, flags, provider ids/dates, and the sync-time markers
// firstSeen/lastSeen). Those are supplied here by the connector, while
// everything else is derived deterministically from Raw.
package model

import "time"

// Label describes a single label/folder as the connector understands it. The
// original name is preserved so the lossy on-disk filename can be mapped back.
type Label struct {
	// OriginalName is the provider's label/folder name (e.g. "INBOX").
	OriginalName string
	// Type is "system" for provider-defined labels (Inbox/Sent/Spam/...),
	// otherwise "user".
	Type string
	// Visibility is "visible" or "hidden"; "visible" when unknown.
	Visibility string
	// ProviderID is the connector's label id when available, else nil.
	ProviderID *string
}

// Message is one source message handed to the producer.
type Message struct {
	// Raw is the authoritative, verbatim RFC 5322 byte sequence. It is written
	// to message.eml unchanged and is the basis of the content-addressed key.
	Raw []byte
	// Labels lists the original label names this message carries.
	Labels []string
	// Flags lists normalised flags already mapped to the SPEC §10 vocabulary
	// (seen, answered, flagged, draft, deleted, recent, starred, important).
	Flags []string
	// ProviderFlags holds verbatim provider flags not in the normalised set.
	ProviderFlags []string
	// InternalDate is the provider's internal received date when known. It takes
	// priority over Received/Date headers for bucketing (SPEC §4.1).
	InternalDate *time.Time
	// ProviderMessageID / ProviderThreadID are connector ids when available.
	ProviderMessageID *string
	ProviderThreadID  *string
}

// Account groups an address's messages and its label definitions.
type Account struct {
	// Address is the account's primary email address. It is lowercased for the
	// on-disk directory name (SPEC §3).
	Address string
	// Messages are the account's source messages.
	Messages []Message
	// Labels maps original label name -> Label metadata. A label referenced by a
	// message but absent here is treated as a "user" visible label.
	Labels map[string]Label
}

// Input is the complete set of accounts to produce an archive from, plus the
// non-content metadata the producer cannot derive from message bytes.
type Input struct {
	Accounts []Account
	// SyncTime is the single timestamp recorded as firstSeen/lastSeen for every
	// message written in this run (SPEC §10). Defaults to the producer's clock.
	SyncTime time.Time
	// CreatedBy / CreatedAt populate the archive manifest's informational fields
	// (SPEC §2). They are the ONLY place a generation timestamp may appear.
	CreatedBy string
	CreatedAt time.Time
}
