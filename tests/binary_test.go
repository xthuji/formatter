package tests

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/formatter/formatter/src/binary"
	"github.com/formatter/formatter/src/config"
)

func TestNewManager(t *testing.T) {
	tmpDir := t.TempDir()
	m := binary.NewManager(tmpDir)
	if m == nil {
		t.Fatal("expected non-nil Manager")
	}
	if m.InstallDir != tmpDir {
		t.Errorf("expected InstallDir = %q, got %q", tmpDir, m.InstallDir)
	}
}

func TestManager_ListInstalled_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	m := binary.NewManager(tmpDir)

	installed, err := m.ListInstalled()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(installed) != 0 {
		t.Errorf("expected empty installed list, got %v", installed)
	}
}

func TestManager_ListInstalled_NonExistentDir(t *testing.T) {
	m := binary.NewManager("/nonexistent/path/that/does/not/exist")

	installed, err := m.ListInstalled()
	if err != nil {
		t.Fatalf("expected no error for non-existent dir, got: %v", err)
	}
	if installed != nil {
		t.Errorf("expected nil slice, got %v", installed)
	}
}

func TestManager_ListInstalled_WithInstalledTools(t *testing.T) {
	tmpDir := t.TempDir()

	oxfmtPath := filepath.Join(tmpDir, "oxfmt")
	if err := os.WriteFile(oxfmtPath, []byte("#!/bin/sh\necho oxfmt"), 0755); err != nil {
		t.Fatalf("failed to create oxfmt: %v", err)
	}

	m := binary.NewManager(tmpDir)
	installed, err := m.ListInstalled()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(installed) == 0 {
		t.Fatal("expected at least one installed tool")
	}

	found := false
	for _, name := range installed {
		if name == "oxfmt" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'oxfmt' in installed list, got %v", installed)
	}
}

func TestManager_FindBinary_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	m := binary.NewManager(tmpDir)

	_, err := m.FindBinary("nonexistent-tool")
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
}

func TestManager_FindBinary_NotExecutable(t *testing.T) {
	tmpDir := t.TempDir()

	meta, err := binary.Get("oxfmt")
	if err != nil {
		t.Skipf("oxfmt not in registry: %v", err)
		return
	}

	exePath := filepath.Join(tmpDir, meta.Executable)
	if err := os.WriteFile(exePath, []byte("not a real binary"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	m := binary.NewManager(tmpDir)
	_, err = m.FindBinary("oxfmt")
	if err == nil {
		t.Fatal("expected error for non-executable binary")
	}
}

func TestManager_FindBinary_Success(t *testing.T) {
	tmpDir := t.TempDir()

	meta, err := binary.Get("oxfmt")
	if err != nil {
		t.Skipf("oxfmt not in registry: %v", err)
		return
	}

	exePath := filepath.Join(tmpDir, meta.Executable)
	if err := os.WriteFile(exePath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	m := binary.NewManager(tmpDir)
	foundPath, err := m.FindBinary("oxfmt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if foundPath != exePath {
		t.Errorf("expected %q, got %q", exePath, foundPath)
	}
}

// TestManager_FindBinary_WithRuntime 验证依赖运行时的工具 (如 google-java-format JAR)
// 这类工具由运行时（如 java）加载执行，因此不要求文件本身有可执行权限
func TestManager_FindBinary_WithRuntime(t *testing.T) {
	tmpDir := t.TempDir()

	meta, err := binary.Get("google-java-format")
	if err != nil {
		t.Skipf("google-java-format not in registry: %v", err)
		return
	}

	// 验证 google-java-format 配置为 download 且指定了 runtime
	if meta.Source != binary.SourceDownload {
		t.Errorf("expected Source = download, got %q", meta.Source)
	}
	if meta.Runtime == "" {
		t.Error("expected non-empty Runtime for google-java-format")
	}
	if meta.RunCmd == "" {
		t.Error("expected non-empty RunCmd for google-java-format")
	}
	if !meta.ShouldBundle() {
		t.Error("expected ShouldBundle() = true for downloadable tools")
	}

	// 创建 JAR 文件 (不需要可执行权限，因为通过 java -jar 运行)
	jarPath := filepath.Join(tmpDir, meta.Executable)
	if err := os.WriteFile(jarPath, []byte("fake jar content"), 0644); err != nil {
		t.Fatalf("failed to create jar: %v", err)
	}

	// FindBinary 对于有 Runtime 的工具应跳过可执行权限检查
	m := binary.NewManager(tmpDir)
	foundPath, err := m.FindBinary("google-java-format")
	if err != nil {
		t.Fatalf("should find JAR without executable permission: %v", err)
	}
	if foundPath != jarPath {
		t.Errorf("expected %q, got %q", jarPath, foundPath)
	}

	// 验证移除逻辑
	if err := m.RemoveBinary("google-java-format"); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if _, err := os.Stat(jarPath); !os.IsNotExist(err) {
		t.Error("JAR file should be removed")
	}
}

func TestManager_RemoveBinary_Success(t *testing.T) {
	tmpDir := t.TempDir()

	meta, err := binary.Get("oxfmt")
	if err != nil {
		t.Skipf("oxfmt not in registry: %v", err)
		return
	}

	exePath := filepath.Join(tmpDir, meta.Executable)
	if err := os.WriteFile(exePath, []byte("test"), 0755); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	m := binary.NewManager(tmpDir)
	if err := m.RemoveBinary("oxfmt"); err != nil {
		t.Fatalf("unexpected error removing binary: %v", err)
	}

	if _, err := os.Stat(exePath); !os.IsNotExist(err) {
		t.Error("expected binary file to be removed")
	}
}

func TestManager_RemoveBinary_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	m := binary.NewManager(tmpDir)

	err := m.RemoveBinary("nonexistent-tool")
	if err == nil {
		t.Fatal("expected error for removing nonexistent binary")
	}
}

func TestGlobalRegistry_Get(t *testing.T) {
	for _, tool := range config.Default().Binary.Tools {
		name := tool.Name
		t.Run(name, func(t *testing.T) {
			meta, err := binary.Get(name)
			if err != nil {
				t.Fatalf("Get(%q) error: %v", name, err)
			}
			if meta == nil {
				t.Fatalf("Get(%q) returned nil", name)
			}
			if meta.Name != name {
				t.Errorf("expected Name = %q, got %q", name, meta.Name)
			}
			if meta.Version == "" {
				t.Errorf("expected non-empty Version for %q", name)
			}
			if meta.Executable == "" {
				t.Errorf("expected non-empty Executable for %q", name)
			}
			if len(meta.Languages) == 0 {
				t.Errorf("expected non-empty Languages for %q", name)
			}
		})
	}
}

func TestGlobalRegistry_Get_NotFound(t *testing.T) {
	_, err := binary.Get("nonexistent-tool-xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
}

func TestGlobalRegistry_MustGet_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nonexistent tool")
		}
	}()
	binary.MustGet("nonexistent-tool-xyz")
}

