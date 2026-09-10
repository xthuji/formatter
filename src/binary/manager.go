package binary

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/formatter/formatter/src/config"
)

// BuildToolCmd 构建工具的完整执行命令 (包含运行时前缀和参数)。
// 命令模板支持以下占位符:
//   {runtime} - 运行时可执行文件路径 (仅当工具配置了Runtime时有效)
//   {exe}     - 工具可执行文件/脚本/JAR包的路径
//
// 规则:
//   - 无运行时(Runtime为空): 返回 [exePath, args...]
//   - 有运行时且RunCmd非空: 解析RunCmd模板，将{runtime}/{exe}替换后与args拼接
//   - 有运行时且RunCmd为空: 默认返回 [runtimePath, exePath, args...] (适用于ruby/python脚本)
//
// runtimePath 由调用方通过ResolveRuntimePath获取 (避免与appcommon循环依赖)。
func (m *Manager) BuildToolCmd(name string, args ...string) (cmdPath string, cmdArgs []string, err error) {
	meta, err := Get(name)
	if err != nil {
		return "", nil, err
	}
	exePath, err := m.EnsureBinary(name)
	if err != nil {
		return "", nil, err
	}
	return m.buildCmdFromMeta(meta, exePath, args...)
}

// buildCmdFromMeta 内部方法: 根据BinaryMeta构建命令
func (m *Manager) buildCmdFromMeta(meta *BinaryMeta, exePath string, args ...string) (cmdPath string, cmdArgs []string, err error) {
	// 1. 无运行时依赖: 直接执行工具二进制
	if meta.Runtime == RuntimeNone {
		return exePath, args, nil
	}

	// 2. 解析运行时路径
	runtimePath, ok := m.ResolveRuntimePath(string(meta.Runtime))
	if !ok {
		return "", nil, fmt.Errorf("runtime %q not found for tool %q", meta.Runtime, meta.Name)
	}

	// 3. 根据RunCmd构建命令前缀
	var prefix []string
	if meta.RunCmd != "" {
		// 使用自定义RunCmd模板
		// 先将占位符替换为不包含空格的临时标记，再用 shell 解析，最后还原路径
		// 这样路径中的空格不会被错误分割
		tmpl := meta.RunCmd
		const runtimeToken = "\x00RUNTIME\x00"
		const exeToken = "\x00EXE\x00"
		tmpl = strings.ReplaceAll(tmpl, "{runtime}", runtimeToken)
		tmpl = strings.ReplaceAll(tmpl, "{exe}", exeToken)
		prefix = strings.Fields(tmpl)
		if len(prefix) == 0 {
			return "", nil, fmt.Errorf("invalid run_cmd for %q: empty result after template expansion", meta.Name)
		}
		// 还原占位符为实际路径
		for i, p := range prefix {
			prefix[i] = strings.ReplaceAll(strings.ReplaceAll(p, runtimeToken, runtimePath), exeToken, exePath)
		}
	} else {
		// 默认模式: {runtime} {exe}  (适用于解释型脚本: node script.js, ruby script.rb 等)
		prefix = []string{runtimePath, exePath}
	}

	// 4. 拼接参数
	fullArgs := make([]string, 0, len(prefix)-1+len(args))
	fullArgs = append(fullArgs, prefix[1:]...)
	fullArgs = append(fullArgs, args...)
	return prefix[0], fullArgs, nil
}

// ResolveRuntimePath 解析运行时可执行文件路径 (与appcommon.ResolveRuntimePath逻辑一致，避免循环依赖)。
// 优先级: 1. 配置中指定的路径 2. 系统PATH
func (m *Manager) ResolveRuntimePath(rt string) (string, bool) {
	if m.cfg != nil {
		for _, rd := range m.cfg.Binary.Runtimes {
			if rd.Name == rt && rd.Path != "" {
				customPath := config.ExpandHomePath(rd.Path)
				if info, err := os.Stat(customPath); err == nil && !info.IsDir() {
					return customPath, true
				}
			}
		}
	}
	candidates := m.runtimeDetectExes(rt)
	for _, exe := range candidates {
		if path, err := exec.LookPath(exe); err == nil {
			return path, true
		}
	}
	return "", false
}

