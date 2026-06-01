// Package storage defines the minimal writer interface every Flat Email
// delivery mode targets (SUGGESTIONS.md §8). Keeping the producer behind this
// interface is what lets future Git/cloud/encrypted backends become "just
// another backend" (IDEAS.md §1, §5) instead of a rewrite.
//
// Only the local-filesystem implementation exists today.
package storage

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Backend is a content sink addressed by archive-root-relative, slash-separated
// paths. Implementations MUST treat paths as POSIX-style regardless of host OS.
type Backend interface {
	// Put writes data at the given relative path, creating parents as needed.
	// Writing the same path twice with the same bytes is a harmless no-op-by-
	// value; callers enforce the idempotency invariant (SPEC §6) above this.
	Put(path string, data []byte) error
	// Exists reports whether a path already holds content.
	Exists(path string) (bool, error)
	// Read returns the bytes previously stored at path.
	Read(path string) ([]byte, error)
	// List returns all stored paths under prefix, in lexicographic order.
	List(prefix string) ([]string, error)
}

// FS is a Backend backed by a local directory.
type FS struct {
	root string
}

// NewFS returns a filesystem backend rooted at dir.
func NewFS(dir string) *FS { return &FS{root: dir} }

func (f *FS) native(p string) string {
	return filepath.Join(f.root, filepath.FromSlash(p))
}

// Put implements Backend.
func (f *FS) Put(path string, data []byte) error {
	full := f.native(path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}

// Exists implements Backend.
func (f *FS) Exists(path string) (bool, error) {
	_, err := os.Stat(f.native(path))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// Read implements Backend.
func (f *FS) Read(path string) ([]byte, error) {
	return os.ReadFile(f.native(path))
}

// List implements Backend.
func (f *FS) List(prefix string) ([]string, error) {
	base := f.native(prefix)
	var out []string
	err := filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(f.root, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return strings.Compare(out[i], out[j]) < 0 })
	return out, nil
}