func TestGlobalRegistry_MustGet_Success(t *testing.T) {
	meta := binary.MustGet("oxfmt")
	if meta == nil {
		t.Fatal("expected non-nil meta for oxfmt")
	}
}

func TestGlobalRegistry_List(t *testing.T) {
	list := binary.List()
	if len(list) == 0 {
		t.Error("expected non-empty registry list")
	}

	for _, tool := range config.Default().Binary.Tools {
		name := tool.Name
		found := false
		for _, n := range list {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in registry list", name)
		}
	}
}

// TestToolSourceClassification 验证三类工具架构的分类 (依据 config 中各工具的 Source 字段):
// - download: 下载工具 (打包进 App)
// - preset: 预置工具 (打包进 App)
// - install: 安装工具 (不打包，依赖运行时环境)
func TestToolSourceClassification(t *testing.T) {
	// 从配置构建分类映射 (依据 Source 字段)
	presetTools := map[string]bool{}
	downloadTools := map[string]bool{}
	installTools := map[string]bool{}
	for _, tool := range config.Default().Binary.Tools {
		switch binary.ToolSource(tool.Source) {
		case binary.SourcePreset:
			presetTools[tool.Name] = true
		case binary.SourceDownload:
			downloadTools[tool.Name] = true
		case binary.SourceInstall:
			installTools[tool.Name] = true
		}
	}

	list := binary.List()
	for _, name := range list {
		meta, err := binary.Get(name)
		if err != nil {
			t.Fatalf("Get(%q) error: %v", name, err)
		}

		if presetTools[name] {
			if meta.Source != binary.SourcePreset {
				t.Errorf("tool %q expected source=preset, got %q", name, meta.Source)
			}
			if meta.ShouldBundle() {
				t.Errorf("preset tool %q should not bundle (ShouldBundle should be false)", name)
			}
			if !meta.IsShipped() {
				t.Errorf("preset tool %q should be shipped (IsShipped should be true)", name)
			}
		} else if downloadTools[name] {
			if meta.Source != binary.SourceDownload {
				t.Errorf("tool %q expected source=download, got %q", name, meta.Source)
			}
			if !meta.ShouldBundle() {
				t.Errorf("download tool %q should bundle (ShouldBundle should be true)", name)
			}
			if !meta.IsShipped() {
				t.Errorf("download tool %q should be shipped (IsShipped should be true)", name)
			}
		} else if installTools[name] {
			if meta.Source != binary.SourceInstall {
				t.Errorf("tool %q expected source=install, got %q", name, meta.Source)
			}
			if meta.ShouldBundle() {
				t.Errorf("install tool %q should not bundle (ShouldBundle should be false)", name)
			}
			if meta.IsShipped() {
				t.Errorf("install tool %q should not be shipped (IsShipped should be false)", name)
			}
			if meta.InstallCmds == nil {
				t.Errorf("install tool %q should have InstallCmds", name)
			}
		} else {
			t.Errorf("unexpected tool %q in registry (not in preset/download/install classification)", name)
		}
	}

	// 验证工具总数与配置一致
	if len(list) != len(config.Default().Binary.Tools) {
		t.Errorf("expected %d tools in registry, got %d", len(config.Default().Binary.Tools), len(list))
	}
}