// runtimeDetectExes 返回运行时的备选可执行文件名
func (m *Manager) runtimeDetectExes(rt string) []string {
	if m.cfg != nil {
		for _, rd := range m.cfg.Binary.Runtimes {
			if rd.Name == rt {
				if len(rd.DetectExes) > 0 {
					return rd.DetectExes
				}
				if rd.Exe != "" {
					return []string{rd.Exe}
				}
			}
		}
	}
	// 硬编码兜底
	switch rt {
	case "node":
		return []string{"node"}
	case "python":
		if _, err := exec.LookPath("python3"); err == nil {
			return []string{"python3"}
		}
		return []string{"python"}
	case "java":
		return []string{"java"}
	case "ruby":
		return []string{"ruby"}
	}
	return []string{rt}
}

// Manager 二进制管理器，负责安装、查找、下载、移除二进制工具
type Manager struct {
	InstallDir string
	cfg        *config.Config
}

func NewManager(installDir string) *Manager {
	// 优先级: 1. 显式参数  2. FORMATTER_BIN_HOME 环境变量  3. 自动检测  4. ~/.formatter
	if installDir == "" {
		installDir = os.Getenv("FORMATTER_BIN_HOME")
	}

	// 自动检测: 查找可执行文件附近的 bin 目录
	if installDir == "" {
		if exePath, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exePath)
			parentDir := filepath.Dir(exeDir)
			// 平台子目录: data/bin/{goos}/{goarch}/ (源码树中按平台组织)
			platformBin := filepath.Join(parentDir, "data", "bin", runtime.GOOS, runtime.GOARCH)
			// 回退: 扁平 data/bin/ (兼容旧版布局)
			for _, candidate := range []string{
				filepath.Join(parentDir, "Resources", "bin"), // macOS App bundle
				filepath.Join(exeDir, "bin"),                 // 同级 bin/
				platformBin,                                  // 开发环境 data/bin/{os}/{arch}
				filepath.Join(exeDir, "data", "bin"),         // 开发环境 data/bin (旧版)
				filepath.Join(parentDir, "data", "bin"),      // 父级 data/bin (旧版)
			} {
				if info, err := os.Stat(candidate); err == nil && info.IsDir() {
					installDir = candidate
					break
				}
			}
		}
	}

	// 最终回退到用户主目录
	if installDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			installDir = filepath.Join(home, ".formatter")
		}
	}
	return &Manager{InstallDir: installDir}
}

// writableInstallDir 返回可写的安装目录。
// 若 InstallDir 不可写 (如 macOS App bundle 内的 Resources/bin)，
// 回退到 ~/.formatter 以支持工具下载。
func (m *Manager) writableInstallDir() string {
	// 尝试在 InstallDir 中创建临时文件检测可写性
	testFile := filepath.Join(m.InstallDir, ".write_test")
	if err := os.MkdirAll(m.InstallDir, 0o755); err == nil {
		if f, err := os.Create(testFile); err == nil {
			f.Close()
			os.Remove(testFile)
			return m.InstallDir
		}
	}
	// 回退到用户主目录
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".formatter")
	}
	return m.InstallDir
}

func NewManagerWithConfig(installDir string, cfg *config.Config) *Manager {
	m := NewManager(installDir)
	m.cfg = cfg
	return m
}

// EnsureBinary 确保指定名称的二进制已安装，若不存在则自动下载
func (m *Manager) EnsureBinary(name string) (string, error) {
	if path, err := m.FindBinary(name); err == nil {
		return path, nil
	}
	meta, err := Get(name)
	if err != nil {
		return "", err
	}
	if meta.Path != "" {
		return "", fmt.Errorf("binary %q path %q not found", name, meta.Path)
	}
	if err := m.DownloadBinary(name); err != nil {
		return "", fmt.Errorf("ensure binary %q failed: %w", name, err)
	}
	return m.exePath(meta), nil
}

