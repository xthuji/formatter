package native

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/formatter/formatter/src/compressor"
)

type JSONCompressor struct{}

func NewJSONCompressor() *JSONCompressor {
	return &JSONCompressor{}
}

func (c *JSONCompressor) Compress(input []byte, lang string, opts compressor.CompressOptions) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal(input, &v); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	out, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}

	var buf bytes.Buffer
	buf.Write(out)
	return buf.Bytes(), nil
}

func (c *JSONCompressor) SupportedLanguages() []string {
	return []string{"json"}
}

func (c *JSONCompressor) Name() string {
	return "native-json"
}

func (c *JSONCompressor) Type() compressor.BackendType {
	return compressor.NativeGo
}
