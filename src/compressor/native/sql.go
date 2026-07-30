package native

import (
	"fmt"
	"strings"

	gosqlx "github.com/ajitpratap0/GoSQLX/pkg/gosqlx"
	sqlfmt "github.com/ajitpratap0/GoSQLX/pkg/formatter"
	astfmt "github.com/ajitpratap0/GoSQLX/pkg/sql/ast"
	"github.com/formatter/formatter/src/compressor"
)

// SQLCompressor 使用 GoSQLX 的 CompactStyle 实现 SQL 压缩。
//
// 通过 GoSQLX 解析 SQL 为 AST，再以紧凑风格 (无缩进、无换行、保留关键字大小写)
// 重新渲染，移除所有注释与多余空白，缩减 SQL 体积。
//
// 若 GoSQLX 解析失败 (如含有方言特有语法)，回退到纯文本压缩:
// 移除注释、折叠空白、去除运算符/标点周围的冗余空格。
type SQLCompressor struct{}

func NewSQLCompressor() *SQLCompressor {
	return &SQLCompressor{}
}

func (c *SQLCompressor) Compress(input []byte, lang string, opts compressor.CompressOptions) ([]byte, error) {
	src := strings.TrimSpace(string(input))
	if src == "" {
		return nil, fmt.Errorf("sql compress: empty input")
	}

	// 优先: 使用 GoSQLX CompactStyle 压缩
	if result, ok := compressWithGoSQLX(src); ok {
		return []byte(result), nil
	}

	// 回退: 纯文本压缩 (解析失败时)
	result := compressPlainText(src)
	if result == "" {
		return nil, fmt.Errorf("sql compress: empty result after processing")
	}
	return []byte(result), nil
}

// compressWithGoSQLX 使用 GoSQLX 解析 + CompactStyle 渲染压缩 SQL。
// 返回 (结果, 是否成功)。
func compressWithGoSQLX(src string) (string, bool) {
	parsedAST, err := gosqlx.Parse(src)
	if err != nil {
		return "", false
	}
	// CompactStyle: 无缩进、无换行、保留关键字大小写
	opts := astfmt.CompactStyle()
	result := sqlfmt.FormatAST(parsedAST, opts)
	result = strings.TrimSpace(result)
	return result, result != ""
}

// compressPlainText 在 GoSQLX 解析失败时使用的纯文本压缩。
// 移除注释、折叠空白、去除冗余空格。
func compressPlainText(src string) string {
	var sb strings.Builder
	sb.Grow(len(src))

	i := 0
	n := len(src)
	prevChar := byte(0)
	pendingSpace := false

	writeChar := func(c byte) {
		if pendingSpace && prevChar != 0 {
			if sqlNeedsSpace(prevChar, c) {
				sb.WriteByte(' ')
			}
		}
		pendingSpace = false
		sb.WriteByte(c)
		prevChar = c
	}

	for i < n {
		ch := src[i]

		// 字符串字面量: 原样输出
		if ch == '\'' || ch == '"' || ch == '`' {
			quote := ch
			writeChar(ch)
			i++
			for i < n {
				c2 := src[i]
				sb.WriteByte(c2)
				prevChar = c2
				i++
				if c2 == '\\' && i < n {
					sb.WriteByte(src[i])
					prevChar = src[i]
					i++
					continue
				}
				if c2 == quote {
					if quote == '\'' && i < n && src[i] == '\'' {
						sb.WriteByte(src[i])
						prevChar = src[i]
						i++
						continue
					}
					break
				}
			}
			continue
		}

		// 单行注释
		if ch == '-' && i+1 < n && src[i+1] == '-' {
			i += 2
			for i < n && src[i] != '\n' {
				i++
			}
			continue
		}

		// 多行注释
		if ch == '/' && i+1 < n && src[i+1] == '*' {
			i += 2
			for i+1 < n {
				if src[i] == '*' && src[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			if i >= n {
				i = n
			}
			continue
		}

		// 空白
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			pendingSpace = true
			i++
			for i < n && (src[i] == ' ' || src[i] == '\t' || src[i] == '\n' || src[i] == '\r') {
				i++
			}
			continue
		}

		writeChar(ch)
		i++
	}

	return strings.TrimSpace(sb.String())
}

// sqlNeedsSpace 判断纯文本压缩时两个字符间是否需要保留空格
func sqlNeedsSpace(prev, curr byte) bool {
	isWord := func(c byte) bool {
		return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
	}
	isQuote := func(c byte) bool {
		return c == '\'' || c == '"' || c == '`'
	}
	if isWord(prev) && isWord(curr) {
		return true
	}
	if prev == ')' && isWord(curr) {
		return true
	}
	if isWord(prev) && isQuote(curr) {
		return true
	}
	if isQuote(prev) && isWord(curr) {
		return true
	}
	if prev == ')' && isQuote(curr) {
		return true
	}
	return false
}

func (c *SQLCompressor) SupportedLanguages() []string {
	return []string{"sql"}
}

func (c *SQLCompressor) Name() string {
	return "native-sql"
}

func (c *SQLCompressor) Type() compressor.BackendType {
	return compressor.NativeGo
}
