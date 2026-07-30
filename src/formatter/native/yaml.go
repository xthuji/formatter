package native

import (
	"bytes"
	"fmt"

	"github.com/formatter/formatter/src/formatter"
	"gopkg.in/yaml.v3"
)

type YAMLFormatter struct{}

func NewYAMLFormatter() *YAMLFormatter {
	return &YAMLFormatter{}
}

func (f *YAMLFormatter) Format(input []byte, lang string, opts formatter.FormatOptions) ([]byte, error) {
	var v interface{}
	if err := yaml.Unmarshal(input, &v); err != nil {
		return nil, fmt.Errorf("yaml unmarshal: %w", err)
	}

	indent := 2
	if opts.TabWidth > 0 {
		indent = opts.TabWidth
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(indent)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("yaml marshal: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("yaml close: %w", err)
	}
	return buf.Bytes(), nil
}

func (f *YAMLFormatter) SupportedLanguages() []string {
	return []string{"yaml", "yml"}
}

func (f *YAMLFormatter) Name() string {
	return "native-yaml"
}

func (f *YAMLFormatter) Type() formatter.BackendType {
	return formatter.NativeGo
}
