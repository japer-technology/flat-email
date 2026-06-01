package archive

import (
	"bytes"
	"encoding/json"
)

// encodeJSON serialises v as the format's canonical JSON: two-space indented,
// UTF-8 with no HTML escaping, and a single trailing newline (SPEC §10).
//
// Determinism note: callers pass map[string]any for objects whose keys must be
// emitted in ascending code-point order (Go sorts map keys), and structs/slices
// where a fixed field order is required (e.g. attachments.json, SPEC §4.4).
func encodeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// attachmentRecord is one entry in attachments.json. Field order here is
// significant: attachments.json is emitted in this declaration order (SPEC
// §4.4), not sorted, so the struct encodes the on-disk field sequence directly.
type attachmentRecord struct {
	PartIndex        int     `json:"partIndex"`
	OriginalFilename string  `json:"originalFilename"`
	OnDiskName       string  `json:"onDiskName"`
	ContentType      string  `json:"contentType"`
	Disposition      string  `json:"disposition"`
	ContentID        *string `json:"contentId"`
	Size             int     `json:"size"`
	SHA256           string  `json:"sha256"`
}
