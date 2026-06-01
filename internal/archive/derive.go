package archive

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"net/mail"
	"net/textproto"
	"sort"
	"strings"
	"time"

	"github.com/japer-technology/flat-email/internal/model"
)

const (
	dateLayout    = "2006-01-02T15:04:05Z"
	unknownDate   = "1970-01-01T00:00:00Z"
	unknownBucket = "unknown-date"
	defaultKeyLen = 16
)

// address is the metadata.json address shape: {name, address}.
type address struct {
	name    *string
	address string
}

// jsonObject renders the address with keys in canonical order via the map
// encoder (Go sorts: "address" before "name").
func (a address) jsonObject() map[string]any {
	var name any
	if a.name != nil {
		name = *a.name
	}
	return map[string]any{"address": a.address, "name": name}
}

// derivedMessage holds every value the producer needs to write a message's files
// and to assemble the account-level indexes. It is a pure function of the raw
// bytes plus the connector-supplied, non-derivable fields (labels, flags, sync
// markers).
type derivedMessage struct {
	key             string
	bucket          string // "YYYY/MM/DD" or "unknown-date"
	dirPath         string // archive-relative message directory
	date            string
	dateSource      string
	subject         string
	from            *address
	to, cc, bcc     []address
	replyTo         []address
	threadKey       string
	messageIDHeader *string
	labels          []string // sanitized, sorted, lowercase (filled by producer)
	labelsOriginal  []string // original label names from the connector
	flags           []string // sorted
	providerFlags   []string
	bodyText        *string // file content (LF, trailing newline)
	bodyHTML        *string // sanitized body, file content adds the newline
	attachments     []attachmentRecord
	payloads        map[string][]byte // onDiskName -> bytes
	raw             []byte
}

// deriveMessage computes all derived values for one message. account and labelFor
// give the producer's view of label names; keyLen is the archive's message-key
// byte length (SPEC §4.2).
func deriveMessage(account string, m model.Message, keyLen int) (*derivedMessage, error) {
	parsed, err := mail.ReadMessage(bytesReader(m.Raw))
	if err != nil {
		// A message that net/mail cannot split into header+body is still
		// archivable: the raw bytes are authoritative. Fall back to empty header.
		parsed = &mail.Message{Header: mail.Header{}, Body: bytesReader(nil)}
	}
	hdr := textproto.MIMEHeader(parsed.Header)
	body := readAll(parsed.Body)

	dm := &derivedMessage{
		key:             hashHex(m.Raw, keyLen),
		raw:             m.Raw,
		payloads:        map[string][]byte{},
		subject:         decodeHeaderWord(hdr.Get("Subject")),
		from:            firstAddress(hdr.Get("From")),
		to:              parseAddressList(hdr.Get("To")),
		cc:              parseAddressList(hdr.Get("Cc")),
		bcc:             parseAddressList(hdr.Get("Bcc")),
		replyTo:         parseAddressList(hdr.Get("Reply-To")),
		messageIDHeader: optionalString(hdr.Get("Message-Id")),
	}

	dm.date, dm.dateSource = resolveDate(m.InternalDate, hdr)
	dm.bucket = bucketFor(dm.date, dm.dateSource)
	dm.dirPath = "accounts/" + strings.ToLower(account) + "/messages/" + dm.bucket + "/" + dm.key
	dm.threadKey = resolveThreadKey(m, hdr, dm.key, keyLen)

	// Labels: the producer maps original names to (possibly disambiguated)
	// on-disk filenames at account scope; store the originals here.
	dm.labelsOriginal = append(dm.labelsOriginal, m.Labels...)

	// Flags: copy and sort.
	dm.flags = append(dm.flags, m.Flags...)
	sort.Strings(dm.flags)
	dm.providerFlags = append(dm.providerFlags, m.ProviderFlags...)

	if err := dm.extractBodiesAndAttachments(hdr, body); err != nil {
		return nil, err
	}
	return dm, nil
}

func (dm *derivedMessage) extractBodiesAndAttachments(hdr textproto.MIMEHeader, body []byte) error {
	var leaves []mimeLeaf
	walkMIME(hdr, body, &leaves)

	// Select bodies: first non-attachment text/plain and text/html.
	textIdx, htmlIdx := -1, -1
	for i, l := range leaves {
		if l.disposition == "attachment" {
			continue
		}
		if textIdx == -1 && l.mediaType == "text/plain" {
			textIdx = i
		}
		if htmlIdx == -1 && l.mediaType == "text/html" {
			htmlIdx = i
		}
	}

	if textIdx >= 0 {
		s := ensureTrailingNewline(normalizeNewlines(decodeCharset(leaves[textIdx].payload, leaves[textIdx].params["charset"])))
		dm.bodyText = &s
	}

	// Attachments: every leaf that is not a selected body.
	used := map[string]bool{}
	cids := map[string]string{}
	for _, l := range leaves {
		if l.index == textIdx || l.index == htmlIdx {
			continue
		}
		original := attachmentName(l)
		onDisk := disambiguate(sanitizeSegment(original), used)
		sum := sha256.Sum256(l.payload)
		dm.attachments = append(dm.attachments, attachmentRecord{
			PartIndex:        l.index,
			OriginalFilename: original,
			OnDiskName:       onDisk,
			ContentType:      l.mediaType,
			Disposition:      dispositionOf(l),
			ContentID:        optionalString(l.contentID),
			Size:             len(l.payload),
			SHA256:           hex.EncodeToString(sum[:]),
		})
		dm.payloads[onDisk] = l.payload
		if id := stripAngles(l.contentID); id != "" {
			cids[id] = "attachments/" + onDisk
		}
	}

	if htmlIdx >= 0 {
		raw := decodeCharset(leaves[htmlIdx].payload, leaves[htmlIdx].params["charset"])
		safe, err := sanitizeHTML(raw, cids)
		if err != nil {
			return err
		}
		dm.bodyHTML = &safe
	}
	return nil
}

