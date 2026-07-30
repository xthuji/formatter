// Package compressor 定义代码压缩接口
package compressor

// BackendType 后端类型
type BackendType int

const (
	NativeGo   BackendType = iota
	ToolBinary
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

// Compressor 压缩器接口
type Compressor interface {
	Compress(input []byte, lang string, opts CompressOptions) ([]byte, error)
	SupportedLanguages() []string
	Name() string
	Type() BackendType
}

// CompressOptions 压缩选项
type CompressOptions struct {
	RemoveWhitespace bool
	RemoveComments   bool
	Extra            map[string]interface{}
}
