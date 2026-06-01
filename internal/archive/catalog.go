package archive

import (
	"sort"

	"github.com/japer-technology/flat-email/internal/storage"
)

// catalog accumulates the archive-wide index emitted as catalog.json/catalog.js
// (SPEC §11). All lists are sorted deterministically before serialisation.
type catalog struct {
	accounts []map[string]any
	messages []map[string]any
	threads  []map[string]any
	labels   []map[string]any

	nAccounts, nMessages, nThreads, nLabels int
}

func newCatalog() *catalog { return &catalog{} }

func (c *catalog) addAccount(account string, messages, threads, labels int) {
	c.accounts = append(c.accounts, map[string]any{
		"address": account,
		"counts": map[string]any{
			"messages": messages,
			"threads":  threads,
			"labels":   labels,
		},
		"messagesPath":       "accounts/" + account + "/messages",
		"threadsPath":        "accounts/" + account + "/threads",
		"labelsManifestPath": "accounts/" + account + "/labels/labels.json",
	})
	c.nAccounts++
	c.nMessages += messages
	c.nThreads += threads
	c.nLabels += labels
}

func (c *catalog) addMessage(account string, dm *derivedMessage) {
	c.messages = append(c.messages, map[string]any{
		"messageKey":      dm.key,
		"account":         account,
		"date":            dm.date,
		"subject":         dm.subject,
		"from":            addressOrNil(dm.from),
		"threadKey":       dm.threadKey,
		"labels":          stringArray(dm.labels),
		"attachmentCount": len(dm.attachments),
		"path":            dm.dirPath,
	})
}

func (c *catalog) addThread(account, threadKey string) {
	c.threads = append(c.threads, map[string]any{
		"threadKey": threadKey,
		"account":   account,
		"path":      "accounts/" + account + "/threads/" + threadKey + ".json",
	})
}

func (c *catalog) addLabel(account, name string) {
	c.labels = append(c.labels, map[string]any{
		"name":    name,
		"account": account,
		"path":    "accounts/" + account + "/labels/" + name + ".json",
	})
}

func (c *catalog) document() map[string]any {
	sort.Slice(c.accounts, func(i, j int) bool {
		return c.accounts[i]["address"].(string) < c.accounts[j]["address"].(string)
	})
	sort.Slice(c.messages, func(i, j int) bool {
		return c.messages[i]["messageKey"].(string) < c.messages[j]["messageKey"].(string)
	})
	sort.Slice(c.threads, func(i, j int) bool {
		return c.threads[i]["path"].(string) < c.threads[j]["path"].(string)
	})
	sort.Slice(c.labels, func(i, j int) bool {
		return c.labels[i]["path"].(string) < c.labels[j]["path"].(string)
	})

	return map[string]any{
		"specVersion": SpecVersion,
		"counts": map[string]any{
			"accounts": c.nAccounts,
			"messages": c.nMessages,
			"threads":  c.nThreads,
			"labels":   c.nLabels,
		},
		"accounts": c.accounts,
		"messages": c.messages,
		"threads":  c.threads,
		"labels":   c.labels,
	}
}

// writeCatalog emits catalog.json and the byte-derived catalog.js wrapper.
func writeCatalog(backend storage.Backend, c *catalog) error {
	data, err := encodeJSON(c.document())
	if err != nil {
		return err
	}
	if err := backend.Put("catalog.json", data); err != nil {
		return err
	}
	// catalog.js is exactly catalog.json (without its trailing newline) assigned
	// to a global, then ";\n" (SPEC §11).
	js := append([]byte("window.FLAT_EMAIL_CATALOG = "), trimTrailingNewline(data)...)
	js = append(js, ";\n"...)
	return backend.Put("catalog.js", js)
}

func trimTrailingNewline(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		return b[:len(b)-1]
	}
	return b
}
