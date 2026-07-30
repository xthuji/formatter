package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// defaultConfigJSON 是默认配置的 JSON 字节。
// 由 main 包在启动时通过 SetDefaultConfig() 注入 (源自 //go:embed data/config.json)。
// 若未注入，Default() 会回退到硬编码的基本默认值。
var defaultConfigJSON []byte

// SetDefaultConfig 注入默认配置的 JSON 数据 (由 main 包调用)。
func SetDefaultConfig(data []byte) {
	defaultConfigJSON = data
}

type Config struct {
	Server    ServerConfig          `json:"server"`
	Window    WindowConfig          `json:"window"`
	Binary    BinaryConfig          `json:"binary"`
	Format    FormatConfig          `json:"format"`
	Highlight HighlightConfig       `json:"highlight"`
	Aliases   map[string]string     `json:"aliases,omitempty"`
	Languages map[string]LangConfig `json:"languages"`
}

type ServerConfig struct {
	Port     int  `json:"port"`
	AutoPort bool `json:"auto_port"`
}

// WindowConfig 定义 App 桌面窗口的初始尺寸
type WindowConfig struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type BinaryConfig struct {
	// Runtimes 定义所有支持的运行时环境 (合并了定义与用户自定义路径)
	// 每个运行时包含: 名称、默认可执行文件名、用户自定义路径 (Path)、版本检测命令、各平台安装命令
	// Path 非空时优先使用，为空时自动从系统 PATH 查找 (exec.LookPath)
	Runtimes []RuntimeDef `json:"runtimes,omitempty"`
	// Tools 定义所有二进制工具 (替代 registry.go 中的硬编码注册)
	// 若为空，binary.LoadFromConfig 会回退到硬编码默认工具集
	Tools []ToolDef `json:"tools,omitempty"`
}

// ToolSource 描述二进制工具的安装来源类型，同时决定是否打包进 App
type ToolSource string

// RuntimeDef 定义一个运行时环境 (如 node/python/java/ruby)
type RuntimeDef struct {
	Name       string              `json:"name"`                  // 逻辑名称: node / python / java / ruby
	Exe        string              `json:"exe"`                   // 默认查找的可执行文件名
	Path       string              `json:"path,omitempty"`        // 用户自定义路径 (空=自动从 PATH 查找)
	VersionCmd []string            `json:"version_cmd,omitempty"` // 版本检测参数 (如 ["--version"])
	DetectExes []string            `json:"detect_exes,omitempty"` // 备选可执行文件名 (如 python3/python)
	// InstallCmds 各平台的安装命令: GOOS -> 命令数组
	// 空 slice 或不存在表示该平台不支持自动安装
	InstallCmds map[string][]string `json:"install_cmds,omitempty"`
}

// ToolDef 描述一个二进制工具的配置化定义 (与 binary.BinaryMeta 一一对应)
type ToolDef struct {
	// === 身份 ===
	Name      string   `json:"name"`                // 工具名称，如 "oxfmt"
	Languages []string `json:"languages,omitempty"` // 支持的编程语言列表，如 ["javascript","typescript","css"]
	Version   string   `json:"version,omitempty"`   // 版本号，如 "2.5.4"

	// === 运行时依赖 ===
	// Runtime 运行时名称 (node/python/java/ruby)，空=独立二进制 (无运行时依赖)
	Runtime string `json:"runtime,omitempty"`

	// === 可执行文件定位 (优先级: Path > InstallDir) ===
	// Path 用户直接指定的二进制路径 (绝对路径，支持 ~ 前缀)
	// 非空时优先使用，跳过下载与 InstallDir 查找；为空时走默认下载/查找流程
	Path       string `json:"path,omitempty"`       // 直接指定二进制路径 (可选)
	Executable string `json:"executable,omitempty"` // 解压后的可执行文件名 (Path 为空时用于 InstallDir 定位)

	// === 安装来源 (同时决定是否打包进 App) ===
	// Source 安装来源: download(默认, 下载并打包) / command(运行时安装, 不打包) / embedded(已静态打包)
	// download 时需配合 URL；embedded 时仅检测存在性；command 时需配合 InstallCmds
	Source ToolSource `json:"source,omitempty"`

	// === 下载来源 (仅 source=download 时生效) ===
	// URLs 各平台下载 URL 映射表，key 为 "os/arch" 格式 (如 "darwin/arm64", "linux/amd64")
	// URL 中支持 {version} 占位符，会被替换为 Version 字段值
	// 支持的平台: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64
	URLs    map[string]string `json:"urls,omitempty"`
	Archive string            `json:"archive,omitempty"` // 归档类型: raw / tar.gz / tar.xz / zip

	// === 命令安装 (仅 source=install 时生效) ===
	// InstallCmds 各平台的安装命令: GOOS -> 命令数组
	// 适用于依赖运行时环境的工具 (如 rubocop 通过 gem install 安装)
	// 命令中的 {version} 会被替换为 Version 字段值
	InstallCmds map[string][]string `json:"install_cmds,omitempty"`

	// === 验证 ===
	VerifyCmd string `json:"verify_cmd,omitempty"` // 版本验证命令模板，如 "{exe} --version"
	RunCmd    string `json:"run_cmd,omitempty"`    // 自定义执行命令模板 (如 "{runtime} -jar {exe}")，支持 {runtime}/{exe} 占位符
}

