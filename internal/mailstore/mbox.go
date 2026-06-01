// Package mailstore implements read-only connectors for local mail stores
// (mbox files and Maildir directories). They translate stored mail into
// model.Message records for the archive producer and never modify the source
// (SPEC §1 read-only guarantee).
package mailstore

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/japer-technology/flat-email/internal/model"
)

// ReadMbox parses an mbox file into messages. Every message is assigned the
// given label (typically the mailbox/folder name). Flags are read from the
// Status/X-Status headers when present.
//
// The classic "From " line that separates mbox messages is metadata, not part of
// the RFC 5322 message, so it is stripped; the remaining bytes are the
// authoritative message and are de-quoted of mbox ">From " escaping.
func ReadMbox(r io.Reader, label string) ([]model.Message, error) {
	br := bufio.NewReader(r)
	var msgs []model.Message
	var cur bytes.Buffer
	started := false

	flush := func() {
		if !started {
			return
		}
		raw := unescapeFromQuoting(cur.Bytes())
		msgs = append(msgs, newMessage(raw, label))
		cur.Reset()
	}

	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if isFromLine(line) {
				flush()
				started = true
				continue // drop the "From " separator line
			}
			if started {
				cur.Write(line)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	flush()
	return msgs, nil
}

// ReadMboxFile parses an mbox file at path, using the file's base name (without
// extension) as the label when label is empty.
func ReadMboxFile(path, label string) ([]model.Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if label == "" {
		label = labelFromPath(path)
	}
	return ReadMbox(f, label)
}

func isFromLine(line []byte) bool {
	return bytes.HasPrefix(line, []byte("From "))
}

// unescapeFromQuoting reverses mbox ">From " quoting (and ">>From ", etc.).
func unescapeFromQuoting(b []byte) []byte {
	lines := bytes.Split(b, []byte("\n"))
	for i, ln := range lines {
		trimmed := ln
		gt := 0
		for gt < len(trimmed) && trimmed[gt] == '>' {
			gt++
		}
		if gt > 0 && bytes.HasPrefix(trimmed[gt:], []byte("From ")) {
			lines[i] = ln[1:] // drop one leading '>'
		}
	}
	return bytes.Join(lines, []byte("\n"))
}

func newMessage(raw []byte, label string) model.Message {
	m := model.Message{Raw: raw}
	if label != "" {
		m.Labels = []string{label}
	}
	m.Flags = flagsFromHeaders(raw)
	return m
}

// flagsFromHeaders extracts IMAP-ish flags from mbox Status/X-Status headers.
func flagsFromHeaders(raw []byte) []string {
	header := raw
	if i := bytes.Index(raw, []byte("\r\n\r\n")); i >= 0 {
		header = raw[:i]
	} else if i := bytes.Index(raw, []byte("\n\n")); i >= 0 {
		header = raw[:i]
	}
	var flags []string
	add := func(f string) {
		for _, e := range flags {
			if e == f {
				return
			}
		}
		flags = append(flags, f)
	}
	for _, line := range strings.Split(string(header), "\n") {
		line = strings.TrimRight(line, "\r")
		low := strings.ToLower(line)
		if strings.HasPrefix(low, "status:") || strings.HasPrefix(low, "x-status:") {
			val := line[strings.IndexByte(line, ':')+1:]
			for _, c := range val {
				switch c {
				case 'R':
					add("seen")
				case 'A':
					add("answered")
				case 'F':
					add("flagged")
				case 'D':
					add("deleted")
				case 'T':
					add("draft")
				}
			}
		}
	}
	return flags
}

func labelFromPath(path string) string {
	base := path
	if i := strings.LastIndexAny(base, "/\\"); i >= 0 {
		base = base[i+1:]
	}
	if i := strings.LastIndexByte(base, '.'); i > 0 {
		base = base[:i]
	}
	return base
}

// normalizeToCRLF ensures the authoritative bytes use CRLF line endings, as
// required for valid RFC 5322 on the wire. Local stores often use bare LF; the
// content-addressed key is taken over these normalised bytes so the same logical
// message keys identically regardless of the source store's newline convention.
func normalizeToCRLF(b []byte) []byte {
	// First collapse any existing CRLF to LF, then expand all LF to CRLF, so
	// mixed input becomes uniform.
	s := bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	s = bytes.ReplaceAll(s, []byte("\r"), []byte("\n"))
	s = bytes.ReplaceAll(s, []byte("\n"), []byte("\r\n"))
	// Trim a single trailing blank line introduced by the mbox record separator.
	s = bytes.TrimRight(s, "\r\n")
	return append(s, '\r', '\n')
}
