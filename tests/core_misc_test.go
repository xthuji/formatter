package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/formatter/formatter/src/appcommon"
	"github.com/formatter/formatter/src/binary"
	"github.com/formatter/formatter/src/config"
	iocore "github.com/formatter/formatter/src/io"
)

// ---- ListBinaries ----

func TestListBinaries_ReturnsNonEmpty(t *testing.T) {
	bins := testCore.ListBinaries()
	if len(bins) == 0 {
		t.Fatal("expected non-empty binaries list")
	}

	// 验证每个 BinInfo 的关键字段非空
	for _, bin := range bins {
		if bin.Name == "" {
			t.Error("binary name should not be empty")
		}
		if bin.Version == "" {
			t.Errorf("binary %q: version should not be empty", bin.Name)
		}
		// ConfigSource 应为 download/preset/install 之一
		switch bin.ConfigSource {
		case "download", "preset", "install":
			// OK
		default:
			t.Errorf("binary %q: unexpected config_source %q", bin.Name, bin.ConfigSource)
		}
	}
}

func TestListBinaries_ContainsKnownTool(t *testing.T) {
	bins := testCore.ListBinaries()
	// oxfmt 应在配置中
	found := false
	for _, bin := range bins {
		if bin.Name == "oxfmt" {
			found = true
			if bin.Languages == nil || len(bin.Languages) == 0 {
				t.Errorf("oxfmt should have languages, got %v", bin.Languages)
			}
		}
	}
	if !found {
		t.Error("expected 'oxfmt' in binaries list")
	}
}

// ---- InstallBinary edge cases ----

