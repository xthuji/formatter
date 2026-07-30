package native

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/formatter/formatter/src/formatter"
)

type JSONFormatter struct{}

func NewJSONFormatter() *JSONFormatter {
	return &JSONFormatter{}
}

func (f *JSONFormatter) Format(input []byte, lang string, opts formatter.FormatOptions) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal(input, &v); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	indent := "    "
	if opts.TabWidth == formatter.TabIndent {
		indent = "\t"
	} else if opts.TabWidth > 0 {
		indent = repeatSpace(opts.TabWidth)
	}

	out, err := json.MarshalIndent(v, "", indent)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}
	out = append(out, '\n')
	return out, nil
}

func (f *JSONFormatter) SupportedLanguages() []string {
	return []string{"json"}
}

func (f *JSONFormatter) Name() string {
	return "native-json"
}

func (f *JSONFormatter) Type() formatter.BackendType {
	return formatter.NativeGo
}

func repeatSpace(n int) string {
	return strings.Repeat(" ", n)
}