// TestToolRuntimeClassification 验证工具运行时分类:
// - 独立二进制 (runtime=""): 大部分工具
// - 依赖运行时 (runtime="java"): google-java-format
func TestToolRuntimeClassification(t *testing.T) {
	meta, err := binary.Get("google-java-format")
	if err != nil {
		t.Fatalf("Get(google-java-format) error: %v", err)
	}
	if meta.Runtime != "java" {
		t.Errorf("expected google-java-format runtime='java', got %q", meta.Runtime)
	}
	if meta.RunCmd == "" {
		t.Error("expected non-empty RunCmd for google-java-format")
	}

	// 从配置构建独立二进制工具列表 (Runtime == "" 表示独立二进制)
	standaloneTools := []string{}
	for _, tool := range config.Default().Binary.Tools {
		if tool.Runtime == "" {
			standaloneTools = append(standaloneTools, tool.Name)
		}
	}
	for _, name := range standaloneTools {
		meta, err := binary.Get(name)
		if err != nil {
			t.Fatalf("Get(%q) error: %v", name, err)
		}
		if meta.Runtime != "" {
			t.Errorf("tool %q expected empty runtime, got %q", name, meta.Runtime)
		}
	}
}

func TestGlobalRegistry_Register(t *testing.T) {
	meta := &binary.BinaryMeta{
		Name:        "test-custom-tool",
		Version:     "1.0.0",
		URLs:        map[string]string{"darwin/amd64": "https://example.com/tool-{version}"},
		Languages:   []string{"text"},
		ArchiveType: binary.ArchiveTypeRaw,
		Executable:  "test-tool",
		VerifyCmd:   "{exe} --version",
		Source:      binary.SourceDownload,
	}

	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-custom-tool") })

	got, err := binary.Get("test-custom-tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "test-custom-tool" {
		t.Errorf("expected Name = 'test-custom-tool', got %q", got.Name)
	}
	if got.Version != "1.0.0" {
		t.Errorf("expected Version = '1.0.0', got %q", got.Version)
	}
}

func TestArchiveType_Constants(t *testing.T) {
	if binary.ArchiveTypeTarGZ != "tar.gz" {
		t.Errorf("expected ArchiveTypeTarGZ = 'tar.gz', got %q", binary.ArchiveTypeTarGZ)
	}
	if binary.ArchiveTypeTarXZ != "tar.xz" {
		t.Errorf("expected ArchiveTypeTarXZ = 'tar.xz', got %q", binary.ArchiveTypeTarXZ)
	}
	if binary.ArchiveTypeZip != "zip" {
		t.Errorf("expected ArchiveTypeZip = 'zip', got %q", binary.ArchiveTypeZip)
	}
	if binary.ArchiveTypeRaw != "raw" {
		t.Errorf("expected ArchiveTypeRaw = 'raw', got %q", binary.ArchiveTypeRaw)
	}
}