func TestInstallBinary_EmptyName(t *testing.T) {
	result := testCore.InstallBinary("")
	if result.Success {
		t.Error("expected failure for empty tool name")
	}
	if result.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestInstallBinary_NotFound(t *testing.T) {
	result := testCore.InstallBinary("nonexistent-tool-xyz")
	if result.Success {
		t.Error("expected failure for nonexistent tool")
	}
}

// ---- UninstallBinary edge cases ----

func TestUninstallBinary_EmptyName(t *testing.T) {
	result := testCore.UninstallBinary("")
	if result.Success {
		t.Error("expected failure for empty tool name")
	}
}

func TestUninstallBinary_NotInstalled(t *testing.T) {
	// 使用注册表中存在但不在 data/bin 中的工具 (rubocop 是 source=install，不在 data/bin)
	// 注意: 不能使用 oxfmt-wrapper 等预置工具，否则会删除真实文件
	result := testCore.UninstallBinary("rubocop")
	if result.Success {
		t.Error("expected failure for tool not installed in bin dir")
	}
}

// ---- VerifyBinary edge cases ----

func TestVerifyBinary_EmptyName(t *testing.T) {
	result := testCore.VerifyBinary("")
	if result.Success {
		t.Error("expected failure for empty tool name")
	}
}

// ---- InstallRuntime edge cases ----

func TestInstallRuntime_EmptyName(t *testing.T) {
	result := testCore.InstallRuntime("")
	if result.Success {
		t.Error("expected failure for empty runtime name")
	}
}

func TestInstallRuntime_UnsupportedPlatform(t *testing.T) {
	// 使用一个不存在的运行时名
	result := testCore.InstallRuntime("nonexistent-runtime-xyz")
	if result.Success {
		t.Error("expected failure for nonexistent runtime")
	}
}

// ---- FindInstallScript ----

func TestFindInstallScript_DevMode(t *testing.T) {
	// 在开发模式下 (TestMain 切换到了项目根目录)，
	// scripts/install-bin.sh 应该能被找到
	path := appcommon.FindInstallScript()
	if path == "" {
		t.Skip("install script not found (may not be in dev mode)")
	}
}

// ---- GetStatus ----

func TestGetStatus_ReturnsValidInfo(t *testing.T) {
	status := testCore.GetStatus()
	if status.Version == "" {
		t.Error("expected non-empty version in status")
	}
}

// ---- resolveTabWidth / resolveStyle / resolveLineEnding (通过 FormatParams/HighlightParams/RunParams 间接测试) ----

func TestFormatParams_TabWidth(t *testing.T) {
	params := testCore.FormatParams(appcommon.PipelineOptions{
		Language: "go",
		TabWidth: 8,
	})
	if params.FormatOpts.TabWidth != 8 {
		t.Errorf("expected TabWidth=8, got %d", params.FormatOpts.TabWidth)
	}
}

func TestFormatParams_DefaultTabWidth(t *testing.T) {
	// 当 TabWidth=0 时，应回退到语言级配置 (若有) 或全局默认值
	// "go" 语言有 indent.tab_width=4，应使用语言级配置
	params := testCore.FormatParams(appcommon.PipelineOptions{Language: "go"})
	goIndent := testCore.Cfg.Languages["go"].Indent
	expected := testCore.Cfg.Format.TabWidth
	if goIndent != nil {
		expected = goIndent.TabWidth
	}
	if params.FormatOpts.TabWidth != expected {
		t.Errorf("expected TabWidth=%d (language/global default), got %d", expected, params.FormatOpts.TabWidth)
	}
}

func TestFormatParams_LineEnding(t *testing.T) {
	params := testCore.FormatParams(appcommon.PipelineOptions{
		Language:   "go",
		LineEnding: "\r\n",
	})
	if params.FormatOpts.LineEnding != "\r\n" {
		t.Errorf("expected LineEnding='\\r\\n', got %q", params.FormatOpts.LineEnding)
	}
}

func TestFormatParams_DefaultLineEnding(t *testing.T) {
	params := testCore.FormatParams(appcommon.PipelineOptions{Language: "go"})
	if params.FormatOpts.LineEnding != testCore.Cfg.Format.LineEnding {
		t.Errorf("expected default LineEnding=%q, got %q", testCore.Cfg.Format.LineEnding, params.FormatOpts.LineEnding)
	}
}

// ---- HighlightParams (resolveStyle) ----

func TestHighlightParams_Style(t *testing.T) {
	params := testCore.HighlightParams(appcommon.PipelineOptions{
		Language: "go",
		Style:    "monokai",
	})
	if params.HighlightOpts.Style != "monokai" {
		t.Errorf("expected Style='monokai', got %q", params.HighlightOpts.Style)
	}
}

func TestHighlightParams_DefaultStyle(t *testing.T) {
	params := testCore.HighlightParams(appcommon.PipelineOptions{Language: "go"})
	if params.HighlightOpts.Style != testCore.Cfg.Highlight.Style {
		t.Errorf("expected default Style=%q, got %q", testCore.Cfg.Highlight.Style, params.HighlightOpts.Style)
	}
}

// ---- RunParams (覆盖 format+compress+highlight 组合) ----

func TestRunParams_Default(t *testing.T) {
	params := testCore.RunParams(appcommon.PipelineOptions{Language: "go"})
	if !params.EnableFormat {
		t.Error("expected EnableFormat=true by default")
	}
	if !params.EnableCompress {
		t.Error("expected EnableCompress=true by default")
	}
	if !params.EnableHighlight {
		t.Error("expected EnableHighlight=true by default")
	}
}

func TestRunParams_NoFormat(t *testing.T) {
	params := testCore.RunParams(appcommon.PipelineOptions{
		Language: "go",
		NoFormat: true,
	})
	if params.EnableFormat {
		t.Error("expected EnableFormat=false when NoFormat=true")
	}
}

func TestRunParams_NoCompress(t *testing.T) {
	params := testCore.RunParams(appcommon.PipelineOptions{
		Language:  "go",
		NoCompress: true,
	})
	if params.EnableCompress {
		t.Error("expected EnableCompress=false when NoCompress=true")
	}
}

func TestRunParams_NoHighlight(t *testing.T) {
	params := testCore.RunParams(appcommon.PipelineOptions{
		Language:    "go",
		NoHighlight: true,
	})
	if params.EnableHighlight {
		t.Error("expected EnableHighlight=false when NoHighlight=true")
	}
}

// ---- CompressParams ----

func TestCompressParams(t *testing.T) {
	params := testCore.CompressParams(appcommon.PipelineOptions{Language: "json"})
	if params.EnableFormat {
		t.Error("expected EnableFormat=false for CompressParams")
	}
	if !params.EnableCompress {
		t.Error("expected EnableCompress=true for CompressParams")
	}
}

// ---- resolveIndent 语言级配置覆盖 ----

func TestFormatParams_LanguageIndentOverride(t *testing.T) {
	// 找一个有语言级 indent 配置的语言
	var testLang string
	var expectedTabWidth int
	for lang, lc := range testCore.Cfg.Languages {
		if lc.Indent != nil && lc.Indent.TabWidth > 0 {
			testLang = lang
			expectedTabWidth = lc.Indent.TabWidth
			break
		}
	}
	if testLang == "" {
		t.Skip("no language with indent config found")
	}

	// 不指定 TabWidth，应使用语言级配置
	params := testCore.FormatParams(appcommon.PipelineOptions{Language: testLang})
	if params.FormatOpts.TabWidth != expectedTabWidth {
		t.Errorf("expected language-level TabWidth=%d for %q, got %d", expectedTabWidth, testLang, params.FormatOpts.TabWidth)
	}

	// 指定 TabWidth，应覆盖语言级配置
	params = testCore.FormatParams(appcommon.PipelineOptions{
		Language: testLang,
		TabWidth: 8,
	})
	if params.FormatOpts.TabWidth != 8 {
		t.Errorf("expected explicit TabWidth=8, got %d", params.FormatOpts.TabWidth)
	}
}

// ---- 边界用例 ----

// ---- DefaultRuntimeDefs ----

func TestDefaultRuntimeDefs(t *testing.T) {
	defs := appcommon.DefaultRuntimeDefs()
	if len(defs) == 0 {
		t.Fatal("expected non-empty default runtime defs")
	}
	// 验证包含 node/python/java/ruby
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
		if d.Exe == "" {
			t.Errorf("runtime %q: exe should not be empty", d.Name)
		}
	}
	for _, expected := range []string{"node", "python", "java", "ruby"} {
		if !names[expected] {
			t.Errorf("expected %q in default runtime defs", expected)
		}
	}
}