type FormatConfig struct {
	TabWidth   int    `json:"tab_width"` // -1=Tab 缩进, >0=空格缩进宽度
	LineEnding string `json:"line_ending"`
}

type HighlightConfig struct {
	Style       string `json:"style"`
	FontSize    int    `json:"font_size"`  // 高亮输出字体大小 (px), 0=不设置
	LineNumbers bool   `json:"line_numbers"`
	CompatHTML  bool   `json:"compat_html"` // 兼容性HTML输出 (内联样式+空格保护+<br>换行)
}

type LangConfig struct {
	Formatter   *ToolConfig      `json:"formatter"`
	Compressor  *ToolConfig      `json:"compressor"`
	Highlighter *ToolConfig      `json:"highlighter"`
	Indent      *IndentConfig    `json:"indent,omitempty"`
	Detection   *DetectionConfig `json:"detection,omitempty"`
}

// DetectionConfig 定义语言的自动检测规则。所有规则均为配置化，
// 新增语言支持只需在 config.json 中填写 detection 字段，无需修改代码。
type DetectionConfig struct {
	// Extensions 文件扩展名 (含点，匹配时不区分大小写)，如 [".go", ".go2"]
	Extensions []string `json:"extensions,omitempty"`
	// Shebangs 首行 shebang 匹配正则 (RE2 语法)，仅当内容以 #! 开头时检测，
	// 如 ["^#!.*\\bpython3?\\b", "^#!.*\\bnode\\b"]
	Shebangs []string `json:"shebangs,omitempty"`
	// Content 内容强信号正则 (RE2 语法)，对内容前 2048 字节匹配，
	// 如 ["^\\s*package\\s+main\\b"]。首个命中即返回该语言。
	Content []string `json:"content,omitempty"`
	// Priority 内容检测优先级，数值小者先匹配 (默认 0)。
	// 仅在多个语言的内容正则存在重叠时需要显式设置
	// (如 go 的 "package main" 须先于 java 的 "package" 匹配，故 go=1, java=2)。
	// 无 content 规则的语言无需设置。
	Priority int `json:"priority,omitempty"`
}

// IndentConfig 定义语言级别的缩进配置，覆盖全局 format 设置。
type IndentConfig struct {
	TabWidth int `json:"tab_width"` // -1=Tab 缩进, >0=空格缩进宽度, 0=动态 (仅 customIndentFrom 中使用)
	// CustomIndentFrom 声明工具自身输出的缩进格式。
	// 当工具不支持用户指定的 tab_width 时，
	// 程序在工具格式化完成后，将输出的缩进字符转换为用户指定的目标格式。
	// 例如工具固定输出 2 空格缩进: {"tab_width": 2}
	// 工具支持动态 tab_width (通过 {tab_width} 占位符): {"tab_width": 0}
	// 工具固定输出 Tab 缩进: {"tab_width": -1}
	CustomIndentFrom *IndentConfig `json:"customIndentFrom,omitempty"`
}

// IsTabIndent 返回是否使用 Tab 缩进 (tab_width == -1)
func (ic *IndentConfig) IsTabIndent() bool {
	return ic != nil && ic.TabWidth == -1
}

// ToolConfig 描述一个格式化/压缩/高亮工具的配置。
// 对于 native backend，仅需 tool 字段；
// 对于 external backend，通过 cmd 字段定义命令参数模板 (字符串数组)，
// 模板占位符由 ExpandCmdArgs 解析替换。
type ToolConfig struct {
	Backend string                 `json:"backend"`
	Tool    string                 `json:"tool"`
	Cmd     []string               `json:"cmd,omitempty"`
	Options map[string]interface{} `json:"options,omitempty"`
}

