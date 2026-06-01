package mailstore

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/japer-technology/flat-email/internal/model"
)

// ReadMaildir parses a Maildir directory (its cur/ and new/ subfolders) into
// messages. The label defaults to the maildir's own directory name. Flags are
// decoded from the Maildir "info" suffix (":2,FRS...") on filenames in cur/.
//
// Files are read in sorted filename order for determinism, though the producer
// re-sorts all output anyway.
func ReadMaildir(dir, label string) ([]model.Message, error) {
	if label == "" {
		label = filepath.Base(dir)
	}
	var msgs []model.Message
	for _, sub := range []string{"new", "cur"} {
		subdir := filepath.Join(dir, sub)
		entries, err := os.ReadDir(subdir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			raw, err := os.ReadFile(filepath.Join(subdir, name))
			if err != nil {
				return nil, err
			}
			m := newMessage(raw, label)
			if f := maildirInfoFlags(name); len(f) > 0 {
				m.Flags = mergeFlags(m.Flags, f)
			}
			msgs = append(msgs, m)
		}
	}
	return msgs, nil
}

// maildirInfoFlags decodes the experimental Maildir flag set from a filename's
// ":2,<flags>" info suffix.
func maildirInfoFlags(name string) []string {
	i := strings.Index(name, ":2,")
	if i < 0 {
		return nil
	}
	var flags []string
	for _, c := range name[i+3:] {
		switch c {
		case 'S':
			flags = append(flags, "seen")
		case 'R':
			flags = append(flags, "answered")
		case 'F':
			flags = append(flags, "flagged")
		case 'T':
			flags = append(flags, "deleted")
		case 'D':
			flags = append(flags, "draft")
		}
	}
	return flags
}

func mergeFlags(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range append(append([]string{}, a...), b...) {
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}
