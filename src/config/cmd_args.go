package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ExpandCmdArgs 将命令参数模板中的占位符替换为实际值。
// 支持的占位符：
//   {tab_width}        - 缩进宽度 (tab_width=-1 时展开为默认宽度 4)
//   {use_tabs}         - "true" 或 "false" (tab_width=-1 时为 "true")
//   {indent_style}     - "space" 或 "tab" (tab_width=-1 时为 "tab")
//   {line_ending}      - 行结束符 (\n / \r\n)
//   {ext}              - 语言对应的文件扩展名 (js/ts/css/html/json/yaml)
//   {lang}             - 语言名称
//   {dialect}          - SQL 方言 (默认 ansi，可通过 options.dialect 覆盖)
//   {edition}          - Rust edition (默认 2021，可通过 options.edition 覆盖)
//   {line_length}      - Python 行长度 (默认 88，可通过 options.line_length 覆盖)
//   {options.xxx}      - 自定义 options 字段
func (tc *ToolConfig) ExpandCmdArgs(cfg *Config, lang string) []string {
	if len(tc.Cmd) == 0 {
		return nil
	}

	vars := buildTemplateVars(cfg, tc, lang)

	result := make([]string, 0, len(tc.Cmd))
	for _, arg := range tc.Cmd {
		result = append(result, expandTemplate(arg, vars))
	}
	return result
}

func buildTemplateVars(cfg *Config, tc *ToolConfig, lang string) map[string]string {
	// 解析缩进配置：优先使用 format 级别 (由 pipeline opts 传入)，
	// format 级别 tab_width=0 (未显式设置) 时回退到语言级别 indent
	tabWidth := cfg.Format.TabWidth
	if tabWidth == 0 {
		if langCfg, ok := cfg.Languages[lang]; ok && langCfg.Indent != nil {
			tabWidth = langCfg.Indent.TabWidth
		}
	}

	// 派生 useTabs 和 indentStyle (tab_width=-1 表示 Tab 缩进)
	useTabs := tabWidth == -1
	indentStyle := "space"
	if useTabs {
		indentStyle = "tab"
	}

	// {tab_width} 占位符: Tab 缩进 (-1) 时展开为默认宽度 4 (供工具使用)
	// 工具输出的空格缩进后续由 ConvertIndent 转换为 Tab
	effectiveTabWidth := tabWidth
	if effectiveTabWidth == -1 {
		effectiveTabWidth = 4 // formatter.DefaultTabWidth
	}

	vars := map[string]string{
		"tab_width":    strconv.Itoa(effectiveTabWidth),
		"use_tabs":     strconv.FormatBool(useTabs),
		"indent_style": indentStyle,
		"line_ending":  cfg.Format.LineEnding,
		"ext":          langExtension(cfg, lang),
		"lang":         lang,
		"dialect":      "ansi",
		"edition":      "2021",
		"line_length":  "88",
	}

	// 合并 options 中的自定义变量
	for k, v := range tc.Options {
		vars["options."+k] = fmt.Sprintf("%v", v)
		// 同时支持直接引用 (不带 options. 前缀)
		if _, exists := vars[k]; !exists {
			vars[k] = fmt.Sprintf("%v", v)
		}
	}

	return vars
}

// expandTemplate 替换字符串中的 {var} 占位符
func expandTemplate(s string, vars map[string]string) string {
	for k, v := range vars {
		placeholder := "{" + k + "}"
		s = strings.ReplaceAll(s, placeholder, v)
	}
	return s
}

// langExtension 返回语言对应的文件扩展名 (无点前缀)。
// 从配置的 detection.extensions 首个扩展名派生，未配置时回退到 "txt"。
func langExtension(cfg *Config, lang string) string {
	if lc, ok := cfg.Languages[lang]; ok && lc.Detection != nil && len(lc.Detection.Extensions) > 0 {
		return strings.TrimPrefix(lc.Detection.Extensions[0], ".")
	}
	return "txt"
}
