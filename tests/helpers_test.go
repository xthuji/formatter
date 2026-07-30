package tests

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// update 控制 是否生成测试输出文件 (.output) 用于审查。
// 期望文件 (.expected) 始终由开发者手动编写维护，测试永远不会覆盖。
//   -update   生成测试输出文件 (.output)，便于审查实际输出；不覆盖 .expected
//   默认      直接将实际输出与期望文件 (.expected) 比较，不写入任何文件
var update = flag.Bool("update", false, "生成测试输出文件 (.output) 用于审查 (不覆盖期望文件)")

// testdataDir 返回测试数据目录的路径 (TestMain 已 chdir 到项目根目录)
func testdataDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("tests", "testdata")
}

// langDir 返回指定语言的测试数据目录路径
func langDir(t *testing.T, lang string) string {
	t.Helper()
	return filepath.Join(testdataDir(t), lang)
}

// loadLangFile 读取指定语言目录下的测试数据文件
func loadLangFile(t *testing.T, lang, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(langDir(t, lang), name))
	if err != nil {
		t.Fatalf("无法读取测试文件 %s/%s: %v", lang, name, err)
	}
	return string(data)
}

// loadTestFile 读取测试数据文件内容 (兼容旧用法，位于 testdata 根目录)
func loadTestFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir(t), name))
	if err != nil {
		t.Fatalf("无法读取测试文件 %s: %v", name, err)
	}
	return string(data)
}

// expectedPath 构造期望输出文件路径: tests/testdata/{lang}/{filename}
func expectedPath(t *testing.T, lang, filename string) string {
	t.Helper()
	return filepath.Join(langDir(t, lang), filename)
}

// compareGolden 将 actual 输出与手动编写的期望文件 (.expected) 内容比较。
//   -update: 同时将 actual 写入 .output 文件 (测试输出文件)，便于审查；不覆盖 .expected
//   默认:    仅比较，不写入任何文件
// 期望文件 (.expected) 始终由开发者手动编写维护，测试永远不会覆盖。
// 任何空白、缩进、尾行差异都会导致测试失败，确保输出与预期完全一致。
func compareGolden(t *testing.T, goldenFile, actual string) {
	t.Helper()

	// -update 模式: 生成测试输出文件 (.output) 供审查 (不覆盖 .expected)
	if *update {
		outputFile := strings.TrimSuffix(goldenFile, ".expected") + ".output"
		if err := os.WriteFile(outputFile, []byte(actual), 0644); err != nil {
			t.Fatalf("无法写入测试输出文件 %s: %v", outputFile, err)
		}
		t.Logf("已生成测试输出文件: %s (%d 字节)", filepath.Base(outputFile), len(actual))
	}

	// 读取手动编写的期望文件 (.expected)
	expected, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatalf("期望文件 %s 不存在或无法读取: %v\n"+
			"期望文件由开发者手动编写。可运行 `go test -run %s -update` 生成测试输出文件 (.output)，\n"+
			"确认内容正确后复制为 .expected 文件",
			goldenFile, err, t.Name())
	}

	// 仅统一换行符 (CRLF → LF)，其余内容逐字节比较
	actualNorm := normalizeLineEndings(actual)
	expectedNorm := normalizeLineEndings(string(expected))
	if actualNorm != expectedNorm {
		t.Errorf("输出与期望文件 %s 不匹配:\n--- 期望 (%d 字节, 前 500 字符) ---\n%s\n--- 实际 (%d 字节, 前 500 字符) ---\n%s",
			filepath.Base(goldenFile),
			len(expectedNorm), truncate(expectedNorm, 500),
			len(actualNorm), truncate(actualNorm, 500))
	}
}

// normalizeLineEndings 统一换行符为 \n (CRLF → LF, CR → LF)，不修改任何空白。
func normalizeLineEndings(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

// ---- 辅助函数 ----

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// toolAvailable 检查工具是否可用: 先查 binDir，再查系统 PATH
func toolAvailable(binDir, toolFile string) bool {
	if fileExists(filepath.Join(binDir, toolFile)) {
		return true
	}
	_, err := exec.LookPath(toolFile)
	return err == nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func execCLI(cliPath string, args []string, env []string, stdin string) (string, error) {
	cmd := exec.Command(cliPath, args...)
	cmd.Env = append(os.Environ(), env...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}
