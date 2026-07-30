package native

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/formatter/formatter/src/formatter"
	"gopkg.in/ini.v1"
)

type INIFormatter struct{}

func NewINIFormatter() *INIFormatter {
	return &INIFormatter{}
}

func (f *INIFormatter) Format(input []byte, lang string, opts formatter.FormatOptions) ([]byte, error) {
	// 加载INI文件
	cfg, err := ini.Load(input)
	if err != nil {
		return nil, fmt.Errorf("ini load: %w", err)
	}

	var buf bytes.Buffer

	// 先处理默认section（无section名的键值对）
	if defaultSection := cfg.Section(""); defaultSection != nil {
		keys := defaultSection.Keys()
		if len(keys) > 0 {
			sort.Slice(keys, func(i, j int) bool {
				return keys[i].Name() < keys[j].Name()
			})
			for _, key := range keys {
				buf.WriteString(key.Name())
				buf.WriteString(" = ")
				buf.WriteString(key.Value())
				buf.WriteString("\n")
			}
			buf.WriteString("\n")
		}
	}

	// 获取所有section（按名称排序）
	sections := cfg.Sections()
	sort.Slice(sections, func(i, j int) bool {
		return sections[i].Name() < sections[j].Name()
	})

	for _, section := range sections {
		// 跳过默认section
		if section.Name() == "" || section.Name() == "DEFAULT" {
			continue
		}

		// 写入section名
		buf.WriteString("[")
		buf.WriteString(section.Name())
		buf.WriteString("]\n")

		// 获取并排序该section的所有key
		keys := section.Keys()
		if len(keys) > 0 {
			sort.Slice(keys, func(i, j int) bool {
				return keys[i].Name() < keys[j].Name()
			})

			for _, key := range keys {
				buf.WriteString(key.Name())
				buf.WriteString(" = ")
				buf.WriteString(key.Value())
				buf.WriteString("\n")
			}
		}
		buf.WriteString("\n")
	}

	result := strings.TrimRight(buf.String(), "\n")
	return []byte(result), nil
}

func (f *INIFormatter) SupportedLanguages() []string {
	return []string{"ini"}
}

func (f *INIFormatter) Name() string {
	return "native-ini"
}

func (f *INIFormatter) Type() formatter.BackendType {
	return formatter.NativeGo
}