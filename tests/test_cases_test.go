package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/formatter/formatter/src/appcommon"
	"github.com/formatter/formatter/src/binary"
	"github.com/formatter/formatter/src/compressor"
	compressornative "github.com/formatter/formatter/src/compressor/native"
	compressortool "github.com/formatter/formatter/src/compressor/tool"
	"github.com/formatter/formatter/src/config"
	"github.com/formatter/formatter/src/formatter"
	natfmt "github.com/formatter/formatter/src/formatter/native"
	formattertool "github.com/formatter/formatter/src/formatter/tool"
	"github.com/formatter/formatter/src/highlighter"
)

// ---- 配置化测试用例数据结构 ----

// CaseConfig 测试用例配置文件结构 (tests/testdata/test_cases.json)
type CaseConfig struct {
	Languages        map[string]LangConfig `json:"languages"`
	HighlightLangs   []string              `json:"highlight_languages"`
	InvalidInputs    []InvalidInputTest    `json:"invalid_inputs"`
	CLITests         []CLITestCase         `json:"cli_tests"`
}

// LangConfig 单个语言的测试配置
type LangConfig struct {
	Backend  string     `json:"backend"`
	Tool     string     `json:"tool,omitempty"`
	Skip     string     `json:"skip,omitempty"`
	Aliases  []string   `json:"aliases,omitempty"`
	Cases    []CaseDef  `json:"cases"`
}

// CaseDef 单个测试用例定义
type CaseDef struct {
	Name      string                 `json:"name"`
	Operation string                 `json:"operation"`           // format/compress/beautify
	Input     string                 `json:"input"`               // 输入文件 (相对于语言目录)
	Expected  string                 `json:"expected"`            // 期望输出文件 (相对于语言目录)
	Options   map[string]interface{} `json:"options,omitempty"`   // tab_width(-1=Tab)/style
	Methods   []string               `json:"methods,omitempty"`   // pipeline/cli
}

// InvalidInputTest 无效输入测试定义
type InvalidInputTest struct {
	Name        string `json:"name"`
	Lang        string `json:"lang"`
	Input       string `json:"input"`
	ExpectError bool   `json:"expect_error"`
}

// CLITestCase CLI 端到端测试用例
type CLITestCase struct {
	Name     string   `json:"name"`
	Args     []string `json:"args"`
	Stdin    string   `json:"stdin"`
	NeedTool string   `json:"need_tool,omitempty"`
	Checks   []string `json:"checks"`
}

// loadCaseConfig 加载测试用例配置
func loadCaseConfig(t *testing.T) *CaseConfig {
	t.Helper()
	path := filepath.Join(testdataDir(t), "test_cases.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("无法读取测试配置 %s: %v", path, err)
	}
	var cc CaseConfig
	if err := json.Unmarshal(data, &cc); err != nil {
		t.Fatalf("解析测试配置失败: %v", err)
	}
	return &cc
}

// ---- 选项解析 ----

// parseOptions 从 CaseDef.Options 构建 PipelineOptions
// tab_width 选项: -1=Tab 缩进, >0=空格缩进宽度, 0=使用配置默认值
func parseOptions(lang string, tc CaseDef) appcommon.PipelineOptions {
	opts := appcommon.PipelineOptions{Language: lang}
	if v, ok := tc.Options["tab_width"]; ok {
		if f, ok := v.(float64); ok {
			opts.TabWidth = int(f)
		}
	}
	// 兼容旧配置: use_tabs=true 时转换为 tab_width=-1
	if opts.TabWidth == 0 {
		if v, ok := tc.Options["use_tabs"]; ok {
			if b, ok := v.(bool); ok && b {
				opts.TabWidth = -1
			}
		}
	}
	if v, ok := tc.Options["style"]; ok {
		if s, ok := v.(string); ok {
			opts.Style = s
		}
	}
	// 美化&高亮 = format + highlight, 不压缩
	if tc.Operation == "beautify" {
		opts.NoCompress = true
	}
	return opts
}

// ---- 执行器 ----