func TestPlatformURL(t *testing.T) {
	meta := &binary.BinaryMeta{
		Name:    "test-tool",
		Version: "1.0.0",
		URLs: map[string]string{
			"darwin/amd64":  "https://example.com/test-{version}-darwin-x64",
			"darwin/arm64":  "https://example.com/test-{version}-darwin-arm64",
			"linux/amd64":   "https://example.com/test-{version}-linux-x64",
			"windows/amd64": "https://example.com/test-{version}-win-x64.exe",
			"*":             "https://example.com/test-{version}-all.jar",
		},
	}

	tests := []struct {
		goos, goarch, want string
	}{
		{"darwin", "amd64", "https://example.com/test-1.0.0-darwin-x64"},
		{"darwin", "arm64", "https://example.com/test-1.0.0-darwin-arm64"},
		{"linux", "amd64", "https://example.com/test-1.0.0-linux-x64"},
		{"windows", "amd64", "https://example.com/test-1.0.0-win-x64.exe"},
		{"linux", "arm64", "https://example.com/test-1.0.0-all.jar"}, // 未配置的平台回退到 "*"
	}
	for _, tt := range tests {
		got := meta.PlatformURL(tt.goos, tt.goarch)
		if got != tt.want {
			t.Errorf("PlatformURL(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
		}
	}
}

func TestPlatformURL_OSWildcard(t *testing.T) {
	// 测试 "os/*" 通配符优先级高于 "*"
	meta := &binary.BinaryMeta{
		Name:    "test-jar",
		Version: "2.0",
		URLs: map[string]string{
			"darwin/*": "https://example.com/test-{version}-darwin.sh",
			"linux/*":  "https://example.com/test-{version}-linux.sh",
			"*":        "https://example.com/test-{version}-all.jar",
		},
	}

	tests := []struct {
		goos, goarch, want string
	}{
		{"darwin", "amd64", "https://example.com/test-2.0-darwin.sh"},
		{"darwin", "arm64", "https://example.com/test-2.0-darwin.sh"},
		{"linux", "arm64", "https://example.com/test-2.0-linux.sh"},
		{"windows", "amd64", "https://example.com/test-2.0-all.jar"},
	}
	for _, tt := range tests {
		got := meta.PlatformURL(tt.goos, tt.goarch)
		if got != tt.want {
			t.Errorf("PlatformURL(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
		}
	}
}

// TestManager_FindBinary_CustomPath 验证当 meta.Path 设置时,
// FindBinary 直接使用用户指定路径 (仅验证文件存在，不强制可执行权限)
func TestManager_FindBinary_CustomPath(t *testing.T) {
	tmpDir := t.TempDir()
	// 创建一个不可执行的文件 (mode 0644)
	customExe := filepath.Join(tmpDir, "my-oxfmt")
	if err := os.WriteFile(customExe, []byte("#!/bin/sh\necho test"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	meta := &binary.BinaryMeta{
		Name:       "test-custom-path-tool",
		Version:    "1.0.0",
		Languages:  []string{"text"},
		Path:       customExe, // 用户直接指定路径
		Executable: "unused-executable",
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-custom-path-tool") })

	m := binary.NewManager(t.TempDir()) // InstallDir 不应被使用
	foundPath, err := m.FindBinary("test-custom-path-tool")
	if err != nil {
		t.Fatalf("custom path should be found without executable permission check: %v", err)
	}
	if foundPath != customExe {
		t.Errorf("expected %q, got %q", customExe, foundPath)
	}
}

// TestManager_FindBinary_CustomPath_NotExist 验证当 meta.Path 指向不存在的文件时,
// FindBinary 返回错误 (且 EnsureBinary 不会尝试下载)
func TestManager_FindBinary_CustomPath_NotExist(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "does-not-exist")

	meta := &binary.BinaryMeta{
		Name:       "test-missing-path-tool",
		Version:    "1.0.0",
		Languages:  []string{"text"},
		Path:       missingPath,
		Executable: "unused",
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-missing-path-tool") })

	m := binary.NewManager(t.TempDir())
	_, err := m.FindBinary("test-missing-path-tool")
	if err == nil {
		t.Fatal("expected error for non-existent custom path")
	}

	// EnsureBinary 也应失败，且不触发下载 (因为 Path 已设置)
	_, err = m.EnsureBinary("test-missing-path-tool")
	if err == nil {
		t.Fatal("EnsureBinary should fail without downloading when custom path is invalid")
	}
}

// TestBuildToolCmd_StandaloneBinary 验证无运行时依赖的独立二进制命令构建
// 期望: cmdPath = exePath, cmdArgs = 传入的 args
func TestBuildToolCmd_StandaloneBinary(t *testing.T) {
	tmpDir := t.TempDir()

	meta := &binary.BinaryMeta{
		Name:       "test-standalone-tool",
		Version:    "1.0.0",
		Languages:  []string{"text"},
		Executable: "test-exe",
		Source:     binary.SourceDownload,
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-standalone-tool") })

	// 创建可执行文件
	exePath := filepath.Join(tmpDir, "test-exe")
	if err := os.WriteFile(exePath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("failed to create exe: %v", err)
	}

	m := binary.NewManager(tmpDir)
	cmdPath, cmdArgs, err := m.BuildToolCmd("test-standalone-tool", "--format", "-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmdPath != exePath {
		t.Errorf("expected cmdPath = %q, got %q", exePath, cmdPath)
	}
	if len(cmdArgs) != 2 || cmdArgs[0] != "--format" || cmdArgs[1] != "-" {
		t.Errorf("expected args [--format -], got %v", cmdArgs)
	}
}

// TestBuildToolCmd_WithRuntime_DefaultMode 验证有运行时依赖但无 RunCmd 时的默认模式
// 期望: cmdPath = runtimePath, cmdArgs = [exePath, args...]
// 使用 /bin/echo 作为模拟运行时 (所有 Unix 系统都存在)
func TestBuildToolCmd_WithRuntime_DefaultMode(t *testing.T) {
	tmpDir := t.TempDir()

	// 使用 /bin/echo 作为模拟运行时 (跨平台可用)
	echoPath := "/bin/echo"
	if _, err := os.Stat(echoPath); err != nil {
		t.Skipf("/bin/echo not available: %v", err)
		return
	}

	meta := &binary.BinaryMeta{
		Name:       "test-runtime-default-tool",
		Version:    "1.0.0",
		Languages:  []string{"text"},
		Executable: "script.js",
		Runtime:    binary.RuntimeNode,
		Source:     binary.SourceDownload,
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-runtime-default-tool") })

	// 创建脚本文件 (JAR/脚本不需要可执行权限)
	scriptPath := filepath.Join(tmpDir, "script.js")
	if err := os.WriteFile(scriptPath, []byte("console.log('test')"), 0644); err != nil {
		t.Fatalf("failed to create script: %v", err)
	}

	// 配置运行时路径指向 /bin/echo
	cfg := &config.Config{
		Binary: config.BinaryConfig{
			Runtimes: []config.RuntimeDef{
				{Name: "node", Exe: "node", Path: echoPath},
			},
		},
	}
	m := binary.NewManagerWithConfig(tmpDir, cfg)

	cmdPath, cmdArgs, err := m.BuildToolCmd("test-runtime-default-tool", "--arg1", "value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmdPath != echoPath {
		t.Errorf("expected cmdPath = %q (runtime), got %q", echoPath, cmdPath)
	}
	// 默认模式: [exePath, --arg1, value]
	if len(cmdArgs) != 3 {
		t.Fatalf("expected 3 args, got %d: %v", len(cmdArgs), cmdArgs)
	}
	if cmdArgs[0] != scriptPath {
		t.Errorf("expected first arg = script path %q, got %q", scriptPath, cmdArgs[0])
	}
	if cmdArgs[1] != "--arg1" || cmdArgs[2] != "value" {
		t.Errorf("unexpected trailing args: %v", cmdArgs[1:])
	}
}

// TestBuildToolCmd_WithRuntime_CustomRunCmd 验证自定义 RunCmd 模板模式
// 期望: 解析 RunCmd 模板，{runtime}/{exe} 替换后与 args 拼接
func TestBuildToolCmd_WithRuntime_CustomRunCmd(t *testing.T) {
	tmpDir := t.TempDir()

	echoPath := "/bin/echo"
	if _, err := os.Stat(echoPath); err != nil {
		t.Skipf("/bin/echo not available: %v", err)
		return
	}

	meta := &binary.BinaryMeta{
		Name:       "test-runtime-runcmd-tool",
		Version:    "1.0.0",
		Languages:  []string{"text"},
		Executable: "app.jar",
		Runtime:    binary.RuntimeJava,
		RunCmd:     "{runtime} -jar {exe}",
		Source:     binary.SourceDownload,
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-runtime-runcmd-tool") })

	jarPath := filepath.Join(tmpDir, "app.jar")
	if err := os.WriteFile(jarPath, []byte("fake jar"), 0644); err != nil {
		t.Fatalf("failed to create jar: %v", err)
	}

	cfg := &config.Config{
		Binary: config.BinaryConfig{
			Runtimes: []config.RuntimeDef{
				{Name: "java", Exe: "java", Path: echoPath},
			},
		},
	}
	m := binary.NewManagerWithConfig(tmpDir, cfg)

	cmdPath, cmdArgs, err := m.BuildToolCmd("test-runtime-runcmd-tool", "--flag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmdPath != echoPath {
		t.Errorf("expected cmdPath = %q (runtime), got %q", echoPath, cmdPath)
	}
	// RunCmd: {runtime} -jar {exe} → [echo, -jar, jarPath]
	// 加上 args: [echo, -jar, jarPath, --flag]
	if len(cmdArgs) != 3 {
		t.Fatalf("expected 3 args (-jar, jarPath, --flag), got %d: %v", len(cmdArgs), cmdArgs)
	}
	if cmdArgs[0] != "-jar" {
		t.Errorf("expected first arg = '-jar', got %q", cmdArgs[0])
	}
	if cmdArgs[1] != jarPath {
		t.Errorf("expected second arg = jar path %q, got %q", jarPath, cmdArgs[1])
	}
	if cmdArgs[2] != "--flag" {
		t.Errorf("expected third arg = '--flag', got %q", cmdArgs[2])
	}
}

// TestBuildToolCmd_RuntimeNotFound 验证运行时不可用时返回错误
func TestBuildToolCmd_RuntimeNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	meta := &binary.BinaryMeta{
		Name:       "test-runtime-missing-tool",
		Version:    "1.0.0",
		Languages:  []string{"text"},
		Executable: "script.js",
		Runtime:    binary.RuntimeNode,
		Source:     binary.SourceDownload,
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-runtime-missing-tool") })

	scriptPath := filepath.Join(tmpDir, "script.js")
	if err := os.WriteFile(scriptPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create script: %v", err)
	}

	// 配置中不提供运行时路径，且使用一个不存在的运行时名
	cfg := &config.Config{
		Binary: config.BinaryConfig{
			Runtimes: []config.RuntimeDef{
				{Name: "node", Exe: "node-nonexistent-xyz"},
			},
		},
	}
	m := binary.NewManagerWithConfig(tmpDir, cfg)

	_, _, err := m.BuildToolCmd("test-runtime-missing-tool", "--arg")
	if err == nil {
		t.Fatal("expected error when runtime not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// ---- Manager 下载/验证/安装 ----

// ---- VerifyBinary ----

func TestManager_VerifyBinary_NotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	m := binary.NewManager(tmpDir)
	_, err := m.VerifyBinary("oxfmt")
	if err == nil {
		t.Error("expected error for not installed binary")
	}
}

func TestManager_VerifyBinary_Success(t *testing.T) {
	tmpDir := t.TempDir()

	meta, err := binary.Get("oxfmt")
	if err != nil {
		t.Skipf("oxfmt not in registry: %v", err)
	}

	// 创建一个假的可执行文件，响应 --version
	exePath := filepath.Join(tmpDir, meta.Executable)
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo \"1.0.0\"; fi"
	if err := os.WriteFile(exePath, []byte(script), 0755); err != nil {
		t.Fatalf("create fake binary: %v", err)
	}

	m := binary.NewManager(tmpDir)
	version, err := m.VerifyBinary("oxfmt")
	if err != nil {
		t.Fatalf("VerifyBinary failed: %v", err)
	}
	if version == "" {
		t.Error("expected non-empty version")
	}
}

// ---- Download (with local HTTP server) ----

func TestDownload_Success(t *testing.T) {
	content := []byte("test binary content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write(content)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "downloaded.bin")
	err := binary.Download(server.URL, destPath)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("expected %q, got %q", string(content), string(data))
	}
}

func TestDownload_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "failed.bin")
	err := binary.Download(server.URL, destPath)
	if err == nil {
		t.Error("expected error for HTTP 404")
	}
}

func TestDownload_InvalidURL(t *testing.T) {
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "failed.bin")
	// 使用一个无效的 URL，应该连接失败
	err := binary.Download("http://127.0.0.1:1/file", destPath)
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

// ---- EnsureBinary with preset source ----

func TestManager_EnsureBinary_PresetNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	m := binary.NewManager(tmpDir)

	// 注册一个 preset 工具 (不在安装目录中)
	meta := &binary.BinaryMeta{
		Name:       "test-preset-tool",
		Version:    "1.0.0",
		Executable: "test-preset",
		Source:     binary.SourcePreset,
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-preset-tool") })

	_, err := m.EnsureBinary("test-preset-tool")
	if err == nil {
		t.Error("expected error for preset binary not found")
	}
}

// ---- DownloadBinary with install source ----

func TestManager_DownloadBinary_InstallNoCmd(t *testing.T) {
	tmpDir := t.TempDir()
	m := binary.NewManager(tmpDir)

	meta := &binary.BinaryMeta{
		Name:       "test-install-tool",
		Version:    "1.0.0",
		Executable: "test-install",
		Source:     binary.SourceInstall,
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-install-tool") })

	err := m.DownloadBinary("test-install-tool")
	if err == nil {
		t.Error("expected error for install tool without install commands")
	}
}

// ---- DownloadBinary with download source (using local HTTP server) ----

func TestManager_DownloadBinary_DownloadSuccess(t *testing.T) {
	content := []byte("#!/bin/sh\necho test")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write(content)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	m := binary.NewManager(tmpDir)

	meta := &binary.BinaryMeta{
		Name:        "test-download-tool",
		Version:     "1.0.0",
		URLs:        map[string]string{"*": server.URL},
		Executable:  "test-download",
		ArchiveType: binary.ArchiveTypeRaw,
		Source:      binary.SourceDownload,
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-download-tool") })

	err := m.DownloadBinary("test-download-tool")
	if err != nil {
		t.Fatalf("DownloadBinary failed: %v", err)
	}

	// 验证文件已下载
	exePath := filepath.Join(tmpDir, meta.Executable)
	if _, err := os.Stat(exePath); err != nil {
		t.Errorf("expected file at %s: %v", exePath, err)
	}
}

// ---- ListEmbedded ----

func TestListEmbedded(t *testing.T) {
	// ListEmbedded 返回预置工具列表
	list := binary.ListEmbedded()
	// 可能返回空列表 (如果没有 preset 工具)，但不应 panic
	for _, name := range list {
		meta, err := binary.Get(name)
		if err != nil {
			t.Errorf("ListEmbedded returned %q but Get failed: %v", name, err)
		}
		if !meta.IsShipped() {
			t.Errorf("ListEmbedded returned %q but IsShipped=false", name)
		}
	}
}

// ---- InstallRuntime (manager) ----

func TestManager_InstallRuntime_Success(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Binary: config.BinaryConfig{
			Runtimes: []config.RuntimeDef{
				{
					Name: "testrt",
					InstallCmds: map[string][]string{
						runtime.GOOS: {"echo", "install-ok"},
					},
				},
			},
		},
	}
	m := binary.NewManagerWithConfig(tmpDir, cfg)
	out, err := m.InstallRuntime("testrt")
	if err != nil {
		t.Fatalf("InstallRuntime failed: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestManager_InstallRuntime_NoConfig(t *testing.T) {
	tmpDir := t.TempDir()
	// NewManager (无 cfg) 创建的 Manager，InstallRuntime 应返回错误
	m := binary.NewManager(tmpDir)
	_, err := m.InstallRuntime("node")
	if err == nil {
		t.Error("expected error when manager config is nil")
	}
}

func TestManager_InstallRuntime_NotDefined(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{}
	m := binary.NewManagerWithConfig(tmpDir, cfg)
	_, err := m.InstallRuntime("nonexistent-rt")
	if err == nil {
		t.Error("expected error for runtime not defined in config")
	}
}

func TestManager_InstallRuntime_NoInstallCmd(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Binary: config.BinaryConfig{
			Runtimes: []config.RuntimeDef{
				{Name: "noruntime"},
			},
		},
	}
	m := binary.NewManagerWithConfig(tmpDir, cfg)
	_, err := m.InstallRuntime("noruntime")
	if err == nil {
		t.Error("expected error for runtime without install command")
	}
}

func TestManager_InstallRuntime_CmdFailure(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Binary: config.BinaryConfig{
			Runtimes: []config.RuntimeDef{
				{
					Name: "failrt",
					InstallCmds: map[string][]string{
						runtime.GOOS: {"false"},
					},
				},
			},
		},
	}
	m := binary.NewManagerWithConfig(tmpDir, cfg)
	_, err := m.InstallRuntime("failrt")
	if err == nil {
		t.Error("expected error for failing install command")
	}
}

// ---- installFromCommand (source=install) ----

func TestManager_DownloadBinary_InstallCmdSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	m := binary.NewManager(tmpDir)

	meta := &binary.BinaryMeta{
		Name:       "test-install-success",
		Version:    "1.0.0",
		Executable: "test-install-success",
		Source:     binary.SourceInstall,
		InstallCmds: map[string][]string{
			runtime.GOOS: {"echo", "done"},
		},
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-install-success") })

	err := m.DownloadBinary("test-install-success")
	if err != nil {
		t.Fatalf("DownloadBinary with install source failed: %v", err)
	}
}

func TestManager_DownloadBinary_InstallCmdFailure(t *testing.T) {
	tmpDir := t.TempDir()
	m := binary.NewManager(tmpDir)

	meta := &binary.BinaryMeta{
		Name:       "test-install-fail",
		Version:    "1.0.0",
		Executable: "test-install-fail",
		Source:     binary.SourceInstall,
		InstallCmds: map[string][]string{
			runtime.GOOS: {"false"},
		},
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-install-fail") })

	err := m.DownloadBinary("test-install-fail")
	if err == nil {
		t.Error("expected error for failing install command")
	}
}

// ---- downloadFromURL archive paths (findAndExtractExecutable) ----
// 注: 归档创建辅助函数 createTarGZ/createZip 定义在下方 Extractor 测试段中，
// 本文件通过同包共享使用，不再重复定义。

func TestManager_DownloadBinary_TarGZArchive(t *testing.T) {
	// 创建一个 tar.gz 归档，包含一个可执行文件
	exeContent := "#!/bin/sh\necho hello"
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "src.tar.gz")
	createTarGZ(t, archivePath, map[string]string{
		"mytool": exeContent,
	})
	archiveData, _ := os.ReadFile(archivePath)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(archiveData)))
		w.Write(archiveData)
	}))
	defer server.Close()

	installDir := t.TempDir()
	m := binary.NewManager(installDir)

	meta := &binary.BinaryMeta{
		Name:        "test-targz-tool",
		Version:     "1.0.0",
		URLs:        map[string]string{"*": server.URL},
		Executable:  "mytool",
		ArchiveType: binary.ArchiveTypeTarGZ,
		Source:      binary.SourceDownload,
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-targz-tool") })

	if err := m.DownloadBinary("test-targz-tool"); err != nil {
		t.Fatalf("DownloadBinary tar.gz failed: %v", err)
	}
	// 验证可执行文件已安装
	installedPath := filepath.Join(installDir, "mytool")
	data, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatalf("read installed file: %v", err)
	}
	if string(data) != exeContent {
		t.Errorf("expected %q, got %q", exeContent, string(data))
	}
}

func TestManager_DownloadBinary_ZipArchive(t *testing.T) {
	exeContent := "#!/bin/sh\necho zip"
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "src.zip")
	createZip(t, archivePath, map[string]string{
		"ziptool": exeContent,
	})
	archiveData, _ := os.ReadFile(archivePath)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(archiveData)))
		w.Write(archiveData)
	}))
	defer server.Close()

	installDir := t.TempDir()
	m := binary.NewManager(installDir)

	meta := &binary.BinaryMeta{
		Name:        "test-zip-tool",
		Version:     "1.0.0",
		URLs:        map[string]string{"*": server.URL},
		Executable:  "ziptool",
		ArchiveType: binary.ArchiveTypeZip,
		Source:      binary.SourceDownload,
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-zip-tool") })

	if err := m.DownloadBinary("test-zip-tool"); err != nil {
		t.Fatalf("DownloadBinary zip failed: %v", err)
	}
	installedPath := filepath.Join(installDir, "ziptool")
	if _, err := os.Stat(installedPath); err != nil {
		t.Errorf("expected file at %s: %v", installedPath, err)
	}
}

