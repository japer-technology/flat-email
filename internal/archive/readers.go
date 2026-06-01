package archive

import (
	"strings"
)

// renderEmailHTML produces the self-contained per-message reader page (SPEC §13).
// All styles are inlined and the sanitized body is embedded so the file renders
// identically from disk with no network access.
func renderEmailHTML(dm *derivedMessage) string {
	date := dm.date
	if dm.dateSource == "unknown" {
		date = "unknown"
	}

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n")
	b.WriteString("<html><head><meta charset=\"utf-8\"><style>body{font-family:sans-serif;margin:2rem}</style></head>\n")
	b.WriteString("<body>\n")
	b.WriteString("<header>\n")
	b.WriteString("<div><strong>From:</strong> " + escapeHTMLText(fromDisplay(dm.from)) + "</div>\n")
	b.WriteString("<div><strong>To:</strong> " + escapeHTMLText(addressListDisplay(dm.to)) + "</div>\n")
	b.WriteString("<div><strong>Subject:</strong> " + escapeHTMLText(dm.subject) + "</div>\n")
	b.WriteString("<div><strong>Date:</strong> " + escapeHTMLText(date) + "</div>\n")
	b.WriteString("</header>\n")
	b.WriteString("<hr>\n")
	b.WriteString("<section>\n")
	b.WriteString(bodySection(dm) + "\n")
	b.WriteString("</section>\n")
	b.WriteString("</body></html>\n")
	return b.String()
}

func bodySection(dm *derivedMessage) string {
	if dm.bodyHTML != nil {
		return *dm.bodyHTML
	}
	if dm.bodyText != nil {
		return "<pre>" + escapeHTMLText(strings.TrimRight(*dm.bodyText, "\n")) + "</pre>"
	}
	return "<pre></pre>"
}

// renderThreadHTML produces the standalone thread reader listing member subjects
// in thread order (SPEC §8).
func renderThreadHTML(members []*derivedMessage) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n")
	b.WriteString("<html><head><meta charset=\"utf-8\"></head>\n")
	b.WriteString("<body>\n")
	b.WriteString("<h1>Thread</h1>\n")
	b.WriteString("<ol>\n")
	for _, m := range members {
		b.WriteString("<li>" + escapeHTMLText(m.subject) + "</li>\n")
	}
	b.WriteString("</ol>\n")
	b.WriteString("</body></html>\n")
	return b.String()
}

func fromDisplay(a *address) string {
	if a == nil {
		return ""
	}
	return addressDisplay(*a)
}

func addressDisplay(a address) string {
	if a.name != nil && *a.name != "" {
		return *a.name + " <" + a.address + ">"
	}
	return a.address
}

func addressListDisplay(list []address) string {
	parts := make([]string, 0, len(list))
	for _, a := range list {
		parts = append(parts, addressDisplay(a))
	}
	return strings.Join(parts, ", ")
}