// runPipeline 通过 Go 代码直接调用 pipeline 执行操作
func runPipeline(operation, input string, opts appcommon.PipelineOptions) (string, error) {
	switch operation {
	case "format":
		return testCore.ExecutePipeline(context.Background(), input, testCore.FormatParams(opts))
	case "compress":
		return testCore.ExecutePipeline(context.Background(), input, testCore.CompressParams(opts))
	case "highlight":
		return testCore.ExecutePipeline(context.Background(), input, testCore.HighlightParams(opts))
	case "beautify":
		return testCore.ExecutePipeline(context.Background(), input, testCore.RunParams(opts))
	default:
		return "", fmt.Errorf("未知操作: %s", operation)
	}
}

// runCLIViaApp 通过 CLI (编译好的 App 二进制) 执行操作
func runCLIViaApp(cliPath, operation, input string, opts appcommon.PipelineOptions, binDir string) (string, error) {
	args := []string{"-l", opts.Language, operation}
	if opts.TabWidth != 0 {
		args = append(args, "--tab-width", fmt.Sprintf("%d", opts.TabWidth))
	}
	if opts.Style != "" {
		args = append(args, "--style", opts.Style)
	}
	return execCLI(cliPath, args, []string{"FORMATTER_BIN_HOME=" + binDir}, input)
}

// hasMethod 检查用例是否指定了测试方式
func hasMethod(methods []string, method string) bool {
	if len(methods) == 0 {
		return method == "pipeline" // 默认 pipeline
	}
	for _, m := range methods {
		if m == method {
			return true
		}
	}
	return false
}

// ---- 核心测试: Golden File 驱动 ----

// TestCases_GoldenFiles 从 test_cases.json 驱动，对每种语言的每个用例
// 按 methods (pipeline/cli) 执行操作，将结果与期望文件精确比较。
// 各语言用例并行执行以加快测试速度。
func TestCases_GoldenFiles(t *testing.T) {
	cc := loadCaseConfig(t)
	binDir := os.Getenv("FORMATTER_BIN_HOME")
	cliPath := findAppBinary()

	for lang, lc := range cc.Languages {
		lang, lc := lang, lc
		for _, tc := range lc.Cases {
			tc := tc
			caseName := lang + "/" + tc.Name
			t.Run(caseName, func(t *testing.T) {
				t.Parallel()
				if lc.Skip != "" {
					t.Skipf("跳过 %s: %s", lang, lc.Skip)
				}
				if lc.Backend == "tool" && lc.Tool != "" {
					if !toolAvailable(binDir, lc.Tool) {
						t.Skipf("跳过 %s: 工具 %s 未安装", lang, lc.Tool)
					}
				}

				input := loadLangFile(t, lang, tc.Input)
				opts := parseOptions(lang, tc)

				// pipeline 方式
				if hasMethod(tc.Methods, "pipeline") {
					t.Run("pipeline", func(t *testing.T) {
						t.Parallel()
						result, err := runPipeline(tc.Operation, input, opts)
						if err != nil {
							t.Fatalf("%s %s (pipeline) 失败: %v", tc.Operation, caseName, err)
						}
						if len(result) == 0 {
							t.Fatalf("%s %s (pipeline) 返回空结果", tc.Operation, caseName)
						}
						compareGolden(t, expectedPath(t, lang, tc.Expected), result)
					})
				}

				// cli 方式: 始终比较 (不写入期望文件，期望文件由 pipeline 生成)
				if hasMethod(tc.Methods, "cli") {
					t.Run("cli", func(t *testing.T) {
						t.Parallel()
						if cliPath == "" {
							t.Skip("App 二进制未构建，跳过 CLI 测试")
						}
						if *update {
							t.Skip("-update 模式下跳过 CLI 测试 (测试输出文件由 pipeline 生成)")
						}
						result, err := runCLIViaApp(cliPath, tc.Operation, input, opts, binDir)
						if err != nil {
							t.Fatalf("%s %s (cli) 失败: %v\n输出: %s", tc.Operation, caseName, err, result)
						}
						if len(result) == 0 {
							t.Fatalf("%s %s (cli) 返回空结果", tc.Operation, caseName)
						}
						compareGolden(t, expectedPath(t, lang, tc.Expected), result)
					})
				}
			})
		}
	}
}

// ---- 测试数据生成 ----

