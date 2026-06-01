package archive

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/japer-technology/flat-email/internal/model"
	"github.com/japer-technology/flat-email/internal/storage"
)

// SpecVersion is the layout version this producer emits (SPEC §2).
const SpecVersion = 1

// Produce writes a complete SPEC.md-conformant archive for in into backend.
//
// The output is deterministic: given the same Input it produces byte-identical
// files, and re-running it is idempotent (SPEC §6).
func Produce(backend storage.Backend, in model.Input) error {
	keyLen := chooseKeyLen(in)

	createdAt := in.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	createdBy := in.CreatedBy
	if createdBy == "" {
		createdBy = "flat-email/0.0.0"
	}
	syncTime := in.SyncTime
	if syncTime.IsZero() {
		syncTime = time.Now().UTC()
	}
	syncStamp := syncTime.UTC().Format(dateLayout)

	cat := newCatalog()

	for _, acc := range sortedAccounts(in.Accounts) {
		if err := produceAccount(backend, acc, keyLen, syncStamp, cat); err != nil {
			return err
		}
	}

	// Archive manifest (the only place a generation timestamp may live, SPEC §2).
	manifest := map[string]any{
		"format":          "flat-email",
		"specVersion":     SpecVersion,
		"messageKeyBytes": keyLen,
		"createdBy":       createdBy,
		"createdAt":       createdAt.UTC().Format(dateLayout),
	}
	if err := putJSON(backend, "flat-email.json", manifest); err != nil {
		return err
	}

	return writeCatalog(backend, cat)
}

func produceAccount(backend storage.Backend, acc model.Account, keyLen int, syncStamp string, cat *catalog) error {
	account := strings.ToLower(acc.Address)

	// Derive every message first.
	derived := make([]*derivedMessage, 0, len(acc.Messages))
	for _, m := range acc.Messages {
		dm, err := deriveMessage(acc.Address, m, keyLen)
		if err != nil {
			return err
		}
		derived = append(derived, dm)
	}

	// Resolve account-wide label original->filename mapping (handles collisions).
	labelFile := resolveLabelFilenames(acc, derived)
	for _, dm := range derived {
		set := map[string]bool{}
		for _, orig := range dm.labelsOriginal {
			set[labelFile[orig]] = true
		}
		dm.labels = dm.labels[:0]
		for f := range set {
			dm.labels = append(dm.labels, f)
		}
		sort.Strings(dm.labels)
	}

	// Write each message's files and collect index data.
	membership := map[string][]string{} // label filename -> message keys
	threadMembers := map[string][]*derivedMessage{}
	for _, dm := range derived {
		if err := writeMessage(backend, dm, syncStamp); err != nil {
			return err
		}
		for _, l := range dm.labels {
			membership[l] = append(membership[l], dm.key)
		}
		threadMembers[dm.threadKey] = append(threadMembers[dm.threadKey], dm)
		cat.addMessage(account, dm)
	}

	// Threads.
	threadKeys := sortedKeys(threadMembers)
	for _, tk := range threadKeys {
		members := threadMembers[tk]
		sortThreadMembers(members)
		keys := make([]string, len(members))
		for i, m := range members {
			keys[i] = m.key
		}
		base := "accounts/" + account + "/threads/" + tk
		if err := putJSON(backend, base+".json", keys); err != nil {
			return err
		}
		if err := backend.Put(base+".html", []byte(renderThreadHTML(members))); err != nil {
			return err
		}
		cat.addThread(account, tk)
	}

	// Labels.
	labelManifest := map[string]any{}
	for _, dm := range derived {
		for i, orig := range dm.labelsOriginal {
			_ = i
			f := labelFile[orig]
			labelManifest[f] = labelEntry(acc, orig)
		}
	}
	// Ensure labels declared but unused still appear.
	for orig := range acc.Labels {
		f := labelFile[orig]
		if _, ok := labelManifest[f]; !ok {
			labelManifest[f] = labelEntry(acc, orig)
		}
	}
	if err := putJSON(backend, "accounts/"+account+"/labels/labels.json", labelManifest); err != nil {
		return err
	}
	for label, keys := range membership {
		sort.Strings(keys)
		if err := putJSON(backend, "accounts/"+account+"/labels/"+label+".json", keys); err != nil {
			return err
		}
		cat.addLabel(account, label)
	}

	cat.addAccount(account, len(derived), len(threadKeys), len(membership))
	return nil
}

