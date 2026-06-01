package archive

import (
	"bytes"
	"sort"
	"strings"
)

// memBackend is an in-memory storage.Backend used by tests.
type memBackend struct {
	files map[string][]byte
}

func newMemBackend() *memBackend { return &memBackend{files: map[string][]byte{}} }

func (m *memBackend) Put(path string, data []byte) error {
	cp := make([]byte, len(data))
	copy(cp, data)
	m.files[path] = cp
	return nil
}

func (m *memBackend) Exists(path string) (bool, error) {
	_, ok := m.files[path]
	return ok, nil
}

func (m *memBackend) Read(path string) ([]byte, error) {
	return m.files[path], nil
}

func (m *memBackend) List(prefix string) ([]string, error) {
	var out []string
	for k := range m.files {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (m *memBackend) equal(other *memBackend) bool {
	if len(m.files) != len(other.files) {
		return false
	}
	for k, v := range m.files {
		ov, ok := other.files[k]
		if !ok || !bytes.Equal(v, ov) {
			return false
		}
	}
	return true
}