// TestCases_GenerateTestData 专门用于生成各个语言的测试数据 (.output 文件)。
// 仅在 -update 模式下执行: 遍历所有语言的全部用例，执行 pipeline 并写入 .output，
// 随后与现有 .expected 逐一比对并输出审查报告 (一致/不一致/缺失)，
// 确保 .output 始终反映当前工具链的真实输出，避免输出文件长期未更新。
// 运行方式: go test ./tests/ -run TestCases_GenerateTestData -update -count=1
func TestCases_GenerateTestData(t *testing.T) {
	if !*update {
		t.Skip("仅在 -update 模式下生成测试数据: go test ./tests/ -run TestCases_GenerateTestData -update -count=1")
	}
	cc := loadCaseConfig(t)
	binDir := os.Getenv("FORMATTER_BIN_HOME")

	var generated, skippedLangs, matched, mismatched, missing []string
	for lang, lc := range cc.Languages {
		if lc.Skip != "" {
			skippedLangs = append(skippedLangs, fmt.Sprintf("%s (%s)", lang, lc.Skip))
			continue
		}
		if lc.Backend == "tool" && lc.Tool != "" && !toolAvailable(binDir, lc.Tool) {
			skippedLangs = append(skippedLangs, fmt.Sprintf("%s (工具 %s 未安装)", lang, lc.Tool))
			continue
		}
		for _, tc := range lc.Cases {
			name := lang + "/" + tc.Name
			input := loadLangFile(t, lang, tc.Input)
			result, err := runPipeline(tc.Operation, input, parseOptions(lang, tc))
			if err != nil {
				t.Errorf("生成 %s 失败: %v", name, err)
				continue
			}
			expPath := expectedPath(t, lang, tc.Expected)
			outputFile := strings.TrimSuffix(expPath, ".expected") + ".output"
			if err := os.WriteFile(outputFile, []byte(result), 0644); err != nil {
				t.Fatalf("无法写入测试输出文件 %s: %v", outputFile, err)
			}
			generated = append(generated, outputFile)

			expected, err := os.ReadFile(expPath)
			if err != nil {
				missing = append(missing, name)
				continue
			}
			if normalizeLineEndings(result) == normalizeLineEndings(string(expected)) {
				matched = append(matched, name)
			} else {
				mismatched = append(mismatched, name)
			}
		}
	}

	t.Logf("已生成 %d 个测试输出文件; 与期望一致 %d 个; 不一致 %d 个; 缺失期望文件 %d 个; 跳过语言 %d 个",
		len(generated), len(matched), len(mismatched), len(missing), len(skippedLangs))
	for _, s := range skippedLangs {
		t.Logf("跳过: %s", s)
	}
	for _, m := range missing {
		t.Logf("缺失期望文件: %s (审查对应 .output 后复制为 .expected)", m)
	}
	for _, m := range mismatched {
		t.Logf("输出与期望不一致: %s (请审查 .output 与 .expected 的差异)", m)
	}
	if len(missing) > 0 || len(mismatched) > 0 {
		t.Errorf("存在 %d 个缺失期望文件与 %d 个不一致用例，请审查 .output 后更新 .expected",
			len(missing), len(mismatched))
	}
}

// ---- 配置化高亮测试 ----

// TestCases_HighlightAllLanguages 测试所有语言 (含别名) 的高亮输出包含 HTML 标记
func TestCases_HighlightAllLanguages(t *testing.T) {
	cc := loadCaseConfig(t)

	// 构建别名 → 语言目录映射
	aliasToLang := make(map[string]string)
	for lang, lc := range cc.Languages {
		aliasToLang[lang] = lang
		for _, alias := range lc.Aliases {
			aliasToLang[alias] = lang
		}
	}

	for _, lang := range cc.HighlightLangs {
		lang := lang
		t.Run(lang, func(t *testing.T) {
			t.Parallel()
			dirLang := aliasToLang[lang]
			if dirLang == "" {
				t.Fatalf("语言 %s 未配置测试文件", lang)
			}
			// 使用语言目录下的 input 文件 (取第一个用例的 input)
			lc := cc.Languages[dirLang]
			if len(lc.Cases) == 0 {
				t.Fatalf("语言 %s 无测试用例", dirLang)
			}
			code := loadLangFile(t, dirLang, lc.Cases[0].Input)
			result, err := testCore.ExecutePipeline(context.Background(), code,
				testCore.HighlightParams(appcommon.PipelineOptions{
					Language: lang,
					Style:    "monokai",
				}))
			if err != nil {
				t.Fatalf("高亮 %s 失败: %v", lang, err)
			}
			if len(result) == 0 {
				t.Fatalf("高亮 %s 返回空结果", lang)
			}
			hasHTML := strings.Contains(result, "<") && strings.Contains(result, "class=")
			hasANSI := strings.Contains(result, "\x1b[")
			hasSpan := strings.Contains(result, "<span")
			if !hasHTML && !hasANSI && !hasSpan {
				t.Errorf("高亮 %s 结果不包含格式化标记，前200字符: %s", lang, truncate(result, 200))
			}
		})
	}
}