// ---- ClipboardReader ----

func TestClipboardReader_Source(t *testing.T) {
	r := iocore.NewClipboardReader()
	if r.Source() != "clipboard" {
		t.Errorf("expected 'clipboard', got %q", r.Source())
	}
}

func TestClipboardReader_Read_Empty(t *testing.T) {
	// 在测试环境中剪贴板可能为空，验证不 panic 即可
	r := iocore.NewClipboardReader()
	_, _, err := r.Read()
	// 剪贴板为空时应返回 error，有内容时应成功
	_ = err
}

// ---- ClipboardWriter ----

func TestClipboardWriter_Dest(t *testing.T) {
	w := iocore.NewClipboardWriter()
	if w.Dest() != "clipboard" {
		t.Errorf("expected 'clipboard', got %q", w.Dest())
	}
}

func TestClipboardWriter_Write(t *testing.T) {
	w := iocore.NewClipboardWriter()
	err := w.Write([]byte("test"), false)
	if err != nil {
		t.Errorf("unexpected error writing to clipboard: %v", err)
	}
}

// ---- extractTarXZ ----

func TestExtract_TarXZ(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tar.xz not supported on Windows without system tar")
	}
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar command not available")
	}

	tmpDir := t.TempDir()

	// 创建源文件
	srcFile := filepath.Join(tmpDir, "source.txt")
	srcContent := "tar.xz test content"
	if err := os.WriteFile(srcFile, []byte(srcContent), 0644); err != nil {
		t.Fatalf("create source file: %v", err)
	}

	// 使用系统 tar 创建 .tar.xz
	archivePath := filepath.Join(tmpDir, "test.tar.xz")
	cmd := exec.Command("tar", "-cJf", archivePath, "-C", tmpDir, "source.txt")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create tar.xz: %v\n%s", err, string(out))
	}

	// 解压
	destDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("mkdir destDir: %v", err)
	}

	if err := binary.Extract(archivePath, destDir, binary.ArchiveTypeTarXZ); err != nil {
		t.Fatalf("Extract tar.xz failed: %v", err)
	}

	// 验证文件
	extractedPath := filepath.Join(destDir, "source.txt")
	data, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(data) != srcContent {
		t.Errorf("expected %q, got %q", srcContent, string(data))
	}
}

// ---- NewCore with config load failure ----

func TestNewCore_ConfigLoadFailure(t *testing.T) {
	// NewCore 在配置加载失败时回退到默认配置
	// 通过传入一个不存在的目录来测试
	core := appcommon.NewCore("/nonexistent/path/that/does/not/exist")
	if core == nil {
		t.Fatal("expected non-nil Core even with bad path")
	}
	if core.Cfg == nil {
		t.Error("expected non-nil Cfg")
	}
}

// ---- runtimeExecutableLegacy (via ResolveRuntimePath with empty config) ----

func TestResolveRuntimePath_LegacyFallback(t *testing.T) {
	// 使用空配置 (无 runtimes 定义) 触发 runtimeExecutableLegacy 兜底逻辑
	emptyCfg := &config.Config{}
	for _, rt := range []string{"node", "java", "ruby"} {
		// 这些运行时在系统 PATH 中可能存在也可能不存在，仅验证不 panic
		_, _ = appcommon.ResolveRuntimePath(emptyCfg, rt)
	}
	// python 分支: 会调用 exec.LookPath("python3")
	_, _ = appcommon.ResolveRuntimePath(emptyCfg, "python")
	// 未知运行时: 返回 rt 自身作为兜底
	_, _ = appcommon.ResolveRuntimePath(emptyCfg, "unknown-runtime-xyz")
}

