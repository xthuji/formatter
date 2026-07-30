package native

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"

	"github.com/formatter/formatter/src/formatter"
)

// GoFormatter 使用 Go 标准库 go/format 实现代码格式化，
// 替代外部 gofmt 二进制 (节省 ~3M)。
//
// go/format 始终使用 Tab 缩进 (Go 规范)。当用户指定空格缩进 (TabWidth > 0) 时，
// 对输出进行后处理: 将行首 Tab 转换为对应数量的空格。
type GoFormatter struct{}

func NewGoFormatter() *GoFormatter {
	return &GoFormatter{}
}

func (f *GoFormatter) Format(input []byte, lang string, opts formatter.FormatOptions) ([]byte, error) {
	out, err := format.Source(input)
	if err != nil {
		return nil, fmt.Errorf("go/format: %w", err)
	}
	// go/format 固定输出 Tab 缩进; 仅当用户明确要求空格缩进时转换行首 Tab → 空格
	if opts.TabWidth > 0 {
		out = convertLeadingTabsToSpaces(out, opts.TabWidth)
	}
	return out, nil
}

// convertLeadingTabsToSpaces 将每行行首的 Tab 字符替换为指定数量的空格。
// 仅处理行首 Tab (缩进)，不影响字符串/注释内的 Tab。
func convertLeadingTabsToSpaces(src []byte, tabWidth int) []byte {
	spaces := strings.Repeat(" ", tabWidth)
	lines := bytes.Split(src, []byte("\n"))
	for i, line := range lines {
		// 统计行首 Tab 数量
		j := 0
		for j < len(line) && line[j] == '\t' {
			j++
		}
		if j > 0 {
			lines[i] = append([]byte(strings.Repeat(spaces, j)), line[j:]...)
		}
	}
	return bytes.Join(lines, []byte("\n"))
}

func (f *GoFormatter) SupportedLanguages() []string {
	return []string{"go"}
}

func (f *GoFormatter) Name() string {
	return "native-go"
}

func (f *GoFormatter) Type() formatter.BackendType {
	return formatter.NativeGo
}
