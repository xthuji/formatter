// Package formatter 定义代码格式化接口
package formatter

// BackendType 后端类型
type BackendType int

const (
	NativeGo   BackendType = iota // Go 原生实现
	ToolBinary                    // 工具二进制
)

func (b BackendType) String() string {
	switch b {
	case NativeGo:
		return "native"
	case ToolBinary:
		return "tool"
	default:
		return "unknown"
	}
}

// Formatter 格式化器接口
type Formatter interface {
	Format(input []byte, lang string, opts FormatOptions) ([]byte, error)
	SupportedLanguages() []string
	Name() string
	Type() BackendType
}

// FormatOptions 格式化选项
type FormatOptions struct {
	TabWidth   int // -1=Tab 缩进, >0=空格缩进宽度
	SortImport bool
	LineEnding string
	Extra      map[string]interface{}
}
