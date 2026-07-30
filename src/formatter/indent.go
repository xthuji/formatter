package formatter

import (
	"strings"
)

// 缩进配置常量
const (
	// TabIndent 表示使用 Tab 字符进行缩进 (tab_width 字段值为 -1)
	TabIndent = -1
	// DefaultTabWidth 当使用 Tab 缩进且需要数值宽度时 (如工具调用、缩进转换) 的默认宽度
	DefaultTabWidth = 4
)

// ConvertIndent 将文本的缩进从 from 格式转换到 to 格式。
// from: 工具输出的缩进格式
//   - TabWidth > 0: 空格缩进，值为每级空格数
//   - TabWidth == TabIndent (-1): Tab 缩进
//   - TabWidth == 0: 动态缩进 (工具支持通过 {tab_width} 占位符动态指定，输出已匹配目标宽度)
//
// to: 用户期望的缩进格式
//   - TabWidth > 0: 空格缩进，值为每级空格数
//   - TabWidth == TabIndent (-1): Tab 缩进
//
// 转换逻辑:
// 1. 检测每行开头的缩进字符 (空格/Tab)
// 2. 将 from 格式的缩进统一转换为缩进级别数
// 3. 将级别数转换为 to 格式的缩进字符
//
// 注意: 只转换行首缩进，不修改字符串字面量、注释等非缩进内容
func ConvertIndent(input string, from, to IndentSpec) string {
	// 如果格式相同，直接返回
	if from.TabWidth == to.TabWidth {
		return input
	}

	// 动态缩进 (from.TabWidth == 0): 工具输出已匹配有效目标宽度
	// 当目标也是空格时无需转换; 当目标为 Tab 时按 DefaultTabWidth 转换
	if from.TabWidth == 0 {
		if to.TabWidth == TabIndent {
			from = IndentSpec{TabWidth: DefaultTabWidth}
		} else {
			// 目标为空格，工具已输出正确宽度的空格，无需转换
			return input
		}
	}

	lines := strings.Split(input, "\n")
	result := make([]string, len(lines))

	for i, line := range lines {
		result[i] = convertLineIndent(line, from, to)
	}

	return strings.Join(result, "\n")
}

// IndentSpec 缩进规格
type IndentSpec struct {
	TabWidth int // -1=Tab 缩进, >0=空格缩进宽度, 0=动态 (仅用于 from)
}

// convertLineIndent 转换单行的缩进
func convertLineIndent(line string, from, to IndentSpec) string {
	// 计算行首缩进字符数
	spaceCount := 0
	tabCount := 0
	contentStart := 0

	for i, ch := range line {
		if ch == ' ' {
			spaceCount++
		} else if ch == '\t' {
			tabCount++
		} else {
			contentStart = i
			break
		}
	}

	// 没有缩进或空行，直接返回
	if spaceCount == 0 && tabCount == 0 {
		return line
	}

	// 计算源格式的有效宽度 (Tab 缩进时使用 DefaultTabWidth)
	fromWidth := from.TabWidth
	if fromWidth == TabIndent {
		fromWidth = DefaultTabWidth
	}

	// 计算缩进级别数
	var levels int
	if from.TabWidth == TabIndent {
		// 源格式使用 Tab，每个 Tab 算一级
		levels = tabCount
		if spaceCount > 0 {
			levels += spaceCount / fromWidth
		}
	} else {
		// 源格式使用空格，总空格数 / fromWidth = 级别数
		totalSpaces := spaceCount + tabCount*fromWidth
		levels = totalSpaces / fromWidth
	}

	if levels == 0 {
		levels = 1
	}

	// 生成目标格式的缩进
	var newIndent string
	if to.TabWidth == TabIndent {
		newIndent = strings.Repeat("\t", levels)
	} else {
		newIndent = strings.Repeat(" ", levels*to.TabWidth)
	}

	// 返回新缩进 + 原内容 (去除原缩进)
	return newIndent + line[contentStart:]
}