// ---- installViaScript (via InstallBinary with runtime tool) ----

func TestInstallBinary_RuntimeTool(t *testing.T) {
	// 注册一个带 Runtime 的工具，InstallBinary 会委托给 installViaScript
	// installViaScript 调用 scripts/install-bin.sh --tool=<name>
	// 使用不存在的工具名，脚本应失败但不会 panic
	setupCRUDTest(t)
	meta := &binary.BinaryMeta{
		Name:      "test-runtime-tool-xyz",
		Version:   "1.0.0",
		Runtime:   binary.RuntimeNode,
		Source:    binary.SourceInstall,
		Executable: "test-runtime-tool-xyz",
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-runtime-tool-xyz") })

	result := testCore.InstallBinary("test-runtime-tool-xyz")
	// 安装脚本对未知工具应返回失败 (success=false)，但不应 panic
	_ = result
}

// ---- parseRuntimeDef / parseToolDef error paths (via CRUD with bad data) ----

func TestCRUD_AddRuntime_UnmarshalableData(t *testing.T) {
	setupCRUDTest(t)
	// 传入无法序列化的数据 (通道) 触发 json.Marshal 错误
	err := testCore.AddRuntime(map[string]interface{}{
		"name": "bad-rt",
		"bad":  make(chan int), // 通道无法被 JSON 序列化
	})
	if err == nil {
		t.Error("expected error for unmarshalable runtime data")
	}
}

func TestCRUD_AddTool_UnmarshalableData(t *testing.T) {
	setupCRUDTest(t)
	err := testCore.AddTool(map[string]interface{}{
		"name": "bad-tool",
		"bad":  make(chan int),
	})
	if err == nil {
		t.Error("expected error for unmarshalable tool data")
	}
}

// ---- NewManager auto-detection paths ----

func TestNewManager_AutoDetection(t *testing.T) {
	// 清除环境变量，触发自动检测逻辑
	orig := os.Getenv("FORMATTER_BIN_HOME")
	os.Unsetenv("FORMATTER_BIN_HOME")
	t.Cleanup(func() {
		if orig != "" {
			os.Setenv("FORMATTER_BIN_HOME", orig)
		}
	})

	m := binary.NewManager("")
	if m == nil {
		t.Fatal("expected non-nil Manager")
	}
	if m.InstallDir == "" {
		t.Error("expected non-empty InstallDir after auto-detection")
	}
}

func TestNewManager_ExplicitDir(t *testing.T) {
	tmpDir := t.TempDir()
	m := binary.NewManager(tmpDir)
	if m.InstallDir != tmpDir {
		t.Errorf("expected InstallDir=%q, got %q", tmpDir, m.InstallDir)
	}
}

// ---- StdinReader.Read ----

func TestStdinReader_Read_DevStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/dev/stdin not available on Windows")
	}
	// 在测试环境中 stdin 通常连接到 /dev/null，Read 应返回空数据或错误
	// 此测试主要覆盖 Read 代码路径 (os.ReadFile("/dev/stdin"))
	r := iocore.NewStdinReader()
	data, lang, err := r.Read()
	// 接受任何结果 (空数据、错误均可)，只要不 panic 即可
	_ = data
	_ = lang
	_ = err
}

// ---- LocalConfigPath / Save fallback paths ----

func TestLocalConfigPath_DevMode(t *testing.T) {
	// 在开发模式下 (cwd=项目根), data/config.json 存在
	path := config.LocalConfigPath()
	if path == "" {
		t.Error("expected non-empty config path")
	}
}

func TestSave_WriteToCustomPath(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Default()
	customPath := filepath.Join(tmpDir, "custom-config.json")
	if err := config.Save(cfg, customPath); err != nil {
		t.Fatalf("Save to custom path failed: %v", err)
	}
	if _, err := os.Stat(customPath); err != nil {
		t.Errorf("expected file at %s: %v", customPath, err)
	}
}

func TestSave_EmptyPath_WritableDevDir(t *testing.T) {
	// 在开发模式下 (data/ 可写), Save 应写入 data/config.json
	// 使用备份/恢复避免污染真实配置
	origConfig, err := os.ReadFile("data/config.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile("data/config.json", origConfig, 0644)
	})
	cfg := config.Default()
	if err := config.Save(cfg, ""); err != nil {
		t.Fatalf("Save with empty path failed: %v", err)
	}
}
