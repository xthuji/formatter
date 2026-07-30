package native

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/formatter/formatter/src/compressor"
)

type XMLCompressor struct{}

func NewXMLCompressor() *XMLCompressor {
	return &XMLCompressor{}
}

func (c *XMLCompressor) Compress(input []byte, lang string, opts compressor.CompressOptions) ([]byte, error) {
	data := input

	if opts.RemoveComments {
		data = removeXMLComments(data)
	}

	var buf bytes.Buffer
	dec := xml.NewDecoder(bytes.NewReader(data))
	enc := xml.NewEncoder(&buf)

	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xml decode: %w", err)
		}

		switch t := token.(type) {
		case xml.CharData:
			if opts.RemoveWhitespace {
				trimmed := strings.TrimSpace(string(t))
				if trimmed == "" {
					continue
				}
				token = xml.CharData(trimmed)
			}
		case xml.StartElement:
			if opts.RemoveWhitespace {
				token = xml.StartElement{Name: t.Name, Attr: t.Attr}
			}
		}

		if err := enc.EncodeToken(token); err != nil {
			return nil, fmt.Errorf("xml encode: %w", err)
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, fmt.Errorf("xml flush: %w", err)
	}

	out := buf.Bytes()
	if opts.RemoveWhitespace {
		out = removeNewlines(out)
	}
	return out, nil
}

var xmlCommentRe = regexp.MustCompile(`<!--.*?-->`)

func removeXMLComments(data []byte) []byte {
	for {
		m := xmlCommentRe.FindIndex(data)
		if m == nil {
			break
		}
		data = append(data[:m[0]], data[m[1]:]...)
	}
	return data
}

func removeNewlines(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		b := data[i]
		if b == '\n' || b == '\r' || b == '\t' {
			continue
		}
		out = append(out, b)
	}
	return out
}

func (c *XMLCompressor) SupportedLanguages() []string {
	return []string{"xml"}
}

func (c *XMLCompressor) Name() string {
	return "native-xml"
}

func (c *XMLCompressor) Type() compressor.BackendType {
	return compressor.NativeGo
}
