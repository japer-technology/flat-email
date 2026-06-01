package archive

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"strings"

	"golang.org/x/text/encoding/htmlindex"
)

// mimeLeaf is a single leaf part of a message's MIME tree, in the depth-first
// order that defines its partIndex (SPEC §12).
type mimeLeaf struct {
	index       int
	mediaType   string
	params      map[string]string
	disposition string // "attachment", "inline", or ""
	dispParams  map[string]string
	contentID   string // verbatim Content-ID, including angle brackets, or ""
	payload     []byte // decoded from its transfer encoding
}

// walkMIME recursively flattens a message body into its leaf parts. A multipart
// container with a missing boundary, or any unparseable structure, is treated as
// a single opaque application/octet-stream leaf rather than guessed at (SPEC
// §12), so two implementations agree.
func walkMIME(hdr textproto.MIMEHeader, body []byte, leaves *[]mimeLeaf) {
	mediaType, params, ok := parseContentType(hdr.Get("Content-Type"))
	if ok && strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			appendOpaque(hdr, body, leaves)
			return
		}
		mr := multipart.NewReader(bytes.NewReader(body), boundary)
		any := false
		for {
			p, err := mr.NextRawPart()
			if err != nil {
				break
			}
			any = true
			pb, _ := io.ReadAll(p)
			walkMIME(p.Header, pb, leaves)
		}
		if !any {
			appendOpaque(hdr, body, leaves)
		}
		return
	}

	disposition, dispParams := "", map[string]string{}
	if cd := hdr.Get("Content-Disposition"); cd != "" {
		if d, dp, err := mime.ParseMediaType(cd); err == nil {
			disposition, dispParams = d, dp
		}
	}
	*leaves = append(*leaves, mimeLeaf{
		index:       len(*leaves),
		mediaType:   mediaType,
		params:      params,
		disposition: disposition,
		dispParams:  dispParams,
		contentID:   hdr.Get("Content-Id"),
		payload:     decodeTransferEncoding(hdr, body),
	})
}

func appendOpaque(hdr textproto.MIMEHeader, body []byte, leaves *[]mimeLeaf) {
	*leaves = append(*leaves, mimeLeaf{
		index:       len(*leaves),
		mediaType:   "application/octet-stream",
		params:      map[string]string{},
		disposition: "attachment",
		dispParams:  map[string]string{},
		contentID:   hdr.Get("Content-Id"),
		payload:     decodeTransferEncoding(hdr, body),
	})
}

// parseContentType returns the media type and params, defaulting to text/plain
// when the header is absent (per RFC 2045). ok is false only when the header is
// present but unparseable, signalling malformed structure to the caller.
func parseContentType(raw string) (string, map[string]string, bool) {
	if strings.TrimSpace(raw) == "" {
		return "text/plain", map[string]string{}, true
	}
	mt, p, err := mime.ParseMediaType(raw)
	if err != nil {
		return "application/octet-stream", map[string]string{}, false
	}
	return mt, p, true
}

func decodeTransferEncoding(hdr textproto.MIMEHeader, body []byte) []byte {
	switch strings.ToLower(strings.TrimSpace(hdr.Get("Content-Transfer-Encoding"))) {
	case "base64":
		clean := strings.Map(func(r rune) rune {
			if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, string(body))
		if data, err := base64.StdEncoding.DecodeString(clean); err == nil {
			return data
		}
		if data, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(clean, "=")); err == nil {
			return data
		}
		return body
	case "quoted-printable":
		if data, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(body))); err == nil {
			return data
		}
		return body
	default:
		return body
	}
}

// decodeCharset decodes part bytes to a UTF-8 string using the declared charset,
// falling back to a lossless byte-preserving interpretation when the label is
// missing or unknown (SPEC §12).
func decodeCharset(b []byte, charset string) string {
	cs := strings.ToLower(strings.TrimSpace(charset))
	switch cs {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		return string(b)
	}
	enc, err := htmlindex.Get(cs)
	if err != nil {
		return string(b)
	}
	out, err := enc.NewDecoder().Bytes(b)
	if err != nil {
		return string(b)
	}
	return string(out)
}

// normalizeNewlines converts CRLF (and lone CR) to LF for derived text files.
func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

// ensureTrailingNewline guarantees exactly one trailing LF on a non-empty body.
func ensureTrailingNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}
