package native

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"

	"github.com/formatter/formatter/src/formatter"
)

type XMLFormatter struct{}

func NewXMLFormatter() *XMLFormatter {
	return &XMLFormatter{}
}

func (f *XMLFormatter) Format(input []byte, lang string, opts formatter.FormatOptions) ([]byte, error) {
	indent := "    "
	if opts.TabWidth == formatter.TabIndent {
		indent = "\t"
	} else if opts.TabWidth > 0 {
		indent = repeatSpace(opts.TabWidth)
	}

	var buf bytes.Buffer
	dec := xml.NewDecoder(bytes.NewReader(input))
	enc := xml.NewEncoder(&buf)
	enc.Indent("", indent)

	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xml decode: %w", err)
		}
		if err := enc.EncodeToken(token); err != nil {
			return nil, fmt.Errorf("xml encode: %w", err)
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, fmt.Errorf("xml flush: %w", err)
	}
	out := buf.Bytes()
	if len(out) == 0 || out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return out, nil
}

func (f *XMLFormatter) SupportedLanguages() []string {
	return []string{"xml"}
}

func (f *XMLFormatter) Name() string {
	return "native-xml"
}

func (f *XMLFormatter) Type() formatter.BackendType {
	return formatter.NativeGo
}