// ---- 配置化错误处理测试 ----

// TestCases_InvalidInputs 测试无效输入的错误处理
func TestCases_InvalidInputs(t *testing.T) {
	cc := loadCaseConfig(t)

	for _, ii := range cc.InvalidInputs {
		ii := ii
		t.Run(ii.Name, func(t *testing.T) {
			t.Parallel()
			_, err := testCore.ExecutePipeline(context.Background(), ii.Input,
				testCore.FormatParams(appcommon.PipelineOptions{Language: ii.Lang}))
			if ii.ExpectError && err == nil {
				t.Errorf("期望 %s 格式化返回错误，但成功了", ii.Name)
			}
			if !ii.ExpectError && err != nil {
				t.Errorf("期望 %s 格式化成功，但返回错误: %v", ii.Name, err)
			}
		})
	}
}

// TestCases_UnsupportedLanguage 测试不支持的语言返回错误
func TestCases_UnsupportedLanguage(t *testing.T) {
	_, err := testCore.ExecutePipeline(context.Background(), "test",
		testCore.FormatParams(appcommon.PipelineOptions{Language: "unknown_lang_xyz"}))
	if err == nil {
		t.Error("期望不支持的语言返回错误")
	}
}

// ---- 状态和语言列表测试 ----

// TestCases_Status 测试核心状态
func TestCases_Status(t *testing.T) {
	status := testCore.GetStatus()
	if !status.Success {
		t.Fatal("期望状态成功")
	}
	if status.Version == "" {
		t.Error("期望版本号非空")
	}
	t.Logf("版本: %s, 平台: %s", status.Version, status.Platform)
	t.Logf("安装目录: %s", status.InstallDir)
	t.Logf("已安装工具: %v", status.InstalledBin)
	t.Logf("缺失工具: %v", status.MissingBin)
}

// TestCases_LanguageList 测试支持的语言列表完整 (与 config.json 一致)
func TestCases_LanguageList(t *testing.T) {
	cfg := config.Default()
	langs := testCore.ListLanguages()
	if len(langs) == 0 {
		t.Fatal("期望至少有一个支持的语言")
	}

	registered := make(map[string]bool)
	for _, info := range langs {
		registered[info.Lang] = true
		t.Logf("语言: %-12s fmt=%-35s cmp=%-30s hl=%s",
			info.Lang, info.Formatter, info.Compressor, info.Highlighter)
	}

	for lang := range cfg.Languages {
		if !registered[lang] {
			t.Errorf("配置中的语言 %q 未在 pipeline 注册表中找到", lang)
		}
	}
}

// ---- 配置一致性验证 ----

