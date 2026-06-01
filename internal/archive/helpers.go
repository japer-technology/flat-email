package archive

import (
	"bytes"
	"io"

	"golang.org/x/text/encoding/htmlindex"
)

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

func readAll(r io.Reader) []byte {
	if r == nil {
		return nil
	}
	b, _ := io.ReadAll(r)
	return b
}

// charsetReader lets mime.WordDecoder decode non-UTF-8 RFC 2047 encoded-words
// (e.g. ISO-8859-1 subjects) using the same charset table as body decoding.
func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	enc, err := htmlindex.Get(charset)
	if err != nil {
		return input, nil
	}
	return enc.NewDecoder().Reader(input), nil
}
