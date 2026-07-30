package native

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/formatter/formatter/src/formatter"
	"golang.org/x/net/html"
)

type HTMLFormatter struct{}

func NewHTMLFormatter() *HTMLFormatter {
	return &HTMLFormatter{}
}

func (f *HTMLFormatter) Format(input []byte, lang string, opts formatter.FormatOptions) ([]byte, error) {
	indent := "    "
	if opts.TabWidth == formatter.TabIndent {
		indent = "\t"
	} else if opts.TabWidth > 0 {
		indent = repeatSpace(opts.TabWidth)
	}

	doc, err := html.Parse(strings.NewReader(string(input)))
	if err != nil {
		return nil, fmt.Errorf("html parse: %w", err)
	}

	var buf bytes.Buffer
	writeNode(&buf, doc, indent, 0)
	return buf.Bytes(), nil
}

func writeNode(w *bytes.Buffer, n *html.Node, indent string, depth int) {
	switch n.Type {
	case html.CommentNode:
		writeIndent(w, indent, depth)
		w.WriteString("<!--")
		w.WriteString(n.Data)
		w.WriteString("-->\n")
	case html.DoctypeNode:
		writeIndent(w, indent, depth)
		w.WriteString("<!DOCTYPE ")
		w.WriteString(n.Data)
		w.WriteString(">\n")
	case html.TextNode:
		text := strings.TrimSpace(n.Data)
		if text != "" {
			writeIndent(w, indent, depth)
			w.WriteString(text)
			w.WriteString("\n")
		}
	case html.ElementNode:
		writeIndent(w, indent, depth)
		w.WriteByte('<')
		w.WriteString(n.Data)
		for _, attr := range n.Attr {
			w.WriteByte(' ')
			w.WriteString(attr.Key)
			if attr.Val != "" {
				w.WriteString(`="`)
				w.WriteString(attr.Val)
				w.WriteByte('"')
			}
		}
		if isVoidElement(n.Data) {
			w.WriteString(">\n")
			return
		}
		w.WriteString(">\n")
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			writeNode(w, c, indent, depth+1)
		}
		writeIndent(w, indent, depth)
		w.WriteString("</")
		w.WriteString(n.Data)
		w.WriteString(">\n")
	case html.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			writeNode(w, c, indent, depth)
		}
	}
}

func writeIndent(w *bytes.Buffer, indent string, depth int) {
	for i := 0; i < depth; i++ {
		w.WriteString(indent)
	}
}

func isVoidElement(tag string) bool {
	switch tag {
	case "area", "base", "br", "col", "embed", "hr", "img", "input",
		"link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

func (f *HTMLFormatter) SupportedLanguages() []string {
	return []string{"html"}
}

func (f *HTMLFormatter) Name() string {
	return "native-html"
}

func (f *HTMLFormatter) Type() formatter.BackendType {
	return formatter.NativeGo
}