func Default() *Config {
	cfg := &Config{
		Languages: make(map[string]LangConfig),
	}

	if err := json.Unmarshal(defaultConfigJSON, cfg); err != nil {
		// Fallback to hardcoded defaults
		cfg.Server = ServerConfig{Port: 7890, AutoPort: true}
		cfg.Window = WindowConfig{Width: 1280, Height: 800}
		cfg.Format = FormatConfig{TabWidth: 2, LineEnding: "\n"}
		cfg.Highlight = HighlightConfig{Style: "github", FontSize: 14, LineNumbers: false, CompatHTML: false}
	}

	// Ensure window size has defaults (兼容缺少 window 字段的旧配置)
	if cfg.Window.Width <= 0 {
		cfg.Window.Width = 1280
	}
	if cfg.Window.Height <= 0 {
		cfg.Window.Height = 800
	}

	// Ensure maps are initialized
	if cfg.Languages == nil {
		cfg.Languages = make(map[string]LangConfig)
	}

	return cfg
}

// LocalConfigPath 返回用户配置文件的路径，按以下优先级查找：
//  1. 工作目录下的 data/config.json (开发模式，统一使用 data/config.json)
//  2. 可执行文件同目录的 config.json (App 模式，如 macOS: Formatter.app/Contents/MacOS/config.json)
func LocalConfigPath() string {
	// 开发模式：优先使用工作目录下的 data/config.json
	if devPath := filepath.Join("data", "config.json"); fileExists(devPath) {
		return devPath
	}
	// App 模式：可执行文件同目录的 config.json
	exePath, err := os.Executable()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(filepath.Dir(exePath), "config.json")
}

// fileExists 检查文件是否存在。
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// userConfigPath 返回用户主目录下的配置文件路径 (~/.formatter/config.json)
func userConfigPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".formatter", "config.json")
	}
	return "config.json"
}

// Load 从指定路径加载用户自定义配置并覆盖默认值。
// 若 path 为空，通过 LocalConfigPath() 自动定位：
//   - 开发模式：工作目录下的 data/config.json
//   - App 模式：可执行文件同目录的 config.json
// 文件存在则读取并覆盖默认值，不存在则返回嵌入的默认配置。
func Load(path string) (*Config, error) {
	cfg := Default()
	if path == "" {
		path = LocalConfigPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		// 文件不存在或读取失败，使用嵌入的默认配置
		return cfg, nil
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save 将配置写入指定路径。若 path 为空，通过 LocalConfigPath() 自动定位：
//   - 开发模式：写入工作目录下的 data/config.json
//   - App 模式：写入可执行文件同目录的 config.json
//   - 若目标目录不可写 (如 macOS App bundle 在 /Applications 中)，则回退到 ~/.formatter/config.json
func Save(cfg *Config, path string) error {
	if path == "" {
		path = LocalConfigPath()
		// 检测可执行文件目录是否可写，不可写则回退到用户主目录
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			path = userConfigPath()
		} else {
			testFile := filepath.Join(filepath.Dir(path), ".write_test")
			if f, err := os.Create(testFile); err != nil {
				path = userConfigPath()
			} else {
				f.Close()
				os.Remove(testFile)
			}
		}
	}
	data, err := json.MarshalIndent(map[string]interface{}{
		"server": map[string]interface{}{
			"port":      cfg.Server.Port,
			"auto_port": cfg.Server.AutoPort,
		},
		"window": map[string]interface{}{
			"width":  cfg.Window.Width,
			"height": cfg.Window.Height,
		},
		"binary": map[string]interface{}{
			"runtimes":     cfg.Binary.Runtimes,
			"tools":        cfg.Binary.Tools,
		},
		"format": map[string]interface{}{
			"tab_width":   cfg.Format.TabWidth,
			"line_ending": cfg.Format.LineEnding,
		},
		"highlight": map[string]interface{}{
			"style":        cfg.Highlight.Style,
			"font_size":    cfg.Highlight.FontSize,
			"line_numbers": cfg.Highlight.LineNumbers,
			"compat_html":  cfg.Highlight.CompatHTML,
		},
		"aliases":   cfg.Aliases,
		"languages": cfg.Languages,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ExpandHomePath 展开路径中的 ~ 前缀为用户主目录。
// 支持 "~" (单独) 和 "~/..." 两种形式；其他情况原样返回。
// 供 binary 和 appcommon 包共用，避免重复实现。
func ExpandHomePath(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if len(path) >= 2 && path[0] == '~' && path[1] == '/' {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
