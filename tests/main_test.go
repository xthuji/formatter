package tests

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/formatter/formatter/src/appcommon"
	"github.com/formatter/formatter/src/config"
)

// testdataFS 嵌入 testdata 目录。Go 测试缓存仅跟踪 Go 源文件变化，
// 不跟踪 testdata 下的数据文件。通过 //go:embed 将 testdata 纳入测试二进制，
// 任何 testdata 文件的修改都会改变测试二进制，从而使 Go 测试缓存自动失效。
//
//go:embed testdata
var testdataFS embed.FS

// testCore 是所有集成测试共享的 Core 实例，在 TestMain 中用真实配置初始化。
var testCore *appcommon.Core

// TestMain 在所有测试开始前加载真实配置 (data/config.json) 和二进制目录 (data/bin)，
// 模拟真实工作环境，不使用 mock。
func TestMain(m *testing.M) {
	// 定位项目根目录 (tests/..)
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	projectRoot := filepath.Dir(wd)

	// 读取真实配置文件并注入为默认配置
	configData, err := os.ReadFile(filepath.Join(projectRoot, "data", "config.json"))
	if err != nil {
		panic("无法读取 data/config.json: " + err.Error())
	}
	config.SetDefaultConfig(configData)

	// 读取版本号并注入 (与 main.go 一致，源自 data/version.txt)
	if versionData, err := os.ReadFile(filepath.Join(projectRoot, "data", "version.txt")); err == nil {
		appcommon.SetVersion(string(versionData))
	}

	// 设置二进制工具目录，指向 data/bin/
	binDir := filepath.Join(projectRoot, "data", "bin")
	os.Setenv("FORMATTER_BIN_HOME", binDir)

	// 检测 RVM/rbenv 并将 gem bin 加入 PATH (使 source=install 的工具如 rubocop 可被找到)
	augmentPathForGemTools()

	// 切换到项目根目录
	origWd := wd
	os.Chdir(projectRoot)

	// 引用 testdataFS 防止编译器优化掉 //go:embed，确保 testdata 文件变化
	// 能使测试缓存失效
	_ = testdataFS

	// 用真实配置初始化 Core
	testCore = appcommon.NewCore(binDir)

	code := m.Run()

	os.Chdir(origWd)
	os.Exit(code)
}

// augmentPathForGemTools 检测 RVM/rbenv 并将 gem bin 和 ruby bin 目录加入 PATH，
// 使通过 gem install 安装的工具 (如 rubocop) 能被 exec.LookPath 找到，
// 且运行时使用正确的 Ruby 版本 (而非系统自带的老版本)。
func augmentPathForGemTools() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	var dirs []string
	// RVM: ~/.rvm/rubies/ruby-*/bin (Ruby 可执行文件，须优先于系统 /usr/bin)
	if rubies, _ := filepath.Glob(filepath.Join(home, ".rvm", "rubies", "ruby-*", "bin")); len(rubies) > 0 {
		for _, m := range rubies {
			if !strings.Contains(m, "@") {
				dirs = append(dirs, m)
			}
		}
	}
	// RVM: ~/.rvm/gems/ruby-*/bin (gem 安装的工具，排除 @global gemset)
	if gems, _ := filepath.Glob(filepath.Join(home, ".rvm", "gems", "ruby-*", "bin")); len(gems) > 0 {
		for _, m := range gems {
			if !strings.Contains(m, "@global") {
				dirs = append(dirs, m)
			}
		}
	}
	// rbenv: ~/.rbenv/shims (包含 ruby 和 gem 工具)
	if shims := filepath.Join(home, ".rbenv", "shims"); fileExists(shims) {
		dirs = append(dirs, shims)
	}
	if len(dirs) > 0 {
		os.Setenv("PATH", joinPathList(dirs)+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
}

func joinPathList(dirs []string) string {
	result := ""
	for i, d := range dirs {
		if i > 0 {
			result += string(os.PathListSeparator)
		}
		result += d
	}
	return result
}
