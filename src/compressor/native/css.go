package native

import (
	"bytes"
	"fmt"

	"github.com/formatter/formatter/src/compressor"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
)

// CSSCompressor 使用 tdewolff/minify 实现 CSS 压缩，
// 替代外部 esbuild 二进制 (节省 ~10M)。
type CSSCompressor struct {
	m *minify.M
}

func NewCSSCompressor() *CSSCompressor {
	m := minify.New()
	m.Add("text/css", &css.Minifier{})
	return &CSSCompressor{m: m}
}

func (c *CSSCompressor) Compress(input []byte, lang string, opts compressor.CompressOptions) ([]byte, error) {
	var buf bytes.Buffer
	if err := c.m.Minify("text/css", &buf, bytes.NewReader(input)); err != nil {
		return nil, fmt.Errorf("css minify: %w", err)
	}
	return buf.Bytes(), nil
}

func (c *CSSCompressor) SupportedLanguages() []string {
	return []string{"css"}
}

func (c *CSSCompressor) Name() string {
	return "native-css"
}

func (c *CSSCompressor) Type() compressor.BackendType {
	return compressor.NativeGo
}
