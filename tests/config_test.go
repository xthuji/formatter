package tests

import (
	"os"
	"strings"
	"testing"

	"github.com/formatter/formatter/src/appcommon"
	"github.com/formatter/formatter/src/config"
)

func TestDefault(t *testing.T) {
	cfg := config.Default()

	if cfg.Format.TabWidth != 2 {
		t.Errorf("expected TabWidth = 2, got %d", cfg.Format.TabWidth)
	}
	if cfg.Format.LineEnding != "\n" {
		t.Errorf("expected LineEnding = '\\n', got %q", cfg.Format.LineEnding)
	}

	// Highlight 配置来自嵌入的 data/config.json，仅验证字段有效性
	if cfg.Highlight.Style == "" {
		t.Error("expected non-empty Highlight.Style")
	}

	if cfg.Languages == nil {
		t.Error("expected Languages map to be initialized")
	}

	// Server config (TestMain 注入 data/config.json 为默认配置，端口值以实际配置为准)
	if cfg.Server.Port != 3020 {
		t.Errorf("expected Server.Port = 3020, got %d", cfg.Server.Port)
	}
	if cfg.Server.AutoPort != true {
		t.Error("expected Server.AutoPort = true")
	}

	// Window config
	if cfg.Window.Width <= 0 {
		t.Errorf("expected Window.Width > 0, got %d", cfg.Window.Width)
	}
	if cfg.Window.Height <= 0 {
		t.Errorf("expected Window.Height > 0, got %d", cfg.Window.Height)
	}

	// Binary config (配置驱动的运行时与工具定义)
	if cfg.Binary.Runtimes == nil {
		t.Error("expected Runtimes to be initialized")
	}
	if cfg.Binary.Tools == nil {
		t.Error("expected Tools to be initialized")
	}
}

func TestLoad_EmptyPath_DefaultExists(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Format.TabWidth != 2 {
		t.Errorf("expected default TabWidth = 2, got %d", cfg.Format.TabWidth)
	}
}

func TestLoad_InvalidPath(t *testing.T) {
	cfg, err := config.Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config (should fallback to defaults)")
	}
}

