package archive

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// htmlVoidElements have no closing tag and no children. They are serialised as
// "<tag ...>" (no trailing slash) to match the format's canonical HTML output;
// x/net/html's own renderer would emit "<tag .../>", which the golden fixture
// does not use.
var htmlVoidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// htmlRemoveElements are stripped entirely (including their subtree) for safety
// (SPEC §13): scripting, embedding, and active elements.
var htmlRemoveElements = map[string]bool{
	"script": true, "iframe": true, "object": true, "embed": true,
	"frame": true, "frameset": true, "applet": true, "form": true, "base": true,
}

// sanitizeHTML applies the SPEC §13 safety policy to a raw HTML body and returns
// the canonical sanitized serialisation (no trailing newline). cids maps a
// Content-ID (without angle brackets) to the message-relative attachment path so
// inline cid: images render offline.
//
// The transform is deterministic: the same input bytes always yield the same
// output bytes, which is part of the format contract exercised by the golden
// fixture.
func sanitizeHTML(raw string, cids map[string]string) (string, error) {
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return "", err
	}
	sanitizeChildren(doc, cids)
	var buf bytes.Buffer
	renderNode(&buf, doc)
	return buf.String(), nil
}

func sanitizeChildren(n *html.Node, cids map[string]string) {
	var next *html.Node
	for c := n.FirstChild; c != nil; c = next {
		next = c.NextSibling
		if c.Type == html.ElementNode {
			if htmlRemoveElements[c.Data] {
				n.RemoveChild(c)
				continue
			}
			if c.DataAtom == atom.Meta && metaIsRefresh(c) {
				n.RemoveChild(c)
				continue
			}
			sanitizeAttrs(c, cids)
		}
		sanitizeChildren(c, cids)
	}
}

func metaIsRefresh(n *html.Node) bool {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, "http-equiv") && strings.EqualFold(strings.TrimSpace(a.Val), "refresh") {
			return true
		}
	}
	return false
}

func sanitizeAttrs(n *html.Node, cids map[string]string) {
	out := n.Attr[:0]
	for _, a := range n.Attr {
		k := strings.ToLower(a.Key)

		// Drop inline event handlers.
		if strings.HasPrefix(k, "on") {
			continue
		}

		switch k {
		case "src":
			v := strings.TrimSpace(a.Val)
			low := strings.ToLower(v)
			switch {
			case strings.HasPrefix(low, "cid:"):
				if name, ok := cids[v[len("cid:"):]]; ok {
					a.Val = name
				} else {
					continue // unresolved inline reference: drop it
				}
			case isRemoteURL(low):
				// Neutralise remote resources so opening never phones home.
				a.Key = "data-blocked-src"
			case hasActiveScheme(low):
				continue
			}
		case "href":
			low := strings.ToLower(strings.TrimSpace(a.Val))
			if hasActiveScheme(low) || (n.DataAtom != atom.A && isRemoteURL(low)) {
				continue
			}
		}
		out = append(out, a)
	}
	n.Attr = out

	// Make links inert against the local page.
	if n.DataAtom == atom.A {
		n.Attr = append(n.Attr,
			html.Attribute{Key: "target", Val: "_blank"},
			html.Attribute{Key: "rel", Val: "noopener noreferrer nofollow"},
		)
	}
}

func isRemoteURL(low string) bool {
	return strings.HasPrefix(low, "http:") || strings.HasPrefix(low, "https:") || strings.HasPrefix(low, "//")
}

func hasActiveScheme(low string) bool {
	return strings.HasPrefix(low, "javascript:") || strings.HasPrefix(low, "vbscript:") || strings.HasPrefix(low, "data:")
}

// renderNode serialises a parsed HTML tree using the format's canonical style:
// void elements without a trailing slash, comments dropped, attributes double-
// quoted in their existing order.
func renderNode(buf *bytes.Buffer, n *html.Node) {
	switch n.Type {
	case html.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderNode(buf, c)
		}
		return
	case html.TextNode:
		buf.WriteString(escapeHTMLText(n.Data))
		return
	case html.DoctypeNode:
		buf.WriteString("<!DOCTYPE ")
		buf.WriteString(n.Data)
		buf.WriteString(">")
		return
	case html.CommentNode:
		return
	case html.ElementNode:
		// handled below
	default:
		return
	}

	buf.WriteByte('<')
	buf.WriteString(n.Data)
	for _, a := range n.Attr {
		buf.WriteByte(' ')
		buf.WriteString(a.Key)
		buf.WriteString(`="`)
		buf.WriteString(escapeHTMLAttr(a.Val))
		buf.WriteByte('"')
	}
	buf.WriteByte('>')

	if htmlVoidElements[n.Data] {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		renderNode(buf, c)
	}
	buf.WriteString("</")
	buf.WriteString(n.Data)
	buf.WriteByte('>')
}

func escapeHTMLText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func escapeHTMLAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	return s
}