// FindBinary 查找已安装的工具文件 (二进制/JAR/脚本等)。
// 查找顺序: 1. 配置中指定的路径  2. 安装目录  3. 系统 PATH
// 对于需要运行时的工具 (如java -jar xxx.jar, node script.js)，不检查可执行权限位，
// 因为这些文件由运行时解释执行，本身不需要可执行权限。
func (m *Manager) FindBinary(name string) (string, error) {
	meta, err := Get(name)
	if err != nil {
		return "", err
	}

	// 1. 安装目录或用户自定义路径
	exePath := m.exePath(meta)
	if info, err := os.Stat(exePath); err == nil {
		// 用户自定义路径直接返回，不做权限检查
		if meta.Path != "" {
			return exePath, nil
		}
		// 需要运行时的工具 (JAR/脚本) 由解释器执行，本身不需要可执行位
		if meta.Runtime != RuntimeNone {
			return exePath, nil
		}
		// Windows .bat 脚本不需要可执行权限
		if runtime.GOOS == "windows" && strings.HasSuffix(strings.ToLower(exePath), ".bat") {
			return exePath, nil
		}
		// 独立二进制必须有可执行权限
		if info.Mode()&0111 != 0 {
			return exePath, nil
		}
		return "", fmt.Errorf("binary %q exists but is not executable", name)
	}

	// 2. 查找系统 PATH (与 ListBinaries 行为一致)
	exeName := windowsExeName(meta.Executable)
	if meta.Path == "" {
		if sysPath, err := exec.LookPath(exeName); err == nil {
			return sysPath, nil
		}
	}

	return "", fmt.Errorf("binary %q is not installed", name)
}

// DownloadBinary 下载并安装二进制
func (m *Manager) DownloadBinary(name string) error {
	meta, err := Get(name)
	if err != nil {
		return err
	}
	if meta.Source == SourcePreset {
		if _, err := m.FindBinary(name); err != nil {
			return fmt.Errorf("preset binary %q not found in %s", name, m.InstallDir)
		}
		return nil
	}
	if meta.Source == SourceInstall {
		return m.installFromCommand(meta)
	}
	return m.downloadFromURL(meta)
}

