package native

import (
	"bytes"
	"fmt"

	"github.com/formatter/formatter/src/compressor"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/js"
)

// JSCompressor 使用 tdewolff/minify 实现 JavaScript 压缩，
// 替代外部 esbuild 二进制 (节省 ~10M)。
// 仅支持 JavaScript，不支持 TypeScript (TS 类型注解会导致解析失败)。
type JSCompressor struct {
	m *minify.M
}

func NewJSCompressor() *JSCompressor {
	m := minify.New()
	m.Add("text/javascript", &js.Minifier{})
	m.Add("application/javascript", &js.Minifier{})
	return &JSCompressor{m: m}
}

func (c *JSCompressor) Compress(input []byte, lang string, opts compressor.CompressOptions) ([]byte, error) {
	var buf bytes.Buffer
	if err := c.m.Minify("text/javascript", &buf, bytes.NewReader(input)); err != nil {
		return nil, fmt.Errorf("js minify: %w", err)
	}
	return buf.Bytes(), nil
}

func (c *JSCompressor) SupportedLanguages() []string {
	return []string{"javascript"}
}

func (c *JSCompressor) Name() string {
	return "native-js"
}

func (c *JSCompressor) Type() compressor.BackendType {
	return compressor.NativeGo
}
