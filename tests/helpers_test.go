package goldentest

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Tests run from the tests/ directory; the repo root is its parent.
	return filepath.Dir(wd)
}

func jsonUnmarshalReader(r io.Reader, v any) error {
	dec := json.NewDecoder(r)
	return dec.Decode(v)
}
