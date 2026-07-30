package native

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/formatter/formatter/src/formatter"
	"github.com/magiconair/properties"
)

type PropertiesFormatter struct{}

func NewPropertiesFormatter() *PropertiesFormatter {
	return &PropertiesFormatter{}
}

func (f *PropertiesFormatter) Format(input []byte, lang string, opts formatter.FormatOptions) ([]byte, error) {
	// 解析properties文件
	props, err := properties.Load(input, properties.UTF8)
	if err != nil {
		return nil, fmt.Errorf("properties load: %w", err)
	}

	// 按键排序输出
	keys := make([]string, 0, len(props.Keys()))
	for k := range props.Map() {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	for _, key := range keys {
		value, ok := props.Get(key)
		if !ok {
			continue
		}
		// 转义特殊字符（Java Properties规范）
		escapedKey := f.escapeKey(key)
		escapedValue := f.escapeValue(value)
		buf.WriteString(escapedKey)
		buf.WriteString("=")
		buf.WriteString(escapedValue)
		buf.WriteString("\n")
	}

	return buf.Bytes(), nil
}

// escapeKey 转义键中的特殊字符
func (f *PropertiesFormatter) escapeKey(key string) string {
	// Properties文件中键的转义规则
	key = strings.ReplaceAll(key, "\\", "\\\\")
	key = strings.ReplaceAll(key, "\n", "\\n")
	key = strings.ReplaceAll(key, "\r", "\\r")
	key = strings.ReplaceAll(key, "\t", "\\t")
	key = strings.ReplaceAll(key, " ", "\\ ")
	key = strings.ReplaceAll(key, "=", "\\=")
	key = strings.ReplaceAll(key, ":", "\\:")
	key = strings.ReplaceAll(key, "#", "\\#")
	key = strings.ReplaceAll(key, "!", "\\!")
	return key
}

// escapeValue 转义值中的特殊字符
func (f *PropertiesFormatter) escapeValue(value string) string {
	// Properties文件中值的转义规则
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\r", "\\r")
	value = strings.ReplaceAll(value, "\t", "\\t")
	return value
}

func (f *PropertiesFormatter) SupportedLanguages() []string {
	return []string{"properties"}
}

func (f *PropertiesFormatter) Name() string {
	return "native-properties"
}

func (f *PropertiesFormatter) Type() formatter.BackendType {
	return formatter.NativeGo
}