// writeMessage writes the authoritative message.eml plus all derived per-message
// files. firstSeen is preserved from an existing metadata.json so re-syncs do not
// rewrite the marker (SPEC §6, §10).
func writeMessage(backend storage.Backend, dm *derivedMessage, syncStamp string) error {
	emlPath := dm.dirPath + "/message.eml"
	if err := backend.Put(emlPath, dm.raw); err != nil {
		return err
	}

	firstSeen := syncStamp
	if existing, ok := readFirstSeen(backend, dm.dirPath+"/metadata.json"); ok {
		firstSeen = existing
	}

	if dm.bodyText != nil {
		if err := backend.Put(dm.dirPath+"/body.txt", []byte(*dm.bodyText)); err != nil {
			return err
		}
	}
	if dm.bodyHTML != nil {
		if err := backend.Put(dm.dirPath+"/body.html", []byte(*dm.bodyHTML+"\n")); err != nil {
			return err
		}
	}

	if len(dm.attachments) > 0 {
		for _, a := range dm.attachments {
			if err := backend.Put(dm.dirPath+"/attachments/"+a.OnDiskName, dm.payloads[a.OnDiskName]); err != nil {
				return err
			}
		}
		if err := putJSON(backend, dm.dirPath+"/attachments/attachments.json", dm.attachments); err != nil {
			return err
		}
	}

	if err := backend.Put(dm.dirPath+"/email.html", []byte(renderEmailHTML(dm))); err != nil {
		return err
	}

	return putJSON(backend, dm.dirPath+"/metadata.json", dm.metadataObject(firstSeen, syncStamp))
}

func (dm *derivedMessage) metadataObject(firstSeen, lastSeen string) map[string]any {
	md := map[string]any{
		"specVersion":     SpecVersion,
		"messageKey":      dm.key,
		"messageIdHeader": derefString(dm.messageIDHeader),
		"date":            dm.date,
		"dateSource":      dm.dateSource,
		"subject":         dm.subject,
		"from":            addressOrNil(dm.from),
		"to":              addressArray(dm.to),
		"cc":              addressArray(dm.cc),
		"bcc":             addressArray(dm.bcc),
		"replyTo":         addressArray(dm.replyTo),
		"threadKey":       dm.threadKey,
		"labels":          stringArray(dm.labels),
		"flags":           stringArray(dm.flags),
		"attachmentCount": len(dm.attachments),
		"hasBodyText":     dm.bodyText != nil,
		"hasBodyHtml":     dm.bodyHTML != nil,
		"firstSeen":       firstSeen,
		"lastSeen":        lastSeen,
	}
	if len(dm.providerFlags) > 0 {
		md["providerFlags"] = stringArray(dm.providerFlags)
	}
	return md
}

// chooseKeyLen returns 16, or 32 if any two distinct raw messages share a
// 16-byte key prefix (SPEC §4.2 collision handling).
func chooseKeyLen(in model.Input) int {
	seen := map[string]string{} // key16 -> full sha
	for _, acc := range in.Accounts {
		for _, m := range acc.Messages {
			k := hashHex(m.Raw, defaultKeyLen)
			full := hashHex(m.Raw, 32)
			if prev, ok := seen[k]; ok && prev != full {
				return 32
			}
			seen[k] = full
		}
	}
	return defaultKeyLen
}

func resolveLabelFilenames(acc model.Account, derived []*derivedMessage) map[string]string {
	originals := map[string]bool{}
	for orig := range acc.Labels {
		originals[orig] = true
	}
	for _, dm := range derived {
		for _, orig := range dm.labelsOriginal {
			originals[orig] = true
		}
	}
	names := make([]string, 0, len(originals))
	for o := range originals {
		names = append(names, o)
	}
	sort.Strings(names)

	used := map[string]bool{}
	out := map[string]string{}
	for _, o := range names {
		out[o] = disambiguate(sanitizeLabel(o), used)
	}
	return out
}

func labelEntry(acc model.Account, original string) map[string]any {
	l, ok := acc.Labels[original]
	typ, vis := "user", "visible"
	var providerID any
	name := original
	if ok {
		if l.OriginalName != "" {
			name = l.OriginalName
		}
		if l.Type != "" {
			typ = l.Type
		}
		if l.Visibility != "" {
			vis = l.Visibility
		}
		if l.ProviderID != nil {
			providerID = *l.ProviderID
		}
	}
	return map[string]any{
		"originalName": name,
		"providerId":   providerID,
		"type":         typ,
		"visibility":   vis,
	}
}

func sortThreadMembers(members []*derivedMessage) {
	sort.Slice(members, func(i, j int) bool {
		a, b := members[i], members[j]
		if a.date != b.date {
			return a.date < b.date
		}
		return a.key < b.key
	})
}

func readFirstSeen(backend storage.Backend, path string) (string, bool) {
	ok, err := backend.Exists(path)
	if err != nil || !ok {
		return "", false
	}
	data, err := backend.Read(path)
	if err != nil {
		return "", false
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return "", false
	}
	if v, ok := m["firstSeen"].(string); ok {
		return v, true
	}
	return "", false
}

// ---- JSON value helpers (keep empty arrays as [] and absent values as null) ----

func putJSON(backend storage.Backend, path string, v any) error {
	data, err := encodeJSON(v)
	if err != nil {
		return err
	}
	return backend.Put(path, data)
}

func derefString(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func addressOrNil(a *address) any {
	if a == nil {
		return nil
	}
	return a.jsonObject()
}

func addressArray(list []address) []any {
	out := make([]any, 0, len(list))
	for _, a := range list {
		out = append(out, a.jsonObject())
	}
	return out
}

func stringArray(list []string) []any {
	out := make([]any, 0, len(list))
	for _, s := range list {
		out = append(out, s)
	}
	return out
}

func sortedAccounts(accs []model.Account) []model.Account {
	out := append([]model.Account(nil), accs...)
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Address) < strings.ToLower(out[j].Address)
	})
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
