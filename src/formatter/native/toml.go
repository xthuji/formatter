package native

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/formatter/formatter/src/formatter"
	"github.com/pelletier/go-toml/v2"
)

type TOMLFormatter struct{}

func NewTOMLFormatter() *TOMLFormatter {
	return &TOMLFormatter{}
}

func (f *TOMLFormatter) Format(input []byte, lang string, opts formatter.FormatOptions) ([]byte, error) {
	var v interface{}
	if err := toml.Unmarshal(input, &v); err != nil {
		return nil, fmt.Errorf("toml unmarshal: %w", err)
	}

	// 构建缩进符号: TabIndent → "\t", >0 → N 个空格, 默认 → 2 个空格
	indentSymbol := "  "
	if opts.TabWidth == formatter.TabIndent {
		indentSymbol = "\t"
	} else if opts.TabWidth > 0 {
		indentSymbol = strings.Repeat(" ", opts.TabWidth)
	}

	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.SetIndentSymbol(indentSymbol)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("toml marshal: %w", err)
	}

	return buf.Bytes(), nil
}

func (f *TOMLFormatter) SupportedLanguages() []string {
	return []string{"toml"}
}

func (f *TOMLFormatter) Name() string {
	return "native-toml"
}

func (f *TOMLFormatter) Type() formatter.BackendType {
	return formatter.NativeGo
}