// installFromCommand 通过配置中的 install_cmds 安装工具 (适用于依赖运行时环境的工具，如 rubocop)
func (m *Manager) installFromCommand(meta *BinaryMeta) error {
	cmdArgs, ok := meta.InstallCmds[runtime.GOOS]
	if !ok || len(cmdArgs) == 0 {
		return fmt.Errorf("tool %q has no install command for %s", meta.Name, runtime.GOOS)
	}
	// 替换 {version} 占位符
	for i, arg := range cmdArgs {
		cmdArgs[i] = strings.ReplaceAll(arg, "{version}", meta.Version)
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	out := string(output)
	if err != nil {
		return fmt.Errorf("install tool %q via command %v: %w\n%s", meta.Name, cmdArgs, err, out)
	}
	return nil
}

func (m *Manager) downloadFromURL(meta *BinaryMeta) error {
	// 使用可写目录 (App bundle 内的 Resources/bin 只读时回退到 ~/.formatter)
	installDir := m.writableInstallDir()
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return fmt.Errorf("create install dir: %w", err)
	}

	// 从 URLs 映射表查找当前平台的下载 URL
	url := meta.PlatformURL(runtime.GOOS, runtime.GOARCH)
	if url == "" {
		return fmt.Errorf("no download URL for platform %s/%s for tool %q", runtime.GOOS, runtime.GOARCH, meta.Name)
	}

	// cargo: 前缀的 URL 表示需要从源码构建，Go 运行时不支持自动下载
	if strings.HasPrefix(url, "cargo:") {
		return fmt.Errorf("tool %q requires building from source (cargo), please run scripts/install-bin.sh", meta.Name)
	}

	// 创建临时文件路径 (Download 内部会创建/写入文件)
	tmpFile, err := os.CreateTemp("", "binary-download-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close() // 立即关闭，由 Download 函数重新打开写入
	defer os.Remove(tmpPath)

	if err := Download(url, tmpPath); err != nil {
		return fmt.Errorf("download %q from %s: %w", meta.Name, url, err)
	}

	installPath := filepath.Join(installDir, windowsExeName(meta.Executable))

	switch meta.ArchiveType {
	case ArchiveTypeRaw:
		if err := moveFile(tmpPath, installPath); err != nil {
			return fmt.Errorf("install raw binary: %w", err)
		}
	case ArchiveTypeTarGZ, ArchiveTypeTarXZ, ArchiveTypeZip:
		extractDir := filepath.Join(installDir, fmt.Sprintf(".extract-%s", meta.Name))
		defer os.RemoveAll(extractDir)
		if err := Extract(tmpPath, extractDir, meta.ArchiveType); err != nil {
			return fmt.Errorf("extract archive: %w", err)
		}
		if err := m.findAndExtractExecutable(extractDir, installPath, meta); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported archive type: %s", meta.ArchiveType)
	}

	if err := os.Chmod(installPath, 0o755); err != nil {
		return fmt.Errorf("set executable permission: %w", err)
	}
	// strip 二进制以去除调试符号，减小体积 (macOS/Linux)
	stripBinary(installPath, meta)
	return nil
}

// stripBinary 对已下载的二进制文件执行 strip 以去除调试符号，减小体积。
// 仅在 macOS/Linux 上执行；strip 失败不影响工具可用性 (保留原文件)。
// 对于依赖运行时的工具 (如 JAR 包)，跳过 strip。
func stripBinary(path string, meta *BinaryMeta) {
	// Windows 无 strip 命令
	if runtime.GOOS == "windows" {
		return
	}
	// 依赖运行时的工具 (JAR/脚本) 不需要 strip
	if meta.Runtime != "" {
		return
	}
	// 检查 strip 命令是否可用
	if _, err := exec.LookPath("strip"); err != nil {
		return
	}
	// 备份原文件
	backup := path + ".strip-bak"
	if err := os.Rename(path, backup); err != nil {
		return // 无法备份则跳过 strip
	}
	// 恢复函数: strip 失败时还原原文件
	restore := func() {
		os.Rename(backup, path)
	}
	// 执行 strip
	cmd := exec.Command("strip", path)
	if err := cmd.Run(); err != nil {
		restore()
		return
	}
	// 验证 strip 后仍可运行 (使用 verify_cmd 或 --version)
	ok := false
	if meta.VerifyCmd != "" {
		verify := strings.ReplaceAll(meta.VerifyCmd, "{exe}", path)
		parts := strings.Fields(verify)
		if len(parts) > 0 {
			if out, err := exec.Command(parts[0], parts[1:]...).CombinedOutput(); err == nil && len(out) >= 0 {
				ok = true
			}
		}
	}
	if !ok {
		// 回退尝试 --version / -version / -h
		for _, args := range [][]string{{"--version"}, {"-version"}, {"-h"}} {
			if exec.Command(path, args...).Run() == nil {
				ok = true
				break
			}
		}
	}
	if ok {
		os.Remove(backup)
	} else {
		restore()
	}
}

// ListInstalled 列出 InstallDir 中已安装的工具 (不含系统 PATH 中的工具)。
// 与 FindBinary 不同，此方法仅检查安装目录，不回退到系统 PATH，
// 因为 "已安装" 语义指 App 内管理的工具，而非系统全局可执行文件。
func (m *Manager) ListInstalled() ([]string, error) {
	_, err := os.ReadDir(m.InstallDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var installed []string
	for _, name := range List() {
		if _, ok := m.FindInInstallDir(name); ok {
			installed = append(installed, name)
		}
	}
	return installed, nil
}

// FindInInstallDir 仅在安装目录或用户自定义路径中查找工具 (不回退到系统 PATH)。
// 返回 (路径, 是否找到)。用于 ListInstalled 和 ListBinaries 区分 App 工具与系统工具。
func (m *Manager) FindInInstallDir(name string) (string, bool) {
	meta, err := Get(name)
	if err != nil {
		return "", false
	}
	exePath := m.exePath(meta)
	info, err := os.Stat(exePath)
	if err != nil {
		return "", false
	}
	// 用户自定义路径直接返回
	if meta.Path != "" {
		return exePath, true
	}
	// 需要运行时的工具 (JAR/脚本) 不检查可执行权限
	if meta.Runtime != RuntimeNone {
		return exePath, true
	}
	// Windows .bat 脚本不需要可执行权限
	if runtime.GOOS == "windows" && strings.HasSuffix(strings.ToLower(exePath), ".bat") {
		return exePath, true
	}
	// 独立二进制必须有可执行权限
	if info.Mode()&0111 != 0 {
		return exePath, true
	}
	return "", false
}

func (m *Manager) RemoveBinary(name string) error {
	meta, err := Get(name)
	if err != nil {
		return err
	}
	exePath := m.exePath(meta)
	if err := os.Remove(exePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("binary %q is not installed", name)
		}
		return fmt.Errorf("remove binary %q: %w", name, err)
	}
	return nil
}

func (m *Manager) VerifyBinary(name string) (string, error) {
	meta, err := Get(name)
	if err != nil {
		return "", err
	}
	exePath, err := m.FindBinary(name)
	if err != nil {
		return "", err
	}

	// 如果VerifyCmd为空，使用默认--version
	verifyCmd := meta.VerifyCmd
	if verifyCmd == "" {
		verifyCmd = "{exe} --version"
	}

	// 使用与BuildToolCmd相同的模板逻辑构建验证命令
	// 使用临时标记替换占位符，避免路径中的空格被 strings.Fields 错误分割
	cmdStr := verifyCmd
	const runtimeToken = "\x00RUNTIME\x00"
	const exeToken = "\x00EXE\x00"
	cmdStr = strings.ReplaceAll(cmdStr, "{exe}", exeToken)
	if meta.Runtime != RuntimeNone {
		if _, ok := m.ResolveRuntimePath(string(meta.Runtime)); ok {
			cmdStr = strings.ReplaceAll(cmdStr, "{runtime}", runtimeToken)
		}
	}

	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid verify command for %q", name)
	}
	// 还原占位符为实际路径
	runtimePath := "{runtime}"
	if meta.Runtime != RuntimeNone {
		if rp, ok := m.ResolveRuntimePath(string(meta.Runtime)); ok {
			runtimePath = rp
		}
	}
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(p, runtimeToken, runtimePath), exeToken, exePath)
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("verify %q: %s: %w", name, strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (m *Manager) exePath(meta *BinaryMeta) string {
	if meta.Path != "" {
		return config.ExpandHomePath(meta.Path)
	}
	return filepath.Join(m.InstallDir, windowsExeName(meta.Executable))
}

func (m *Manager) findAndExtractExecutable(extractDir, installPath string, meta *BinaryMeta) error {
	exeName := windowsExeName(meta.Executable)
	var foundPath string
	err := filepath.Walk(extractDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == exeName {
			foundPath = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk extract dir: %w", err)
	}
	if foundPath == "" {
		return fmt.Errorf("executable %q not found in archive for %q", exeName, meta.Name)
	}
	data, err := os.ReadFile(foundPath)
	if err != nil {
		return fmt.Errorf("read extracted executable: %w", err)
	}
	if err := os.WriteFile(installPath, data, 0o755); err != nil {
		return fmt.Errorf("write executable: %w", err)
	}
	return nil
}

func windowsExeName(name string) string {
	if runtime.GOOS != "windows" {
		return name
	}
	if filepath.Ext(name) != "" {
		return name
	}
	// Windows 上 wrapper 脚本使用 .bat 扩展名 (如 oxfmt-wrapper.bat, java-wrapper.bat)
	if runtime.GOOS == "windows" {
		if _, err := os.Stat(name + ".bat"); err == nil {
			return name + ".bat"
		}
	}
	return name + ".exe"
}

func (m *Manager) InstallRuntime(name string) (string, error) {
	if m.cfg == nil {
		return "", fmt.Errorf("manager config not set")
	}
	var rd *config.RuntimeDef
	for i := range m.cfg.Binary.Runtimes {
		if m.cfg.Binary.Runtimes[i].Name == name {
			rd = &m.cfg.Binary.Runtimes[i]
			break
		}
	}
	if rd == nil {
		return "", fmt.Errorf("runtime %q not defined in config", name)
	}
	cmdArgs, ok := rd.InstallCmds[runtime.GOOS]
	if !ok || len(cmdArgs) == 0 {
		return "", fmt.Errorf("runtime %q has no install command for %s", name, runtime.GOOS)
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	out := string(output)
	if err != nil {
		return out, fmt.Errorf("install runtime %q: %w\n%s", name, err, out)
	}
	return out, nil
}

func (m *Manager) CanInstallRuntime(name string) bool {
	if m.cfg == nil {
		return false
	}
	for i := range m.cfg.Binary.Runtimes {
		if rd := &m.cfg.Binary.Runtimes[i]; rd.Name == name {
			cmdArgs, ok := rd.InstallCmds[runtime.GOOS]
			return ok && len(cmdArgs) > 0
		}
	}
	return false
}

// moveFile 移动文件，支持跨文件系统 (先尝试 Rename，失败则 copy+delete)
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// 跨文件系统: 读写复制 + 删除源文件
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		return fmt.Errorf("write destination: %w", err)
	}
	os.Remove(src)
	return nil
}
