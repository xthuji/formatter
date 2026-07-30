// Package highlighter 定义代码高亮接口
package highlighter

// Highlighter 高亮器接口
type Highlighter interface {
	Highlight(input []byte, lang string, opts HighlightOptions) (string, error)
	SupportedLanguages() []string
	Name() string
}

// HighlightOptions 高亮选项
type HighlightOptions struct {
	Style       string
	OutputType  string
	FontSize    int  // 字体大小 (px), 0=不设置
	LineNumbers bool
	CompatHTML  bool // 兼容性HTML输出 (内联样式+空格保护+<br>换行)
}