// TestCases_ConfigConsistency 验证 test_cases.json 与 data/config.json 的一致性
func TestCases_ConfigConsistency(t *testing.T) {
	cc := loadCaseConfig(t)
	cfg := config.Default()

	// 1. test_cases.json 中的每个语言都应在 config.json 中有配置
	for lang := range cc.Languages {
		if _, ok := cfg.Languages[lang]; !ok {
			t.Errorf("test_cases.json 中的语言 %q 在 config.json 中未找到", lang)
		}
	}

	// 2. backend 与 tool 应与 config.json 一致
	for lang, lc := range cc.Languages {
		langCfg, ok := cfg.Languages[lang]
		if !ok || langCfg.Formatter == nil {
			continue
		}
		if langCfg.Formatter.Backend != lc.Backend {
			t.Errorf("语言 %q backend 不一致: test_cases=%s, config=%s",
				lang, lc.Backend, langCfg.Formatter.Backend)
		}
		if lc.Backend == "tool" && lc.Tool != "" && langCfg.Formatter.Tool != lc.Tool {
			t.Errorf("语言 %q tool 不一致: test_cases=%s, config=%s (工具名错误会导致用例被静默跳过)",
				lang, lc.Tool, langCfg.Formatter.Tool)
		}
	}

	// 3. 高亮语言列表应覆盖 config.json 中的所有语言
	for lang := range cfg.Languages {
		found := false
		for _, hl := range cc.HighlightLangs {
			if hl == lang {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("config.json 中的语言 %q 未在高亮测试列表中找到", lang)
		}
	}

	// 4. 验证 config 中每个语言都在 pipeline 注册表中有对应的格式化器
	langMap := make(map[string]appcommon.LanguageInfo)
	for _, info := range testCore.ListLanguages() {
		langMap[info.Lang] = info
	}
	for lang, lc := range cfg.Languages {
		info, ok := langMap[lang]
		if !ok {
			t.Errorf("配置中的语言 %q 未在 pipeline 注册表中找到", lang)
			continue
		}
		if lc.Formatter == nil {
			t.Errorf("语言 %q 的 formatter 配置为 nil", lang)
			continue
		}
		if info.Formatter == "" {
			t.Errorf("语言 %q 未注册格式化器 (配置声明: %s/%s)", lang, lc.Formatter.Backend, lc.Formatter.Tool)
		}
		if info.Highlighter == "" {
			t.Errorf("语言 %q 未注册高亮器", lang)
		}
		if lc.Compressor != nil && info.Compressor == "" {
			t.Errorf("语言 %q 配置了压缩器 %s/%s 但 pipeline 未注册", lang, lc.Compressor.Backend, lc.Compressor.Tool)
		}
		if lc.Compressor == nil && info.Compressor != "" {
			t.Errorf("语言 %q 未配置压缩器但 pipeline 注册了 %s", lang, info.Compressor)
		}
	}
}

// ---- CLI 端到端测试 ----

// TestCases_CLIEndToEnd 从 test_cases.json 的 cli_tests 驱动，
// 使用编译得到的 App 二进制执行格式化/压缩/高亮，完整验证业务流程
func TestCases_CLIEndToEnd(t *testing.T) {
	cc := loadCaseConfig(t)

	cliPath := findAppBinary()
	if cliPath == "" {
		t.Skip("App 二进制未构建，跳过 CLI 端到端测试 (运行 run_tools.sh 构建后再测)")
	}
	t.Logf("使用 App 二进制: %s", cliPath)

	binDir, _ := filepath.Abs("data/bin")

	for _, ct := range cc.CLITests {
		ct := ct
		t.Run(ct.Name, func(t *testing.T) {
			t.Parallel()
			if ct.NeedTool != "" && !toolAvailable(binDir, ct.NeedTool) {
				t.Skipf("跳过 %s: 依赖工具 %s 未安装", ct.Name, ct.NeedTool)
			}
			output, err := execCLI(cliPath, ct.Args,
				[]string{"FORMATTER_BIN_HOME=" + binDir}, ct.Stdin)
			if err != nil {
				t.Fatalf("CLI 命令 %v 失败: %v\n输出: %s", ct.Args, err, output)
			}
			for _, check := range ct.Checks {
				if !strings.Contains(output, check) {
					t.Errorf("CLI 输出不符合预期，期望包含 %q，前300字符: %s",
						check, truncate(output, 300))
				}
			}
		})
	}
}

// findAppBinary 查找已编译的 App 二进制路径
func findAppBinary() string {
	candidates := []string{
		"build/bin/formatter",
		"build/bin/Formatter.app/Contents/MacOS/Formatter",
		"./formatter",
		"release/Formatter.app/Contents/MacOS/Formatter",
	}
	for _, p := range candidates {
		if fileExists(p) {
			return p
		}
	}
	if path, err := exec.LookPath("formatter"); err == nil {
		return path
	}
	return ""
}

// =============================================================================
// 工具元数据单元测试 (无法配置化的 API 层校验)
// 格式化/压缩/高亮的行为测试统一由上方 test_cases.json golden file 驱动
// =============================================================================

// TestTools_NativeFormatterMetadata 验证内置 Go 格式化器的元数据 (Name/Type/SupportedLanguages)
func TestTools_NativeFormatterMetadata(t *testing.T) {
	tests := []struct {
		name     string
		factory  func() formatter.Formatter
		wantName string
		wantLang []string
	}{
		{"json", func() formatter.Formatter { return natfmt.NewJSONFormatter() }, "native-json", []string{"json"}},
		{"yaml", func() formatter.Formatter { return natfmt.NewYAMLFormatter() }, "native-yaml", []string{"yaml", "yml"}},
		{"xml", func() formatter.Formatter { return natfmt.NewXMLFormatter() }, "native-xml", []string{"xml"}},
		{"html", func() formatter.Formatter { return natfmt.NewHTMLFormatter() }, "native-html", []string{"html"}},
		{"go", func() formatter.Formatter { return natfmt.NewGoFormatter() }, "native-go", []string{"go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.factory()
			if f.Name() != tt.wantName {
				t.Errorf("expected name %q, got %q", tt.wantName, f.Name())
			}
			if f.Type() != formatter.NativeGo {
				t.Errorf("expected type NativeGo, got %v", f.Type())
			}
			langs := f.SupportedLanguages()
			if len(langs) != len(tt.wantLang) {
				t.Fatalf("expected languages %v, got %v", tt.wantLang, langs)
			}
			for i, l := range tt.wantLang {
				if langs[i] != l {
					t.Errorf("expected language[%d]=%q, got %q", i, l, langs[i])
				}
			}
		})
	}
}

// TestTools_SQLFormatterMetadata 验证 SQL 格式化器支持 sql 语言
func TestTools_SQLFormatterMetadata(t *testing.T) {
	f := natfmt.NewSQLFormatter()
	found := false
	for _, l := range f.SupportedLanguages() {
		if l == "sql" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'sql' in supported languages, got %v", f.SupportedLanguages())
	}
}

// newTestFormatterAdapter 构造一个用于元数据测试的外部工具适配器
func newTestFormatterAdapter(t *testing.T, toolName, lang string) *formattertool.CmdAdapter {
	t.Helper()
	mgr := binary.NewManager(t.TempDir())
	cfg := &config.ToolConfig{Backend: "tool", Tool: "test"}
	return formattertool.NewCmdAdapter(toolName, lang, cfg, config.Default(), mgr, nil)
}

// TestTools_FormatterAdapterMetadata 验证外部工具适配器的 Name/Type/SupportedLanguages
func TestTools_FormatterAdapterMetadata(t *testing.T) {
	a := newTestFormatterAdapter(t, "test-tool", "javascript")
	if langs := a.SupportedLanguages(); len(langs) != 1 || langs[0] != "javascript" {
		t.Errorf("expected ['javascript'], got %v", langs)
	}
	if a.Name() != "external-test-tool" {
		t.Errorf("expected name='external-test-tool', got %q", a.Name())
	}
	if a.Type() != formatter.ToolBinary {
		t.Errorf("expected type=ToolBinary, got %v", a.Type())
	}
}

// TestTools_NativeCompressorMetadata 验证内置压缩器的 SupportedLanguages
func TestTools_NativeCompressorMetadata(t *testing.T) {
	tests := []struct {
		name     string
		langs    []string
		factory  func() compressor.Compressor
	}{
		{"css", []string{"css"}, func() compressor.Compressor { return compressornative.NewCSSCompressor() }},
		{"html", []string{"html"}, func() compressor.Compressor { return compressornative.NewHTMLCompressor() }},
		{"javascript", []string{"javascript"}, func() compressor.Compressor { return compressornative.NewJSCompressor() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.factory().SupportedLanguages()
			for _, want := range tt.langs {
				found := false
				for _, l := range got {
					if l == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected %q in supported languages, got %v", want, got)
				}
			}
		})
	}
}

// TestTools_CompressAdapterMetadata 验证压缩工具适配器的元数据
func TestTools_CompressAdapterMetadata(t *testing.T) {
	mgr := binary.NewManager(t.TempDir())
	cfg := &config.ToolConfig{Backend: "tool", Tool: "test"}
	a := compressortool.NewCmdCompressAdapter("test-tool", "json", cfg, config.Default(), mgr)
	if langs := a.SupportedLanguages(); len(langs) != 1 || langs[0] != "json" {
		t.Errorf("expected ['json'], got %v", langs)
	}
	if a.Name() != "external-test-tool" {
		t.Errorf("expected name='external-test-tool', got %q", a.Name())
	}
	if a.Type() != compressor.ToolBinary {
		t.Errorf("expected type=ToolBinary, got %v", a.Type())
	}
}

// TestTools_CompressAdapter_ToolNotFound 验证工具不存在时 Compress 返回错误
func TestTools_CompressAdapter_ToolNotFound(t *testing.T) {
	mgr := binary.NewManager(t.TempDir())
	cfg := &config.ToolConfig{
		Backend: "tool",
		Tool:    "nonexistent-tool",
		Cmd:     []string{"{exe}"},
	}
	a := compressortool.NewCmdCompressAdapter("nonexistent-tool", "json", cfg, config.Default(), mgr)
	_, err := a.Compress([]byte("test"), "json", compressor.CompressOptions{})
	if err == nil {
		t.Error("expected error for nonexistent tool")
	}
}

// TestTools_ChromaStyles 验证 chroma 高亮器支持多种主题样式 (golden beautify 用例固定 monokai)
func TestTools_ChromaStyles(t *testing.T) {
	h := highlighter.NewChromaHighlighter(nil)
	code := "const x = 42;\n"
	for _, style := range []string{"monokai", "github", "dracula", "solarized-dark"} {
		t.Run(style, func(t *testing.T) {
			t.Parallel()
			out, err := h.Highlight([]byte(code), "javascript", highlighter.HighlightOptions{Style: style})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(out) == 0 {
				t.Error("expected non-empty output")
			}
		})
	}
}

// TestTools_ChromaOutputTypes 验证 chroma 高亮器支持多种输出类型 (html/terminal)
func TestTools_ChromaOutputTypes(t *testing.T) {
	h := highlighter.NewChromaHighlighter(nil)
	for _, ot := range []string{"html", "terminal256", "terminal16m"} {
		t.Run(ot, func(t *testing.T) {
			t.Parallel()
			out, err := h.Highlight([]byte("x := 1\n"), "go", highlighter.HighlightOptions{OutputType: ot})
			if err != nil {
				t.Fatalf("unexpected error for output type %s: %v", ot, err)
			}
			if len(out) == 0 {
				t.Errorf("expected non-empty output for %s", ot)
			}
		})
	}
}

// TestTools_ChromaUnknownLanguage 验证未知语言不报错 (回退纯文本高亮)
func TestTools_ChromaUnknownLanguage(t *testing.T) {
	h := highlighter.NewChromaHighlighter(nil)
	out, err := h.Highlight([]byte("some text { with } symbols"), "unknown_lang_xyz", highlighter.HighlightOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected non-empty output even for unknown language")
	}
}

// TestTools_ChromaMetadata 验证 chroma 高亮器的元数据与语言覆盖映射
func TestTools_ChromaMetadata(t *testing.T) {
	// 无覆盖映射时，SupportedLanguages 返回空 (高亮器动态支持所有语言)
	h := highlighter.NewChromaHighlighter(nil)
	if h.Name() != "chroma" {
		t.Errorf("expected name 'chroma', got %q", h.Name())
	}
	if langs := h.SupportedLanguages(); len(langs) != 0 {
		t.Errorf("expected empty supported languages without overrides, got %v", langs)
	}

	// 有覆盖映射时，SupportedLanguages 返回覆盖的语言列表
	h2 := highlighter.NewChromaHighlighter(map[string]string{
		"python": "python3",
		"shell":  "bash",
	})
	langs := h2.SupportedLanguages()
	if len(langs) != 2 {
		t.Fatalf("expected 2 override languages, got %d: %v", len(langs), langs)
	}
	for _, expected := range []string{"python", "shell"} {
		found := false
		for _, l := range langs {
			if l == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in override languages", expected)
		}
	}
}
