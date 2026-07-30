package binary

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/formatter/formatter/src/config"
)

// ArchiveType 归档类型枚举
type ArchiveType string

const (
	ArchiveTypeTarGZ ArchiveType = "tar.gz"
	ArchiveTypeTarXZ ArchiveType = "tar.xz"
	ArchiveTypeZip   ArchiveType = "zip"
	ArchiveTypeRaw   ArchiveType = "raw" // 无需解压的裸二进制
)

// RuntimeType 运行时类型
type RuntimeType string

const (
	RuntimeNone   RuntimeType = ""       // 独立二进制，无需运行时
	RuntimeNode   RuntimeType = "node"   // 需要 Node.js
	RuntimePython RuntimeType = "python" // 需要 Python
	RuntimeJava   RuntimeType = "java"   // 需要 Java
	RuntimeRuby   RuntimeType = "ruby"   // 需要 Ruby
)

// ToolSource 描述二进制工具的安装来源类型 (与 config.ToolSource 对齐)
type ToolSource string

const (
	SourceDownload ToolSource = "download" // 默认: URL 下载并打包进 App
	SourcePreset   ToolSource = "preset"   // 预置: 已静态打包进 App，仅验证存在性
	SourceInstall  ToolSource = "install"  // 安装: 通过命令安装 (如 gem install)，不打包进 App
)

// BinaryMeta 二进制工具元数据
type BinaryMeta struct {
	Name      string   // 工具名称，如 "oxfmt"
	Languages []string // 支持的编程语言列表
	Version   string   // 版本号

	Runtime    RuntimeType // 运行时类型，空字符串表示独立二进制
	Path       string      // 用户直接指定的二进制路径
	Executable string      // 解压后的可执行文件名

	Source      ToolSource          // 安装来源
	URLs        map[string]string   // 各平台下载 URL 映射表 (key: "os/arch")
	ArchiveType ArchiveType         // 归档类型
	InstallCmds map[string][]string // 各平台安装命令 (仅 source=install 时生效)

	VerifyCmd string // 版本验证命令模板
	RunCmd    string // 自定义执行命令模板 (如 "java -jar {exe}")，可选
}

// PlatformURL 返回当前平台的下载 URL (替换 {version} 占位符)
// 查找优先级:
//  1. 精确匹配 "os/arch" (如 "darwin/arm64")
//  2. OS 通配符 "os/*" (架构无关，如 JAR/脚本)
//  3. 完全通配符 "*" (跨所有平台的资源，如 JAR 包)
//
// 若均未匹配，返回空字符串
func (m *BinaryMeta) PlatformURL(goos, goarch string) string {
	// 1. 精确匹配
	key := goos + "/" + goarch
	if url, ok := m.URLs[key]; ok {
		return strings.ReplaceAll(url, "{version}", m.Version)
	}
	// 2. OS 通配符 "os/*" (架构无关)
	key = goos + "/*"
	if url, ok := m.URLs[key]; ok {
		return strings.ReplaceAll(url, "{version}", m.Version)
	}
	// 3. 完全通配符 "*" (跨平台资源，如 all-deps JAR)
	if url, ok := m.URLs["*"]; ok {
		return strings.ReplaceAll(url, "{version}", m.Version)
	}
	return ""
}

// ShouldBundle 返回工具是否需要通过下载脚本安装到 App Resources/bin 目录。
// 仅 download 类型工具返回 true (需要在构建时下载并打包)；
// preset 类型工具已存在于源码树 (data/bin)，无需下载安装，返回 false；
// install 类型工具通过运行时命令安装 (如 gem install)，不打包，返回 false。
func (m *BinaryMeta) ShouldBundle() bool {
	return m.Source == SourceDownload
}

// IsShipped 返回工具是否随 App 一起分发 (打包进 App Resources/bin)。
// download 和 preset 类型工具都会随 App 分发；
// install 类型工具不随 App 分发 (用户需自行安装)。
func (m *BinaryMeta) IsShipped() bool {
	return m.Source == SourceDownload || m.Source == SourcePreset
}

// registry 全局注册表
var (
	registry   = make(map[string]*BinaryMeta)
	registryMu sync.RWMutex
)

func Register(meta *BinaryMeta) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[meta.Name] = meta
}

// Unregister 从全局注册表移除指定工具 (主要用于测试清理，避免污染全局状态)
func Unregister(name string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	delete(registry, name)
}

func Get(name string) (*BinaryMeta, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	meta, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("binary %q not found in registry", name)
	}
	return meta, nil
}

func List() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func MustGet(name string) *BinaryMeta {
	meta, err := Get(name)
	if err != nil {
		panic(err)
	}
	return meta
}

// LoadFromConfig 从配置文件加载工具定义到注册表
func LoadFromConfig(cfg *config.Config) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = make(map[string]*BinaryMeta)

	if cfg == nil || len(cfg.Binary.Tools) == 0 {
		return
	}
	for i := range cfg.Binary.Tools {
		td := &cfg.Binary.Tools[i]
		meta := toolDefToMeta(td)
		registry[meta.Name] = meta
	}
}

func toolDefToMeta(td *config.ToolDef) *BinaryMeta {
	meta := &BinaryMeta{
		Name:        td.Name,
		Languages:   td.Languages,
		Version:     td.Version,
		Runtime:     RuntimeType(td.Runtime),
		Path:        td.Path,
		Executable:  td.Executable,
		Source:      ToolSource(td.Source),
		URLs:        td.URLs,
		ArchiveType: ArchiveType(td.Archive),
		InstallCmds: td.InstallCmds,
		VerifyCmd:   td.VerifyCmd,
		RunCmd:      td.RunCmd,
	}
	if meta.Source == "" {
		meta.Source = SourceDownload
	}
	if meta.ArchiveType == "" {
		meta.ArchiveType = ArchiveTypeRaw
	}
	return meta
}

// ListEmbedded 返回所有随 App 分发的工具名称 (download + embedded)，
// 用于构建时打包到 App Resources/bin 目录。
func ListEmbedded() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name, meta := range registry {
		if meta.IsShipped() {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