func TestManager_DownloadBinary_TarGZExeNotFound(t *testing.T) {
	// 归档中不包含期望的可执行文件
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "src.tar.gz")
	createTarGZ(t, archivePath, map[string]string{
		"other-file": "content",
	})
	archiveData, _ := os.ReadFile(archivePath)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(archiveData)))
		w.Write(archiveData)
	}))
	defer server.Close()

	installDir := t.TempDir()
	m := binary.NewManager(installDir)

	meta := &binary.BinaryMeta{
		Name:        "test-targz-missing",
		Version:     "1.0.0",
		URLs:        map[string]string{"*": server.URL},
		Executable:  "nonexistent-exe",
		ArchiveType: binary.ArchiveTypeTarGZ,
		Source:      binary.SourceDownload,
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-targz-missing") })

	err := m.DownloadBinary("test-targz-missing")
	if err == nil {
		t.Error("expected error when executable not found in archive")
	}
}

// ---- downloadFromURL error paths ----

func TestManager_DownloadBinary_NoURL(t *testing.T) {
	tmpDir := t.TempDir()
	m := binary.NewManager(tmpDir)

	meta := &binary.BinaryMeta{
		Name:        "test-nourl-tool",
		Version:     "1.0.0",
		URLs:        map[string]string{}, // 无 URL
		Executable:  "noexe",
		ArchiveType: binary.ArchiveTypeRaw,
		Source:      binary.SourceDownload,
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-nourl-tool") })

	err := m.DownloadBinary("test-nourl-tool")
	if err == nil {
		t.Error("expected error for missing download URL")
	}
}

func TestManager_DownloadBinary_CargoURL(t *testing.T) {
	tmpDir := t.TempDir()
	m := binary.NewManager(tmpDir)

	meta := &binary.BinaryMeta{
		Name:        "test-cargo-tool",
		Version:     "1.0.0",
		URLs:        map[string]string{"*": "cargo:crate"},
		Executable:  "cargoexe",
		ArchiveType: binary.ArchiveTypeRaw,
		Source:      binary.SourceDownload,
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-cargo-tool") })

	err := m.DownloadBinary("test-cargo-tool")
	if err == nil {
		t.Error("expected error for cargo: URL")
	}
}

func TestManager_DownloadBinary_UnsupportedArchive(t *testing.T) {
	content := []byte("test")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write(content)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	m := binary.NewManager(tmpDir)

	meta := &binary.BinaryMeta{
		Name:        "test-badarch-tool",
		Version:     "1.0.0",
		URLs:        map[string]string{"*": server.URL},
		Executable:  "badarch",
		ArchiveType: binary.ArchiveType("unknown"),
		Source:      binary.SourceDownload,
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-badarch-tool") })

	err := m.DownloadBinary("test-badarch-tool")
	if err == nil {
		t.Error("expected error for unsupported archive type")
	}
}

// ---- moveFile (raw download already covers Rename path; test copy fallback) ----

func TestManager_DownloadBinary_RawSuccess(t *testing.T) {
	content := []byte("#!/bin/sh\necho raw")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write(content)
	}))
	defer server.Close()

	installDir := t.TempDir()
	m := binary.NewManager(installDir)

	meta := &binary.BinaryMeta{
		Name:        "test-raw-tool2",
		Version:     "1.0.0",
		URLs:        map[string]string{"*": server.URL},
		Executable:  "rawtool2",
		ArchiveType: binary.ArchiveTypeRaw,
		Source:      binary.SourceDownload,
	}
	binary.Register(meta)
	t.Cleanup(func() { binary.Unregister("test-raw-tool2") })

	if err := m.DownloadBinary("test-raw-tool2"); err != nil {
		t.Fatalf("DownloadBinary raw failed: %v", err)
	}
	installedPath := filepath.Join(installDir, "rawtool2")
	data, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatalf("read installed file: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("expected %q, got %q", string(content), string(data))
	}
}

