package native

import (
	"bytes"
	"fmt"

	"github.com/formatter/formatter/src/compressor"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"
)

// HTMLCompressor 使用 tdewolff/minify 实现 HTML 压缩，
// 替代外部 minhtml 二进制 (节省 ~17M)。
type HTMLCompressor struct {
	m *minify.M
}

func NewHTMLCompressor() *HTMLCompressor {
	m := minify.New()
	m.Add("text/html", &html.Minifier{
		KeepComments:            false,
		KeepConditionalComments: false,
		KeepDefaultAttrVals:     true,
		KeepDocumentTags:        true,
		KeepEndTags:             true,
		KeepQuotes:              false,
	})
	return &HTMLCompressor{m: m}
}

func (c *HTMLCompressor) Compress(input []byte, lang string, opts compressor.CompressOptions) ([]byte, error) {
	var buf bytes.Buffer
	if err := c.m.Minify("text/html", &buf, bytes.NewReader(input)); err != nil {
		return nil, fmt.Errorf("html minify: %w", err)
	}
	return buf.Bytes(), nil
}

func (c *HTMLCompressor) SupportedLanguages() []string {
	return []string{"html"}
}

func (c *HTMLCompressor) Name() string {
	return "native-html"
}

func (c *HTMLCompressor) Type() compressor.BackendType {
	return compressor.NativeGo
}
