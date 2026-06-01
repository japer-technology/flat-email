package archive

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// sanitizeSegment applies the SPEC §7 filename sanitization to a single derived
// path segment (a label or attachment name). It does not lowercase; callers that
// need case folding (labels, SPEC §4.5) apply it on top.
func sanitizeSegment(name string) string {
	s := norm.NFC.String(name)

	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '/' || r == '\\':
			b.WriteByte('_')
		case r < 0x20: // control characters
			b.WriteByte('_')
		case strings.ContainsRune(`<>:"|?*`, r):
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	out := b.String()

	// Trim trailing dots and spaces (illegal/ambiguous on Windows).
	out = strings.TrimRight(out, ". ")

	// Collapse to a maximum of 255 bytes; if truncated, append a short hash of
	// the original so distinct long names stay distinct.
	if len(out) > 255 {
		sum := sha256.Sum256([]byte(name))
		suffix := "-" + hex.EncodeToString(sum[:])[:8]
		limit := 255 - len(suffix)
		// Trim on a rune boundary so we never split a multibyte sequence.
		for limit > 0 && !utf8StartsCleanly(out, limit) {
			limit--
		}
		out = out[:limit] + suffix
	}

	if out == "" {
		return "_"
	}
	return out
}

// utf8StartsCleanly reports whether out[:n] ends on a UTF-8 boundary.
func utf8StartsCleanly(out string, n int) bool {
	if n <= 0 || n >= len(out) {
		return true
	}
	return out[n]&0xC0 != 0x80
}

// sanitizeLabel produces the on-disk label filename: the §7-sanitized name,
// lowercased so no two label paths can differ only by case (SPEC §4.5). The
// golden fixture maps "INBOX" -> "inbox", "Receipts" -> "receipts".
func sanitizeLabel(name string) string {
	return strings.ToLower(sanitizeSegment(name))
}

// disambiguate assigns a collision-free name for candidate given the set of
// names already used (compared case-insensitively per SPEC §4.4/§4.5). On
// collision it appends " (n)" before the file extension in increasing n.
func disambiguate(candidate string, usedLower map[string]bool) string {
	if !usedLower[strings.ToLower(candidate)] {
		usedLower[strings.ToLower(candidate)] = true
		return candidate
	}
	base, ext := splitExt(candidate)
	for n := 1; ; n++ {
		try := base + " (" + itoa(n) + ")" + ext
		if !usedLower[strings.ToLower(try)] {
			usedLower[strings.ToLower(try)] = true
			return try
		}
	}
}

// splitExt splits a filename into its base and extension (".pdf"), where the
// extension is the final dot-segment. A leading-dot name has no extension.
func splitExt(name string) (base, ext string) {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 {
		return name, ""
	}
	return name[:i], name[i:]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