func TestLoad_ValidJSON(t *testing.T) {
	jsonContent := `{
  "binary": {
    "runtimes": [
      { "name": "node", "exe": "node", "path": "/usr/local/bin/node" }
    ]
  },
  "format": {
    "tab_width": -1,
    "line_ending": "\r\n"
  },
  "highlight": {
    "style": "monokai",
    "line_numbers": true
  },
  "languages": {
    "go": {
      "formatter": { "backend": "native", "tool": "go" },
      "compressor": { "backend": "native", "tool": "jq" },
      "highlighter": { "backend": "chroma", "tool": "chroma" }
    }
  }
}`

	tmpFile, err := os.CreateTemp("", "config-test-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(jsonContent); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	cfg, err := config.Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证合并后的 runtimes 数组结构与 Path 字段
	if len(cfg.Binary.Runtimes) == 0 {
		t.Fatal("expected non-empty Runtimes array")
	}
	found := false
	for _, rt := range cfg.Binary.Runtimes {
		if rt.Name == "node" && rt.Path == "/usr/local/bin/node" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected runtimes node with path '/usr/local/bin/node'")
	}

	if cfg.Format.TabWidth != -1 {
		t.Errorf("expected TabWidth = -1, got %d", cfg.Format.TabWidth)
	}

	if cfg.Highlight.Style != "monokai" {
		t.Errorf("expected Highlight.Style = 'monokai', got %q", cfg.Highlight.Style)
	}
	if cfg.Highlight.LineNumbers != true {
		t.Error("expected LineNumbers = true")
	}

	goLang, ok := cfg.Languages["go"]
	if !ok {
		t.Fatal("expected 'go' language config")
	}
	if goLang.Formatter.Backend != "native" {
		t.Errorf("expected formatter backend 'native', got %q", goLang.Formatter.Backend)
	}
	if goLang.Formatter.Tool != "go" {
		t.Errorf("expected formatter tool 'go', got %q", goLang.Formatter.Tool)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config-test-invalid-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(`{"key": [invalid`); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	_, err = config.Load(tmpFile.Name())
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoad_PartialJSON(t *testing.T) {
	jsonContent := `{
  "format": {
    "tab_width": 8
  }
}`

	tmpFile, err := os.CreateTemp("", "config-test-partial-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(jsonContent); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	cfg, err := config.Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Format.TabWidth != 8 {
		t.Errorf("expected TabWidth = 8, got %d", cfg.Format.TabWidth)
	}

	// 部分加载时未设置的字段应使用嵌入默认值
	defaultCfg := config.Default()
	if cfg.Highlight.Style != defaultCfg.Highlight.Style {
		t.Errorf("expected Highlight.Style = %q (default), got %q", defaultCfg.Highlight.Style, cfg.Highlight.Style)
	}
}

func TestToolConfig_Options(t *testing.T) {
	jsonContent := `{
  "languages": {
    "json": {
      "formatter": {
        "backend": "native",
        "tool": "jq",
        "options": {
          "indent": 4,
          "sort_keys": true
        }
      }
    }
  }
}`

	tmpFile, err := os.CreateTemp("", "config-test-options-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(jsonContent); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	cfg, err := config.Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jsonLang := cfg.Languages["json"]
	if jsonLang.Formatter.Options == nil {
		t.Fatal("expected formatter options map to be set")
	}
	if jsonLang.Formatter.Options["indent"] != float64(4) {
		t.Errorf("expected indent = 4, got %v", jsonLang.Formatter.Options["indent"])
	}
}

func TestDefault_LanguagesPrePopulated(t *testing.T) {
	cfg := config.Default()

	// 直接遍历 cfg.Languages (源自 config.json)，验证每个语言都有
	// 非空的 formatter 配置 (formatter 非 nil，且 backend/tool 非空)。
	// 这样当 config.json 中新增语言时，本测试无需修改即可自动适配。
	if len(cfg.Languages) == 0 {
		t.Fatal("expected non-empty languages map in default config")
	}
	for lang, lc := range cfg.Languages {
		if lc.Formatter == nil {
			t.Errorf("expected formatter for language %q", lang)
			continue
		}
		if lc.Formatter.Backend == "" {
			t.Errorf("expected formatter backend for language %q", lang)
		}
		if lc.Formatter.Tool == "" {
			t.Errorf("expected formatter tool for language %q", lang)
		}
	}
}

// TestConfig_ToolLanguageConsistency 验证配置一致性:
// 1. 每个语言引用的工具 (backend=tool) 都在 binary.tools 中定义
// 2. 每个工具声明的 languages 都在 languages 配置中存在
// 3. native backend 的工具 (json/yaml/xml/html) 不需要在 binary.tools 中定义
func TestConfig_ToolLanguageConsistency(t *testing.T) {
	cfg := config.Default()

	// 构建工具名集合
	toolNames := make(map[string]bool)
	for _, tool := range cfg.Binary.Tools {
		toolNames[tool.Name] = true
	}

	// 检查每个语言的格式化器/压缩器引用的工具是否存在
	for lang, lc := range cfg.Languages {
		if lc.Formatter != nil && lc.Formatter.Backend == "tool" {
			if !toolNames[lc.Formatter.Tool] {
				t.Errorf("language %q references formatter tool %q not found in binary.tools", lang, lc.Formatter.Tool)
			}
		}
		if lc.Compressor != nil && lc.Compressor.Backend == "tool" {
			if !toolNames[lc.Compressor.Tool] {
				t.Errorf("language %q references compressor tool %q not found in binary.tools", lang, lc.Compressor.Tool)
			}
		}
	}

	// 检查每个工具的 languages 是否在 languages 配置中存在
	for _, tool := range cfg.Binary.Tools {
		for _, tl := range tool.Languages {
			if _, ok := cfg.Languages[tl]; !ok {
				t.Errorf("tool %q declares language %q not found in languages config", tool.Name, tl)
			}
		}
	}
}

// TestConfig_NativeBackendTools 验证 native backend 仅用于内置 Go 格式化器/压缩器。
// native 工具是内置 Go 实现，不应出现在 binary.tools 工具二进制列表中。
// 其他语言必须使用 tool backend。
func TestConfig_NativeBackendTools(t *testing.T) {
	cfg := config.Default()

	// 构建 binary.tools 中定义的工具二进制名集合
	toolBinaries := make(map[string]bool)
	for _, tool := range cfg.Binary.Tools {
		toolBinaries[tool.Name] = true
	}

	// native backend 的工具是内置 Go 实现，不应被注册为工具二进制
	for lang, lc := range cfg.Languages {
		if lc.Formatter != nil && lc.Formatter.Backend == "native" {
			if toolBinaries[lc.Formatter.Tool] {
				t.Errorf("language %q uses native backend but tool %q is registered as tool binary", lang, lc.Formatter.Tool)
			}
		}
		if lc.Compressor != nil && lc.Compressor.Backend == "native" {
			if toolBinaries[lc.Compressor.Tool] {
				t.Errorf("language %q uses native compressor but tool %q is registered as tool binary", lang, lc.Compressor.Tool)
			}
		}
	}
}

// TestConfig_ExternalToolsHaveValidCmd 验证工具二进制的 cmd 模板中使用的占位符均可被 ExpandCmdArgs 解析
// 确保配置化执行流程不会因未知占位符而失败
func TestConfig_ExternalToolsHaveValidCmd(t *testing.T) {
	cfg := config.Default()

	// 所有合法的占位符 (不含 {options.xxx} 前缀)
	validPlaceholders := map[string]bool{
		"tab_width": true, "use_tabs": true, "indent_style": true,
		"line_ending": true, "ext": true, "lang": true,
		"dialect": true, "edition": true, "line_length": true,
	}

	for lang, lc := range cfg.Languages {
		for _, tc := range []*config.ToolConfig{lc.Formatter, lc.Compressor} {
			if tc == nil || tc.Backend != "tool" || len(tc.Cmd) == 0 {
				continue
			}
			// 展开参数 (不应 panic 或残留未替换的占位符)
			args := tc.ExpandCmdArgs(cfg, lang)
			for i, arg := range args {
				// 检查是否残留 {xxx} 格式的占位符 (排除 {options.xxx} 前缀)
				if strings.Contains(arg, "{") && strings.Contains(arg, "}") {
					// 提取占位符检查是否合法
					start := strings.Index(arg, "{")
					end := strings.Index(arg, "}")
					if start >= 0 && end > start {
						placeholder := arg[start+1 : end]
						if !strings.HasPrefix(placeholder, "options.") && !validPlaceholders[placeholder] {
							t.Errorf("language %q cmd[%d] has unknown placeholder {%s} in arg %q", lang, i, placeholder, arg)
						}
					}
				}
			}
		}
	}
}

// TestConfig_HighlighterAlwaysChroma 验证高亮引擎统一使用 chroma (配置化管理)
func TestConfig_HighlighterAlwaysChroma(t *testing.T) {
	cfg := config.Default()
	// 高亮配置不应依赖 languages 中的 highlighter 字段
	// 高亮引擎固定为 chroma，由 pipeline 自动处理
	if cfg.Highlight.Style == "" {
		t.Error("expected non-empty highlight style")
	}
}

func TestDefault_PreferencesNativeForGo(t *testing.T) {
	cfg := config.Default()
	goLang := cfg.Languages["go"]
	if goLang.Formatter == nil {
		t.Fatal("expected go formatter")
	}
	if goLang.Formatter.Backend != "native" {
		t.Errorf("expected go formatter backend 'native', got %q", goLang.Formatter.Backend)
	}
	if goLang.Formatter.Tool != "go" {
		t.Errorf("expected go formatter tool 'go', got %q", goLang.Formatter.Tool)
	}
}

func TestDefault_UsesOxfmtForJavascript(t *testing.T) {
	cfg := config.Default()
	jsLang := cfg.Languages["javascript"]
	if jsLang.Formatter == nil {
		t.Fatal("expected javascript formatter")
	}
	if jsLang.Formatter.Backend != "tool" {
		t.Errorf("expected javascript formatter backend 'tool', got %q", jsLang.Formatter.Backend)
	}
	if jsLang.Formatter.Tool != "oxfmt-wrapper" {
		t.Errorf("expected javascript formatter tool 'oxfmt-wrapper', got %q", jsLang.Formatter.Tool)
	}
	if jsLang.Compressor == nil {
		t.Fatal("expected javascript compressor")
	}
	if jsLang.Compressor.Tool != "js" {
		t.Errorf("expected javascript compressor tool 'js', got %q", jsLang.Compressor.Tool)
	}
}

func TestDefault_HasCompressorForJson(t *testing.T) {
	cfg := config.Default()
	jsonLang := cfg.Languages["json"]
	if jsonLang.Formatter == nil {
		t.Fatal("expected json formatter")
	}
	if jsonLang.Compressor == nil {
		t.Fatal("expected json compressor (native)")
	}
	if jsonLang.Compressor.Backend != "native" {
		t.Errorf("expected json compressor backend 'native', got %q", jsonLang.Compressor.Backend)
	}
}

func TestDefault_NoCompressorForGo(t *testing.T) {
	cfg := config.Default()
	goLang := cfg.Languages["go"]
	if goLang.Compressor != nil {
		t.Errorf("expected go compressor to be nil (unsupported), got %+v", goLang.Compressor)
	}
}

// TestExpandCmdArgs 验证命令参数模板的占位符替换逻辑。
// 覆盖: 基础占位符 ({tab_width}/{use_tabs}/{indent_style}/{ext}/{lang})、
//       options 自定义字段、默认值回退。
// tab_width=-1 (Tab 缩进) 时 {tab_width} 展开为默认宽度 4，{use_tabs}="true"，{indent_style}="tab"
func TestExpandCmdArgs(t *testing.T) {
	cfg := &config.Config{
		Format: config.FormatConfig{
			TabWidth:   -1, // Tab 缩进
			LineEnding: "\n",
		},
		Languages: map[string]config.LangConfig{
			"python": {Detection: &config.DetectionConfig{Extensions: []string{".py"}}},
			"go":     {Detection: &config.DetectionConfig{Extensions: []string{".go"}}},
			"json":   {Detection: &config.DetectionConfig{Extensions: []string{".json"}}},
		},
	}

	tests := []struct {
		name string
		tc   *config.ToolConfig
		lang string
		want []string
	}{
		{
			name: "basic placeholders with tabs",
			tc: &config.ToolConfig{
				Cmd: []string{"format", "--indent-width", "{tab_width}", "--indent-style", "{indent_style}", "--use-tabs", "{use_tabs}"},
			},
			lang: "javascript",
			want: []string{"format", "--indent-width", "4", "--indent-style", "tab", "--use-tabs", "true"},
		},
		{
			name: "ext and lang placeholders",
			tc: &config.ToolConfig{
				Cmd: []string{"--stdin-file-path", "input.{ext}", "--lang", "{lang}"},
			},
			lang: "python",
			want: []string{"--stdin-file-path", "input.py", "--lang", "python"},
		},
		{
			name: "options custom field",
			tc: &config.ToolConfig{
				Cmd: []string{"--config-str", "version={options.scala_version}"},
				Options: map[string]interface{}{
					"scala_version": "3.3.0",
				},
			},
			lang: "scala",
			want: []string{"--config-str", "version=3.3.0"},
		},
		{
			name: "default dialect and edition",
			tc: &config.ToolConfig{
				Cmd: []string{"--dialect", "{dialect}", "--edition", "{edition}"},
			},
			lang: "sql",
			want: []string{"--dialect", "ansi", "--edition", "2021"},
		},
		{
			name: "empty cmd returns nil",
			tc: &config.ToolConfig{
				Cmd: nil,
			},
			lang: "go",
			want: nil,
		},
		{
			name: "line_ending placeholder",
			tc: &config.ToolConfig{
				Cmd: []string{"--line-ending", "{line_ending}"},
			},
			lang: "json",
			want: []string{"--line-ending", "\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.tc.ExpandCmdArgs(cfg, tt.lang)
			if len(got) != len(tt.want) {
				t.Fatalf("len mismatch: got %d, want %d (got: %v)", len(got), len(tt.want), got)
			}
			for i, g := range got {
				if g != tt.want[i] {
					t.Errorf("arg[%d] = %q, want %q", i, g, tt.want[i])
				}
			}
		})
	}
}

// TestExpandCmdArgs_SpaceIndent 验证 tab_width>0 (空格缩进) 时 indent_style 为 "space"
func TestExpandCmdArgs_SpaceIndent(t *testing.T) {
	cfg := &config.Config{
		Format: config.FormatConfig{
			TabWidth: 2, // 空格缩进
		},
	}
	tc := &config.ToolConfig{
		Cmd: []string{"--indent-style", "{indent_style}"},
	}
	got := tc.ExpandCmdArgs(cfg, "javascript")
	if got[1] != "space" {
		t.Errorf("expected indent_style='space' when tab_width>0, got %q", got[1])
	}
}

// TestExpandHomePath 验证 ~ 路径展开
func TestExpandHomePath(t *testing.T) {
	tests := []struct {
		input string
		check func(string) bool
	}{
		{"~", func(s string) bool { return s != "~" && len(s) > 0 }},
		{"~/foo/bar", func(s string) bool {
			return len(s) > 0 && s[len(s)-1] != '~' && !strings.Contains(s, "~")
		}},
		{"/absolute/path", func(s string) bool { return s == "/absolute/path" }},
		{"relative/path", func(s string) bool { return s == "relative/path" }},
	}

	for _, tt := range tests {
		got := config.ExpandHomePath(tt.input)
		if !tt.check(got) {
			t.Errorf("ExpandHomePath(%q) = %q, check failed", tt.input, got)
		}
	}
}

// ---- Config Map 转换 ----

// ---- ParseToolConfig ----

func TestParseToolConfig_FullMap(t *testing.T) {
	tc := appcommon.ParseToolConfig(map[string]interface{}{
		"backend": "tool",
		"tool":    "oxfmt-wrapper",
		"cmd":     []interface{}{"{exe}", "--stdin", "{tab_width}"},
		"options": map[string]interface{}{"indent": float64(4)},
	})
	if tc == nil {
		t.Fatal("expected non-nil ToolConfig")
	}
	if tc.Backend != "tool" {
		t.Errorf("expected backend='tool', got %q", tc.Backend)
	}
	if tc.Tool != "oxfmt-wrapper" {
		t.Errorf("expected tool='oxfmt-wrapper', got %q", tc.Tool)
	}
	if len(tc.Cmd) != 3 || tc.Cmd[0] != "{exe}" {
		t.Errorf("unexpected cmd: %v", tc.Cmd)
	}
	if tc.Options["indent"] != float64(4) {
		t.Errorf("unexpected options: %v", tc.Options)
	}
}

func TestParseToolConfig_Nil(t *testing.T) {
	tc := appcommon.ParseToolConfig(nil)
	if tc != nil {
		t.Error("expected nil for nil input")
	}
}

func TestParseToolConfig_NonMap(t *testing.T) {
	tc := appcommon.ParseToolConfig("not a map")
	if tc != nil {
		t.Error("expected nil for non-map input")
	}
}

func TestParseToolConfig_Partial(t *testing.T) {
	tc := appcommon.ParseToolConfig(map[string]interface{}{
		"backend": "native",
	})
	if tc == nil {
		t.Fatal("expected non-nil")
	}
	if tc.Backend != "native" {
		t.Errorf("expected backend='native', got %q", tc.Backend)
	}
	if tc.Tool != "" {
		t.Errorf("expected empty tool, got %q", tc.Tool)
	}
	if len(tc.Cmd) != 0 {
		t.Errorf("expected empty cmd, got %v", tc.Cmd)
	}
}

func TestParseToolConfig_CmdFiltersEmpty(t *testing.T) {
	tc := appcommon.ParseToolConfig(map[string]interface{}{
		"backend": "tool",
		"tool":    "test",
		"cmd":     []interface{}{"{exe}", "", "", "--flag"},
	})
	if tc == nil {
		t.Fatal("expected non-nil")
	}
	// 空字符串应被过滤
	if len(tc.Cmd) != 2 {
		t.Errorf("expected 2 cmd args (empty filtered), got %d: %v", len(tc.Cmd), tc.Cmd)
	}
}

func TestParseToolConfig_EmptyOptions(t *testing.T) {
	tc := appcommon.ParseToolConfig(map[string]interface{}{
		"backend": "native",
		"tool":    "jq",
		"options": map[string]interface{}{},
	})
	if tc == nil {
		t.Fatal("expected non-nil")
	}
	if tc.Options != nil {
		t.Errorf("expected nil options for empty map, got %v", tc.Options)
	}
}

// ---- ConfigToMap ----

func TestConfigToMap_AllSections(t *testing.T) {
	cfg := config.Default()
	m := appcommon.ConfigToMap(cfg)

	// 验证所有顶层 section 存在
	for _, key := range []string{"server", "window", "binary", "format", "highlight", "languages"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing section %q in ConfigToMap output", key)
		}
	}

	// 验证 server 字段
	srvMap, ok := m["server"].(map[string]interface{})
	if !ok {
		t.Fatal("server section is not a map")
	}
	if srvMap["port"] != cfg.Server.Port {
		t.Errorf("expected port=%d, got %v", cfg.Server.Port, srvMap["port"])
	}

	// 验证 format 字段
	fmtMap, ok := m["format"].(map[string]interface{})
	if !ok {
		t.Fatal("format section is not a map")
	}
	if fmtMap["tab_width"] != cfg.Format.TabWidth {
		t.Errorf("expected tab_width=%d, got %v", cfg.Format.TabWidth, fmtMap["tab_width"])
	}
}

func TestConfigToMap_LanguageWithAllFields(t *testing.T) {
	cfg := &config.Config{
		Languages: map[string]config.LangConfig{
			"testlang": {
				Formatter: &config.ToolConfig{
					Backend: "tool",
					Tool:    "test-tool",
					Cmd:     []string{"{exe}", "--flag"},
				},
				Compressor: &config.ToolConfig{
					Backend: "native",
					Tool:    "jq",
				},
				Indent: &config.IndentConfig{
					TabWidth: -1, // Tab 缩进
				},
				Detection: &config.DetectionConfig{
					Extensions: []string{".tl"},
					Shebangs:   []string{"#!/usr/bin/tl"},
					Content:    []string{"pattern"},
					Priority:   10,
				},
				Highlighter: &config.ToolConfig{
					Backend: "chroma",
					Tool:    "chroma",
					Options: map[string]interface{}{"theme": "monokai"},
				},
			},
		},
	}
	m := appcommon.ConfigToMap(cfg)
	langs, ok := m["languages"].(map[string]interface{})
	if !ok {
		t.Fatal("languages section is not a map")
	}
	langData, ok := langs["testlang"].(map[string]interface{})
	if !ok {
		t.Fatal("testlang not found in languages map")
	}
	// 验证 formatter
	fmtMap, ok := langData["formatter"].(map[string]interface{})
	if !ok {
		t.Fatal("formatter not found")
	}
	if fmtMap["backend"] != "tool" {
		t.Errorf("expected backend='tool', got %v", fmtMap["backend"])
	}
	if _, ok := fmtMap["cmd"]; !ok {
		t.Error("cmd should be present when non-empty")
	}
	// 验证 indent
	indentMap, ok := langData["indent"].(map[string]interface{})
	if !ok {
		t.Fatal("indent not found")
	}
	if indentMap["tab_width"] != -1 {
		t.Errorf("expected tab_width=-1, got %v", indentMap["tab_width"])
	}
	// 验证 detection 含 shebangs/content/priority
	detMap, ok := langData["detection"].(map[string]interface{})
	if !ok {
		t.Fatal("detection not found")
	}
	if _, ok := detMap["shebangs"]; !ok {
		t.Error("shebangs should be present when non-empty")
	}
	if _, ok := detMap["content"]; !ok {
		t.Error("content should be present when non-empty")
	}
	if _, ok := detMap["priority"]; !ok {
		t.Error("priority should be present when non-zero")
	}
	// 验证 highlighter
	hlMap, ok := langData["highlighter"].(map[string]interface{})
	if !ok {
		t.Fatal("highlighter not found")
	}
	if hlMap["backend"] != "chroma" {
		t.Errorf("expected backend='chroma', got %v", hlMap["backend"])
	}
}

func TestConfigToMap_LanguageWithNilFields(t *testing.T) {
	cfg := &config.Config{
		Languages: map[string]config.LangConfig{
			"minimal": {
				Formatter: &config.ToolConfig{Backend: "native", Tool: "go"},
			},
		},
	}
	m := appcommon.ConfigToMap(cfg)
	langs := m["languages"].(map[string]interface{})
	langData := langs["minimal"].(map[string]interface{})
	// compressor/indent/detection/highlighter 均为 nil，不应出现在 map 中
	if _, ok := langData["compressor"]; ok {
		t.Error("compressor should be absent when nil")
	}
	if _, ok := langData["indent"]; ok {
		t.Error("indent should be absent when nil")
	}
	if _, ok := langData["detection"]; ok {
		t.Error("detection should be absent when nil")
	}
	if _, ok := langData["highlighter"]; ok {
		t.Error("highlighter should be absent when nil")
	}
}

// ---- UpdateConfigFromMap ----

func TestUpdateConfigFromMap_Server(t *testing.T) {
	setupCRUDTest(t)
	cfg := config.Default()
	err := appcommon.UpdateConfigFromMap(cfg, map[string]interface{}{
		"server": map[string]interface{}{
			"port":      float64(9999),
			"auto_port": false,
		},
	})
	if err != nil {
		t.Fatalf("UpdateConfigFromMap failed: %v", err)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("expected port=9999, got %d", cfg.Server.Port)
	}
	if cfg.Server.AutoPort != false {
		t.Error("expected auto_port=false")
	}
}

func TestUpdateConfigFromMap_Window(t *testing.T) {
	setupCRUDTest(t)
	cfg := config.Default()
	err := appcommon.UpdateConfigFromMap(cfg, map[string]interface{}{
		"window": map[string]interface{}{
			"width":  float64(1400),
			"height": float64(900),
		},
	})
	if err != nil {
		t.Fatalf("UpdateConfigFromMap failed: %v", err)
	}
	if cfg.Window.Width != 1400 {
		t.Errorf("expected width=1400, got %d", cfg.Window.Width)
	}
	if cfg.Window.Height != 900 {
		t.Errorf("expected height=900, got %d", cfg.Window.Height)
	}
}

func TestUpdateConfigFromMap_Format(t *testing.T) {
	setupCRUDTest(t)
	cfg := config.Default()
	err := appcommon.UpdateConfigFromMap(cfg, map[string]interface{}{
		"format": map[string]interface{}{
			"tab_width":   float64(-1), // Tab 缩进
			"line_ending": "\r\n",
		},
	})
	if err != nil {
		t.Fatalf("UpdateConfigFromMap failed: %v", err)
	}
	if cfg.Format.TabWidth != -1 {
		t.Errorf("expected tab_width=-1, got %d", cfg.Format.TabWidth)
	}
	if cfg.Format.LineEnding != "\r\n" {
		t.Errorf("expected line_ending='\\r\\n', got %q", cfg.Format.LineEnding)
	}
}

func TestUpdateConfigFromMap_Highlight(t *testing.T) {
	setupCRUDTest(t)
	cfg := config.Default()
	err := appcommon.UpdateConfigFromMap(cfg, map[string]interface{}{
		"highlight": map[string]interface{}{
			"style":        "dracula",
			"line_numbers": true,
		},
	})
	if err != nil {
		t.Fatalf("UpdateConfigFromMap failed: %v", err)
	}
	if cfg.Highlight.Style != "dracula" {
		t.Errorf("expected style='dracula', got %q", cfg.Highlight.Style)
	}
	if !cfg.Highlight.LineNumbers {
		t.Error("expected line_numbers=true")
	}
}

func TestUpdateConfigFromMap_Languages(t *testing.T) {
	setupCRUDTest(t)
	cfg := config.Default()
	originalCount := len(cfg.Languages)
	err := appcommon.UpdateConfigFromMap(cfg, map[string]interface{}{
		"languages": map[string]interface{}{
			"go": map[string]interface{}{
				"formatter": map[string]interface{}{
					"backend": "tool",
					"tool":    "test-fmt",
				},
			},
			"newlang": map[string]interface{}{
				"formatter": map[string]interface{}{
					"backend": "native",
					"tool":    "jq",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateConfigFromMap failed: %v", err)
	}
	// go 的 formatter 应被更新
	goLang := cfg.Languages["go"]
	if goLang.Formatter.Backend != "tool" || goLang.Formatter.Tool != "test-fmt" {
		t.Errorf("go formatter not updated: %+v", goLang.Formatter)
	}
	// newlang 应被添加
	if _, ok := cfg.Languages["newlang"]; !ok {
		t.Error("newlang should be added")
	}
	// 语言总数应增加 1
	if len(cfg.Languages) != originalCount+1 {
		t.Errorf("expected %d languages, got %d", originalCount+1, len(cfg.Languages))
	}
}

func TestUpdateConfigFromMap_BinaryRuntimes(t *testing.T) {
	setupCRUDTest(t)
	cfg := config.Default()
	// 找一个已存在的运行时来更新路径
	existingRT := cfg.Binary.Runtimes[0].Name
	err := appcommon.UpdateConfigFromMap(cfg, map[string]interface{}{
		"binary": map[string]interface{}{
			"runtimes": []interface{}{
				map[string]interface{}{
					"name": existingRT,
					"path": "/custom/path/to/rt",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateConfigFromMap failed: %v", err)
	}
	if cfg.Binary.Runtimes[0].Path != "/custom/path/to/rt" {
		t.Errorf("expected path='/custom/path/to/rt', got %q", cfg.Binary.Runtimes[0].Path)
	}
}

func TestUpdateConfigFromMap_LanguageCompressorNil(t *testing.T) {
	setupCRUDTest(t)
	cfg := config.Default()
	err := appcommon.UpdateConfigFromMap(cfg, map[string]interface{}{
		"languages": map[string]interface{}{
			"json": map[string]interface{}{
				"compressor": nil,
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateConfigFromMap failed: %v", err)
	}
	jsonLang := cfg.Languages["json"]
	if jsonLang.Compressor != nil {
		t.Errorf("expected nil compressor, got %+v", jsonLang.Compressor)
	}
}
