package appcommon

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// pathEnriched 用于避免重复执行 PATH 增强 (进程生命周期内执行一次即可)。
var pathEnriched bool

// EnrichPath 增强 GUI 应用的 PATH 环境变量。
//
// 背景: macOS 上从 Finder 启动的 GUI 应用不继承用户 shell 的 PATH 修改
// (如 Homebrew 的 /opt/homebrew/bin、nvm 的 node 路径等)，导致
// exec.LookPath 无法找到 node/python/ruby 等运行时。
// 而从终端启动的 CLI/测试进程会继承完整 PATH，因此出现「测试可见但 App 不可见」的差异。
//
// 本函数在 macOS 上通过登录 shell 获取用户完整 PATH 并合并到当前进程，
// 同时补充常见安装目录作为兜底。Linux/Windows 上为 no-op。
// 函数幂等，多次调用仅首次生效。
func EnrichPath() {
	if pathEnriched {
		return
	}
	pathEnriched = true

	if runtime.GOOS != "darwin" {
		// Linux/Windows 通常从终端启动，PATH 已包含用户目录；不做处理
		return
	}

	current := os.Getenv("PATH")
	merged := current

	// 1. 通过登录 shell 获取用户完整 PATH (会 source ~/.zshrc / ~/.bash_profile 等)
	//    使用 SHELL 环境变量确定用户的登录 shell，默认 zsh (macOS 默认)
	if loginPath := loginShellPath(); loginPath != "" {
		merged = mergePathEntries(merged, loginPath)
	}

	// 2. 补充常见 macOS 安装目录作为兜底 (即使登录 shell 失败也能覆盖主流场景)
	commonDirs := []string{
		"/usr/local/bin",
		"/usr/local/sbin",
		"/opt/homebrew/bin",
		"/opt/homebrew/sbin",
	}
	merged = mergePathEntries(merged, commonDirs...)

	// 3. 扫描 nvm 目录，将已安装的 node 版本 bin 目录加入 PATH
	//    nvm 将 node 安装在 ~/.nvm/versions/node/<version>/bin
	merged = mergePathEntries(merged, nvmBinPaths()...)

	if merged != current {
		os.Setenv("PATH", merged)
	}
}

// loginShellPath 通过登录 shell 获取用户的完整 PATH。
// 失败时返回空字符串 (调用方使用兜底目录)。
func loginShellPath() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	// 使用 -l (login shell) 触发读取 ~/.zprofile / ~/.bash_profile 等
	// -c 在读取配置后执行命令并退出
	cmd := exec.Command(shell, "-l", "-c", "echo $PATH")
	// 通过管道捕获输出，配合超时机制避免 shell 初始化卡住
	pr, pw, err := os.Pipe()
	if err != nil {
		return ""
	}
	cmd.Stdout = pw
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return ""
	}
	pw.Close()

	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		buf := make([]byte, 0, 4096)
		tmp := make([]byte, 4096)
		for {
			n, err := pr.Read(tmp)
			if n > 0 {
				buf = append(buf, tmp[:n]...)
			}
			if err != nil {
				done <- result{buf, err}
				return
			}
		}
	}()

	var out []byte
	select {
	case r := <-done:
		out = r.out
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		out = nil
	}
	pr.Close()
	_ = cmd.Wait()
	return strings.TrimSpace(string(out))
}

// nvmBinPaths 扫描 ~/.nvm/versions/node/ 目录，返回所有已安装 node 版本的 bin 路径。
// nvm 的 node 安装路径为 ~/.nvm/versions/node/v20.10.0/bin/node 等。
func nvmBinPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	versionsDir := filepath.Join(home, ".nvm", "versions", "node")
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		binDir := filepath.Join(versionsDir, entry.Name(), "bin")
		if info, err := os.Stat(binDir); err == nil && info.IsDir() {
			paths = append(paths, binDir)
		}
	}
	return paths
}

// mergePathEntries 将给定的路径条目合并到 base PATH 中，去重。
// 已存在的条目不会重复添加。
func mergePathEntries(base string, additions ...string) string {
	existing := make(map[string]bool)
	for _, p := range strings.Split(base, string(os.PathListSeparator)) {
		if p != "" {
			existing[p] = true
		}
	}
	parts := []string{base}
	for _, add := range additions {
		// additions 可以是单个路径，也可以是用 PathListSeparator 分隔的多个路径
		for _, p := range strings.Split(add, string(os.PathListSeparator)) {
			p = strings.TrimSpace(p)
			if p == "" || existing[p] {
				continue
			}
			existing[p] = true
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, string(os.PathListSeparator))
}