// ---- Extractor 归档解压 ----

// createTarGZ 创建一个包含多个文件的 tar.gz 归档用于测试
func createTarGZ(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create tar.gz: %v", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("write tar content: %v", err)
		}
	}
}

// createZip 创建一个包含多个文件的 zip 归档用于测试
func createZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip content: %v", err)
		}
	}
}

func TestExtract_TarGZ(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")
	destDir := filepath.Join(tmpDir, "extracted")

	files := map[string]string{
		"file1.txt":         "content of file1",
		"subdir/file2.txt":  "content of file2",
	}
	createTarGZ(t, archivePath, files)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("mkdir destDir: %v", err)
	}

	if err := binary.Extract(archivePath, destDir, binary.ArchiveTypeTarGZ); err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// 验证文件内容
	for name, expectedContent := range files {
		path := filepath.Join(destDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("failed to read extracted file %s: %v", name, err)
			continue
		}
		if string(data) != expectedContent {
			t.Errorf("file %s: expected %q, got %q", name, expectedContent, string(data))
		}
	}
}

func TestExtract_Zip(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.zip")
	destDir := filepath.Join(tmpDir, "extracted")

	files := map[string]string{
		"file1.txt":        "zip content 1",
		"dir/file2.txt":    "zip content 2",
	}
	createZip(t, archivePath, files)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("mkdir destDir: %v", err)
	}

	if err := binary.Extract(archivePath, destDir, binary.ArchiveTypeZip); err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	for name, expectedContent := range files {
		path := filepath.Join(destDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("failed to read extracted file %s: %v", name, err)
			continue
		}
		if string(data) != expectedContent {
			t.Errorf("file %s: expected %q, got %q", name, expectedContent, string(data))
		}
	}
}

