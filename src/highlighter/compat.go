package highlighter

import (
	"regexp"
	"strings"
)

// 兼容性 HTML 转换相关正则 (包级编译，避免每次调用重复编译)
var (
	// display:flex; 样式匹配
	flexStyleRe = regexp.MustCompile(`style="([^"]*?)display:\s*flex;?\s*([^"]*)"`)
	// 文本节点匹配 (> 和 < 之间的非标签内容)
	textNodeRe = regexp.MustCompile(`>([^<>]+)<`)
)

// CompatHTML 将 chroma HTML 输出转换为粘贴兼容格式。
//
// chroma 输出中每行包裹在 <span style="display:flex;"> 中，行尾用 \n 分隔。
// 不同粘贴目标 (OneNote, Quiver 等) 对空白和换行的处理不一致:
//   - OneNote: display:flex 触发块级渲染与 <br> 叠加产生多余空行;
//     空格/Tab 被折叠导致缩进丢失
//   - Quiver: 依赖 display:flex 换行，空行 (仅含 \n) 被折叠丢失;
//     空格/Tab 也被折叠
//   - 浏览器: <pre> 保留 \n，与 <br> 叠加产生双重换行
//
// 修复方案:
//  1. 移除 display:flex; 样式 (消除块级渲染的不确定性)
//  2. 文本节点中: 空格 → &nbsp;，Tab → 4 个 &nbsp;，\n → <br>
//     (&nbsp; 不可折叠，<br> 是所有粘贴目标都识别的强制换行)
func CompatHTML(html string) string {
	// 1. 移除 display:flex; 样式 (chroma 行容器的 flex 布局)
	html = flexStyleRe.ReplaceAllString(html, `style="$1$2"`)
	html = strings.ReplaceAll(html, ` style=""`, "")

	// 2. 统一处理文本节点: 空格/Tab → &nbsp;，换行 → <br>
	html = textNodeRe.ReplaceAllStringFunc(html, func(match string) string {
		// match 格式: ">text<"，去除首尾的 > 和 <
		text := match[1 : len(match)-1]
		text = strings.ReplaceAll(text, "\t", strings.Repeat("&nbsp;", 4))
		text = strings.ReplaceAll(text, " ", "&nbsp;")
		text = strings.ReplaceAll(text, "\n", "<br>")
		return ">" + text + "<"
	})

	return html
}
