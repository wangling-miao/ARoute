package content

import (
	"bytes"
	"html"
	"net/url"
	"strings"

	"github.com/wangling-miao/aroute/sdk/interfaces"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var allowedRichTextTags = map[string]bool{
	"a": true, "b": true, "blockquote": true, "br": true, "code": true,
	"div": true, "em": true, "figcaption": true, "figure": true, "h1": true,
	"h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "hr": true,
	"i": true, "img": true, "li": true, "ol": true, "p": true, "pre": true,
	"s": true, "span": true, "strong": true, "table": true, "tbody": true,
	"td": true, "th": true, "thead": true, "tr": true, "u": true, "ul": true,
}

var droppedRichTextTags = map[string]bool{
	"base": true, "embed": true, "form": true, "iframe": true, "link": true,
	"math": true, "meta": true, "object": true, "script": true, "style": true,
	"svg": true, "template": true,
}

func sanitizeRichTextFields(ct *interfaces.ContentType, data map[string]interface{}) {
	for _, field := range ct.Fields {
		if field.Type != "richtext" {
			continue
		}
		raw, ok := data[field.Name].(string)
		if !ok || raw == "" {
			continue
		}
		data[field.Name] = sanitizeRichText(raw)
	}
}

func sanitizeRichText(raw string) string {
	context := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := xhtml.ParseFragment(strings.NewReader(raw), context)
	if err != nil {
		return html.EscapeString(raw)
	}

	var out bytes.Buffer
	for _, node := range nodes {
		for _, clean := range sanitizeHTMLNode(node) {
			_ = xhtml.Render(&out, clean)
		}
	}
	return out.String()
}

func sanitizeHTMLNode(n *xhtml.Node) []*xhtml.Node {
	switch n.Type {
	case xhtml.TextNode:
		return []*xhtml.Node{{Type: xhtml.TextNode, Data: n.Data}}
	case xhtml.ElementNode:
		tag := strings.ToLower(n.Data)
		if droppedRichTextTags[tag] {
			return nil
		}

		var children []*xhtml.Node
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			children = append(children, sanitizeHTMLNode(child)...)
		}
		if !allowedRichTextTags[tag] {
			return children
		}

		clean := &xhtml.Node{Type: xhtml.ElementNode, Data: tag}
		clean.Attr = sanitizeHTMLAttrs(tag, n.Attr)
		for _, child := range children {
			clean.AppendChild(child)
		}
		return []*xhtml.Node{clean}
	case xhtml.DocumentNode:
		var children []*xhtml.Node
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			children = append(children, sanitizeHTMLNode(child)...)
		}
		return children
	default:
		return nil
	}
}

func sanitizeHTMLAttrs(tag string, attrs []xhtml.Attribute) []xhtml.Attribute {
	clean := make([]xhtml.Attribute, 0, len(attrs))
	for _, attr := range attrs {
		key := strings.ToLower(strings.TrimSpace(attr.Key))
		if key == "" || strings.HasPrefix(key, "on") || key == "style" || strings.HasPrefix(key, "xmlns") {
			continue
		}

		switch {
		case key == "class", key == "title", strings.HasPrefix(key, "aria-"), strings.HasPrefix(key, "data-"):
			clean = append(clean, xhtml.Attribute{Key: key, Val: attr.Val})
		case tag == "a" && key == "href":
			if isSafeRichTextURL(attr.Val, true) {
				clean = append(clean, xhtml.Attribute{Key: key, Val: attr.Val})
			}
		case tag == "a" && (key == "target" || key == "rel"):
			clean = append(clean, xhtml.Attribute{Key: key, Val: attr.Val})
		case tag == "img" && key == "src":
			if isSafeRichTextURL(attr.Val, false) {
				clean = append(clean, xhtml.Attribute{Key: key, Val: attr.Val})
			}
		case tag == "img" && (key == "alt" || key == "width" || key == "height" || key == "loading"):
			clean = append(clean, xhtml.Attribute{Key: key, Val: attr.Val})
		}
	}
	return clean
}

func isSafeRichTextURL(raw string, allowMail bool) bool {
	value := strings.TrimSpace(raw)
	if value == "" || strings.ContainsAny(value, "\x00\r\n\t") {
		return false
	}
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return true
	}
	if strings.HasPrefix(value, "#") {
		return true
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed.Host != ""
	case "mailto", "tel":
		return allowMail
	default:
		return false
	}
}