func TestExtract_UnsupportedType(t *testing.T) {
	tmpDir := t.TempDir()
	err := binary.Extract("nonexistent", tmpDir, binary.ArchiveType("raw"))
	if err == nil {
		t.Error("expected error for unsupported archive type")
	}
}

func TestExtract_TarGZ_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	err := binary.Extract("/nonexistent/archive.tar.gz", tmpDir, binary.ArchiveTypeTarGZ)
	if err == nil {
		t.Error("expected error for nonexistent source file")
	}
}

func TestExtract_Zip_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	err := binary.Extract("/nonexistent/archive.zip", tmpDir, binary.ArchiveTypeZip)
	if err == nil {
		t.Error("expected error for nonexistent zip file")
	}
}

func TestExtract_TarGZ_InvalidArchive(t *testing.T) {
	tmpDir := t.TempDir()
	invalidPath := filepath.Join(tmpDir, "invalid.tar.gz")
	// 写入非 gzip 数据
	if err := os.WriteFile(invalidPath, []byte("not a gzip file"), 0644); err != nil {
		t.Fatalf("create invalid file: %v", err)
	}
	err := binary.Extract(invalidPath, tmpDir, binary.ArchiveTypeTarGZ)
	if err == nil {
		t.Error("expected error for invalid gzip file")
	}
}