// attachmentName resolves the declared filename (RFC 2047/2231 decoded), or a
// deterministic placeholder when none is declared (SPEC §4.4, §12).
func attachmentName(l mimeLeaf) string {
	name := l.dispParams["filename"]
	if name == "" {
		name = l.params["name"]
	}
	name = decodeHeaderWord(name)
	if name != "" {
		return name
	}
	if l.mediaType == "text/calendar" {
		return "invite.ics"
	}
	return fmt.Sprintf("part-%02d", l.index)
}

func dispositionOf(l mimeLeaf) string {
	if strings.EqualFold(l.disposition, "inline") {
		return "inline"
	}
	return "attachment"
}

// resolveDate picks the bucket date and its source in priority order (SPEC §4.1).
func resolveDate(internal *time.Time, hdr textproto.MIMEHeader) (string, string) {
	if internal != nil {
		return internal.UTC().Format(dateLayout), "internal"
	}
	if t, ok := earliestReceived(hdr); ok {
		return t.UTC().Format(dateLayout), "received"
	}
	if d := hdr.Get("Date"); strings.TrimSpace(d) != "" {
		if t, err := mail.ParseDate(d); err == nil {
			return t.UTC().Format(dateLayout), "header"
		}
	}
	return unknownDate, "unknown"
}

func earliestReceived(hdr textproto.MIMEHeader) (time.Time, bool) {
	var best time.Time
	found := false
	for _, rv := range hdr["Received"] {
		i := strings.LastIndexByte(rv, ';')
		if i < 0 {
			continue
		}
		if t, err := mail.ParseDate(strings.TrimSpace(rv[i+1:])); err == nil {
			if !found || t.Before(best) {
				best, found = t, true
			}
		}
	}
	return best, found
}

func bucketFor(date, source string) string {
	if source == "unknown" {
		return unknownBucket
	}
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		return unknownBucket
	}
	return t.UTC().Format("2006/01/02")
}

// resolveThreadKey follows SPEC §8: provider thread id, else a digest of the
// References/In-Reply-To root, else the message's own key.
func resolveThreadKey(m model.Message, hdr textproto.MIMEHeader, ownKey string, keyLen int) string {
	if m.ProviderThreadID != nil && *m.ProviderThreadID != "" {
		return hashHex([]byte(*m.ProviderThreadID), keyLen)
	}
	if root := referencesRoot(hdr); root != "" {
		return hashHex([]byte(root), keyLen)
	}
	return ownKey
}

func referencesRoot(hdr textproto.MIMEHeader) string {
	if refs := strings.Fields(hdr.Get("References")); len(refs) > 0 {
		return stripAngles(refs[0])
	}
	return stripAngles(strings.TrimSpace(hdr.Get("In-Reply-To")))
}

// ---- small helpers ----

func hashHex(b []byte, n int) string {
	sum := sha256.Sum256(b)
	if n > len(sum) {
		n = len(sum)
	}
	return hex.EncodeToString(sum[:n])
}

func optionalString(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func stripAngles(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	return s
}

func decodeHeaderWord(s string) string {
	if s == "" {
		return ""
	}
	dec := new(mime.WordDecoder)
	dec.CharsetReader = charsetReader
	if out, err := dec.DecodeHeader(s); err == nil {
		return out
	}
	return s
}

func firstAddress(s string) *address {
	list := parseAddressList(s)
	if len(list) == 0 {
		return nil
	}
	return &list[0]
}

func parseAddressList(s string) []address {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	list, err := mail.ParseAddressList(s)
	if err != nil {
		return nil
	}
	out := make([]address, 0, len(list))
	for _, a := range list {
		out = append(out, address{name: optionalString(a.Name), address: lowerDomain(a.Address)})
	}
	return out
}

// lowerDomain lowercases only the domain part of an address (RFC 5321 keeps the
// local part case-sensitive); SPEC §10.
func lowerDomain(addr string) string {
	i := strings.LastIndexByte(addr, '@')
	if i < 0 {
		return addr
	}
	return addr[:i] + strings.ToLower(addr[i:])
}
