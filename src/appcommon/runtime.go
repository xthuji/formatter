package appcommon

import (
	"os"
	"os/exec"
	"strings"

	"github.com/formatter/formatter/src/config"
)

// RuntimeDefs 返回运行时定义列表，若配置为空则回退到硬编码默认集。
func (c *Core) RuntimeDefs() []config.RuntimeDef {
	if c.Cfg != nil && len(c.Cfg.Binary.Runtimes) > 0 {
		return c.Cfg.Binary.Runtimes
	}
	return DefaultRuntimeDefs()
}

// DefaultRuntimeDefs 返回硬编码的默认运行时定义 (配置缺失时的兜底)。
func DefaultRuntimeDefs() []config.RuntimeDef {
	return []config.RuntimeDef{
		{Name: "node", Exe: "node", VersionCmd: []string{"--version"}, DetectExes: []string{"node"}},
		{Name: "python", Exe: "python3", VersionCmd: []string{"--version"}, DetectExes: []string{"python3", "python"}},
		{Name: "java", Exe: "java", VersionCmd: []string{"-version"}, DetectExes: []string{"java"}},
		{Name: "ruby", Exe: "ruby", VersionCmd: []string{"--version"}, DetectExes: []string{"ruby"}},
	}
}

// DetectRuntimes 检测所有格式化工具依赖的运行时环境。
// 运行时列表从配置文件读取，优先使用配置中指定的路径，未指定时从系统 PATH 查找。
func (c *Core) DetectRuntimes() []RuntimeInfo {
	defs := c.RuntimeDefs()
	infos := make([]RuntimeInfo, 0, len(defs))
	for _, rd := range defs {
		info := RuntimeInfo{Name: rd.Name, Exe: rd.Exe, Installable: c.Mgr.CanInstallRuntime(rd.Name)}
		if path, ok := ResolveRuntimePath(c.Cfg, rd.Name); ok {
			info.Installed = true
			info.Path = path
			info.Version = RuntimeVersion(path, rd.VersionCmd)
		}
		infos = append(infos, info)
	}
	return infos
}

// ResolveRuntimePath 解析运行时可执行文件路径。
// 优先级: 1. 配置中指定的路径 (cfg.Binary.Runtimes) 2. 系统 PATH (exec.LookPath)
func ResolveRuntimePath(cfg *config.Config, rt string) (string, bool) {
	if cfg != nil {
		for _, rd := range cfg.Binary.Runtimes {
			if rd.Name == rt && rd.Path != "" {
				customPath := config.ExpandHomePath(rd.Path)
				if info, err := os.Stat(customPath); err == nil && !info.IsDir() {
					return customPath, true
				}
			}
		}
	}
	candidates := runtimeDetectExes(cfg, rt)
	for _, exe := range candidates {
		if path, err := exec.LookPath(exe); err == nil {
			return path, true
		}
	}
	return "", false
}

// runtimeDetectExes 返回运行时的备选可执行文件名列表。
func runtimeDetectExes(cfg *config.Config, rt string) []string {
	if cfg != nil {
		for _, rd := range cfg.Binary.Runtimes {
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
	if exe := runtimeExecutableLegacy(rt); exe != "" {
		return []string{exe}
	}
	return []string{rt}
}

// runtimeExecutableLegacy 旧版硬编码的运行时到可执行文件映射 (配置缺失兜底)。
func runtimeExecutableLegacy(rt string) string {
	switch rt {
	case "node":
		return "node"
	case "python":
		if _, err := exec.LookPath("python3"); err == nil {
			return "python3"
		}
		return "python"
	case "java":
		return "java"
	case "ruby":
		return "ruby"
	}
	return rt
}

// RuntimeVersion 获取运行时的版本号。
// versionCmd 优先使用配置中指定的参数；为空时回退到按 exe 名推断。
func RuntimeVersion(exe string, versionCmd []string) string {
	args := versionCmd
	if len(args) == 0 {
		switch exe {
		case "node":
			args = []string{"--version"}
		case "python3", "python":
			args = []string{"--version"}
		case "java":
			args = []string{"-version"}
		case "ruby":
			args = []string{"--version"}
		default:
			return ""
		}
	}
	out, err := exec.Command(exe, args...).CombinedOutput()
	if err != nil {
		return ""
	}
	s := string(out)
	if len(s) == 0 {
		return ""
	}
	// 跳过开头的换行
	i := 0
	for i < len(s) && (s[i] == '\n' || s[i] == '\r') {
		i++
	}
	s = s[i:]
	// 截取第一行
	if j := strings.IndexByte(s, '\n'); j >= 0 {
		s = s[:j]
	}
	if len(s) > 60 {
		s = s[:60]
	}
	return strings.TrimSpace(s)
}
