package goldentest

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/japer-technology/flat-email/internal/archive"
	"github.com/japer-technology/flat-email/internal/model"
	"github.com/japer-technology/flat-email/internal/storage"
)

// goldenInput builds the Input that must reproduce tests/golden/archive. The
// label assignments, flags, and sync markers are supplied here because they are
// not derivable from the raw .eml bytes (a real connector supplies them from the
// source mailbox); everything else is parsed from input/*.eml.
func goldenInput(t *testing.T, root string) model.Input {
	t.Helper()
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(root, "input", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return b
	}
	return model.Input{
		SyncTime:  time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		CreatedBy: "flat-email/0.0.0-golden",
		Accounts: []model.Account{
			{
				Address: "me@example.com",
				Labels: map[string]model.Label{
					"INBOX":    {OriginalName: "INBOX", Type: "system", Visibility: "visible"},
					"Receipts": {OriginalName: "Receipts", Type: "user", Visibility: "visible"},
				},
				Messages: []model.Message{
					{Raw: read("msg-a.eml"), Labels: []string{"INBOX", "Receipts"}, Flags: []string{"seen"}},
					{Raw: read("msg-b.eml"), Labels: []string{"INBOX"}},
				},
			},
		},
	}
}

func goldenRoot(t *testing.T) string {
	t.Helper()
	// This test lives in tests/, so the golden fixture is alongside it.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "golden")
}

func TestGoldenArchiveByteExact(t *testing.T) {
	root := goldenRoot(t)
	out := t.TempDir()

	if err := archive.Produce(storage.NewFS(out), goldenInput(t, root)); err != nil {
		t.Fatalf("Produce: %v", err)
	}

	want := collectFiles(t, filepath.Join(root, "archive"))
	got := collectFiles(t, out)

	// Every golden file must be reproduced byte-for-byte.
	for rel, wantBytes := range want {
		gotBytes, ok := got[rel]
		if !ok {
			t.Errorf("missing produced file: %s", rel)
			continue
		}
		if !bytes.Equal(wantBytes, gotBytes) {
			t.Errorf("byte mismatch in %s\n--- want ---\n%s\n--- got ---\n%s", rel, snippet(wantBytes), snippet(gotBytes))
		}
	}
	// No unexpected extra files.
	for rel := range got {
		if _, ok := want[rel]; !ok {
			t.Errorf("unexpected produced file: %s", rel)
		}
	}
}

func TestGoldenIsIdempotent(t *testing.T) {
	root := goldenRoot(t)
	out := t.TempDir()
	backend := storage.NewFS(out)

	if err := archive.Produce(backend, goldenInput(t, root)); err != nil {
		t.Fatalf("first Produce: %v", err)
	}
	first := collectFiles(t, out)

	if err := archive.Produce(backend, goldenInput(t, root)); err != nil {
		t.Fatalf("second Produce: %v", err)
	}
	second := collectFiles(t, out)

	if len(first) != len(second) {
		t.Fatalf("file count changed on re-run: %d -> %d", len(first), len(second))
	}
	for rel, a := range first {
		if !bytes.Equal(a, second[rel]) {
			t.Errorf("re-run changed bytes in %s", rel)
		}
	}
}

func collectFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = b
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func snippet(b []byte) string {
	const max = 600
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}
