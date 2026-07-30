package highlighter

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// ChromaHighlighter 基于 chroma 的代码高亮器。
// 语言 → chroma 词法分析器的映射由配置驱动：
//   - lexerOverrides 中的语言使用指定的 chroma lexer 名
//   - 未在 map 中的语言使用语言名本身作为 chroma lexer 名 (chroma v2 已将
//     所有主流语言名注册为 lexer 别名，如 "python"/"shell"/"javascript" 等)
//   - 若 chroma 仍无法找到 lexer，回退到 plaintext (纯文本高亮)
type ChromaHighlighter struct {
	lexerOverrides map[string]string
}

// NewChromaHighlighter 创建高亮器。
// lexerOverrides 为语言名 → chroma 词法分析器名的覆盖映射 (可为 nil)，
// 用于语言名与 chroma lexer 名不一致的场景 (如配置中可通过 highlighter.options.lexer 指定)。
func NewChromaHighlighter(lexerOverrides map[string]string) *ChromaHighlighter {
	return &ChromaHighlighter{lexerOverrides: lexerOverrides}
}

func (h *ChromaHighlighter) Highlight(input []byte, lang string, opts HighlightOptions) (string, error) {
	chromaLang := h.lexerName(lang)
	lexer := lexers.GlobalLexerRegistry.Get(chromaLang)
	if lexer == nil {
		lexer = lexers.Fallback
	}

	styleName := opts.Style
	if styleName == "" {
		styleName = "monokai"
	}
	style := styles.Get(styleName)
	// styles.Get 对未知主题名返回 styles.Fallback (Name="swapoff") 而非 nil，
	// 因此需要额外检查: 若请求的主题名非 "swapoff" 但返回了 Fallback，说明主题不存在，
	// 回退到 "monokai" 确保用户看到的是有效的高亮主题而非空白 swapoff。
	if style == nil || style.Name == "" || (style == styles.Fallback && styleName != "swapoff") {
		style = styles.Get("monokai")
	}

	outputType := opts.OutputType
	if outputType == "" {
		outputType = "html"
	}

	// HTML 输出统一使用 chromahtml.New 构建 formatter，确保 Standalone=false (内联样式)
	// 避免生成包含 <style> 块的独立 HTML 文档，与 App 内嵌 CSS 冲突。
	var fmtter chroma.Formatter
	if outputType == "html" {
		htmlOpts := []chromahtml.Option{chromahtml.Standalone(false)}
		if opts.LineNumbers {
			htmlOpts = append(htmlOpts, chromahtml.WithLineNumbers(true))
		}
		fmtter = chromahtml.New(htmlOpts...)
	} else {
		fmtter = formatters.Get(outputType)
	}

	it, err := lexer.Tokenise(nil, string(input))
	if err != nil {
		return "", fmt.Errorf("chroma tokenise: %w", err)
	}

	var buf bytes.Buffer
	if err := fmtter.Format(&buf, style, it); err != nil {
		return "", fmt.Errorf("chroma format: %w", err)
	}
	result := buf.String()
	// HTML 输出时注入字体大小到 <pre> 标签的 style 属性
	if outputType == "html" && opts.FontSize > 0 {
		result = injectFontSize(result, opts.FontSize)
	}
	// 兼容性 HTML 输出: 移除 display:flex + 空格保护 + <br> 换行
	if outputType == "html" && opts.CompatHTML {
		result = CompatHTML(result)
	}
	return result, nil
}

// injectFontSize 将 font-size:Npx; 注入到 <pre style="..."> 标签的行首样式位置。
// chroma 输出格式: <pre style="color:...;background-color:...;">
func injectFontSize(html string, fontSize int) string {
	preStyle := `<pre style="`
	idx := strings.Index(html, preStyle)
	if idx < 0 {
		return html
	}
	inject := `font-size:` + strconv.Itoa(fontSize) + `px;`
	// 在 <pre style=" 后插入 font-size，确保它作为首个样式属性
	return html[:idx+len(preStyle)] + inject + html[idx+len(preStyle):]
}

// lexerName 返回语言对应的 chroma 词法分析器名。
// 优先使用配置覆盖，未覆盖则使用语言名本身。
func (h *ChromaHighlighter) lexerName(lang string) string {
	if name, ok := h.lexerOverrides[lang]; ok && name != "" {
		return name
	}
	return lang
}

// SupportedLanguages 返回有显式 lexer 覆盖的语言列表。
// 高亮器实际支持的语言范围由配置决定 (所有配置的语言均注册高亮器)，
// 此方法仅返回有自定义 lexer 映射的语言子集。
func (h *ChromaHighlighter) SupportedLanguages() []string {
	langs := make([]string, 0, len(h.lexerOverrides))
	for lang := range h.lexerOverrides {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	return langs
}

func (h *ChromaHighlighter) Name() string {
	return "chroma"
}
