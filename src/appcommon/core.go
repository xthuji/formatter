// Package appcommon 提供 Wails App 与 Web UI Server 共享的核心逻辑，
// 消除两者之间的大量重复代码 (运行时检测、注册表构建、配置处理、二进制管理等)。
package appcommon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/formatter/formatter/src/binary"
	"github.com/formatter/formatter/src/compressor"
	compressortool "github.com/formatter/formatter/src/compressor/tool"
	compressornative "github.com/formatter/formatter/src/compressor/native"
	"github.com/formatter/formatter/src/config"
	"github.com/formatter/formatter/src/formatter"
	formattertool "github.com/formatter/formatter/src/formatter/tool"
	formatternative "github.com/formatter/formatter/src/formatter/native"
	"github.com/formatter/formatter/src/highlighter"
	iocore "github.com/formatter/formatter/src/io"
	"github.com/formatter/formatter/src/pipeline"
	"github.com/formatter/formatter/src/registry"
)

// Core 持有引擎、注册表、配置和二进制管理器的共享核心。
// Wails App 和 Web UI Server 均通过组合 Core 复用所有业务逻辑。
type Core struct {
	Engine *pipeline.Engine
	Reg    *registry.Registry
	Cfg    *config.Config
	Mgr    *binary.Manager
}

// PipelineOptions 描述一次流水线执行的选项（对应 FormatRequest/RunRequest 的可配置字段）。
type PipelineOptions struct {
	Language           string
	TabWidth           int  // -1=Tab 缩进, >0=空格缩进宽度, 0=使用配置默认值
	Style              string
	FontSize           int  // 高亮字体大小 (px), 0=使用配置默认值
	LineNumbers        bool
	CompatHTML         bool // 兼容性HTML输出 (内联样式+空格保护+<br>换行)
	LineEnding         string
	FormatterBackend   string
	CompressorBackend  string
	HighlighterBackend string
	NoFormat           bool
	NoCompress         bool
	NoHighlight        bool
}

// NewCore 创建共享核心，完成配置加载、注册表构建和引擎初始化。
// homeDir 为二进制工具安装目录 (空则通过 NewManager 自动检测，包括
// App bundle 内的 Resources/bin、exeDir/bin、exeDir/data/bin 等路径)。
// 配置加载失败时回退到默认配置 (Load 内部已处理文件缺失，此处仅记录解析错误)。
func NewCore(homeDir string) *Core {
	// 增强 PATH: macOS GUI 应用不继承用户 shell 的 PATH 修改 (Homebrew/nvm 等)，
	// 导致 exec.LookPath 无法找到 node/python 等运行时。需在任何检测前完成。
	EnrichPath()

	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[appcommon] 配置加载失败，使用默认配置: %v\n", err)
		cfg = config.Default()
	}
	binary.LoadFromConfig(cfg)
	// homeDir 为空时由 NewManager 统一探测 (环境变量 → App bundle → 本地目录 → ~/.formatter)
	mgr := binary.NewManagerWithConfig(homeDir, cfg)
	reg := BuildRegistry(mgr, cfg)
	engine := pipeline.New(reg, cfg)
	// 注入配置驱动的语言检测规则 (扩展名/shebang/内容正则)
	iocore.SetDetectionRules(cfg.Languages)

	return &Core{Engine: engine, Reg: reg, Cfg: cfg, Mgr: mgr}
}

// ExecutePipeline 执行格式化/压缩/高亮流水线，返回处理结果字符串。
func (c *Core) ExecutePipeline(ctx context.Context, code string, params pipeline.ExecuteParams) (string, error) {
	reader := &BufferReader{data: []byte(code)}
	writer := &BufferWriter{}
	params.Reader = reader
	params.Writer = writer
	if err := c.Engine.Execute(ctx, params); err != nil {
		return "", err
	}
	return writer.String(), nil
}

// ---- 流水线参数构建 (消除 CLI/Wails/Server 重复代码) ----

// resolveIndent 解析指定语言的缩进配置 (单字段语义: -1=Tab 缩进, >0=空格缩进宽度, 0=未设置)。
// 优先级：显式用户输入 (tabWidth != 0) > 语言级配置 > 全局配置
func (c *Core) resolveIndent(lang string, tabWidth int) int {
	// 1. 全局默认
	resolved := c.Cfg.Format.TabWidth

	// 2. 语言级配置覆盖全局
	if langCfg, ok := c.Cfg.Languages[lang]; ok && langCfg.Indent != nil {
		resolved = langCfg.Indent.TabWidth
	}

	// 3. 显式用户输入覆盖语言级 (tabWidth != 0 表示用户明确指定: -1 或 >0)
	if tabWidth != 0 {
		resolved = tabWidth
	}

	return resolved
}

// resolveStyle 返回实际使用的高亮主题 (空值时回退到配置默认值)。
func (c *Core) resolveStyle(style string) string {
	if style == "" {
		return c.Cfg.Highlight.Style
	}
	return style
}

// resolveFontSize 返回实际使用的字体大小 (0 时回退到配置默认值)。
func (c *Core) resolveFontSize(fontSize int) int {
	if fontSize <= 0 {
		return c.Cfg.Highlight.FontSize
	}
	return fontSize
}

// resolveLineEnding 返回实际使用的行结束符 (空值时回退到配置默认值)。
func (c *Core) resolveLineEnding(le string) string {
	if le == "" {
		return c.Cfg.Format.LineEnding
	}
	return le
}

// FormatParams 构建「仅格式化」的 ExecuteParams。
func (c *Core) FormatParams(opts PipelineOptions) pipeline.ExecuteParams {
	tabWidth := c.resolveIndent(opts.Language, opts.TabWidth)
	return pipeline.ExecuteParams{
		Language:                opts.Language,
		EnableFormat:            true,
		EnableCompress:          false,
		EnableHighlight:         false,
		CompressSkipUnsupported: true,
		FormatterBackend:        opts.FormatterBackend,
		CompressorBackend:       opts.CompressorBackend,
		HighlighterBackend:      opts.HighlighterBackend,
		FormatOpts: formatter.FormatOptions{
			TabWidth:   tabWidth,
			LineEnding: c.resolveLineEnding(opts.LineEnding),
		},
	}
}

// CompressParams 构建「仅压缩」的 ExecuteParams。
func (c *Core) CompressParams(opts PipelineOptions) pipeline.ExecuteParams {
	return pipeline.ExecuteParams{
		Language:                opts.Language,
		EnableFormat:            false,
		EnableCompress:          true,
		EnableHighlight:         false,
		CompressSkipUnsupported: true,
		FormatterBackend:        opts.FormatterBackend,
		CompressorBackend:       opts.CompressorBackend,
		HighlighterBackend:      opts.HighlighterBackend,
		CompressOpts: compressor.CompressOptions{
			RemoveComments: true,
		},
	}
}

// HighlightParams 构建「仅高亮」的 ExecuteParams。
func (c *Core) HighlightParams(opts PipelineOptions) pipeline.ExecuteParams {
	return pipeline.ExecuteParams{
		Language:                opts.Language,
		EnableFormat:            false,
		EnableCompress:          false,
		EnableHighlight:         true,
		CompressSkipUnsupported: true,
		FormatterBackend:        opts.FormatterBackend,
		CompressorBackend:       opts.CompressorBackend,
		HighlighterBackend:      opts.HighlighterBackend,
		HighlightOpts: highlighter.HighlightOptions{
			Style:       c.resolveStyle(opts.Style),
			FontSize:    c.resolveFontSize(opts.FontSize),
			LineNumbers: opts.LineNumbers,
			CompatHTML:  opts.CompatHTML,
		},
	}
}

// RunParams 构建「美化&高亮 (format+highlight)」的 ExecuteParams。
// 注意: 美化&高亮 仅执行格式化+高亮，不执行压缩 (EnableCompress 由 NoCompress 控制，默认禁用)。
func (c *Core) RunParams(opts PipelineOptions) pipeline.ExecuteParams {
	tabWidth := c.resolveIndent(opts.Language, opts.TabWidth)
	return pipeline.ExecuteParams{
		Language:                opts.Language,
		EnableFormat:            !opts.NoFormat,
		EnableCompress:          !opts.NoCompress,
		EnableHighlight:         !opts.NoHighlight,
		CompressSkipUnsupported: true,
		FormatterBackend:        opts.FormatterBackend,
		CompressorBackend:       opts.CompressorBackend,
		HighlighterBackend:      opts.HighlighterBackend,
		FormatOpts: formatter.FormatOptions{
			TabWidth:   tabWidth,
			LineEnding: c.resolveLineEnding(opts.LineEnding),
		},
		CompressOpts: compressor.CompressOptions{
			RemoveComments: true,
		},
		HighlightOpts: highlighter.HighlightOptions{
			Style:       c.resolveStyle(opts.Style),
			FontSize:    c.resolveFontSize(opts.FontSize),
			LineNumbers: opts.LineNumbers,
			CompatHTML:  opts.CompatHTML,
		},
	}
}

// GetStatus 返回 Core 的运行状态信息 (供 Wails/Server 共用)。
func (c *Core) GetStatus() StatusResult {
	installed, _ := c.Mgr.ListInstalled()
	registered := binary.List()
	missing := make([]string, 0)
	for _, name := range registered {
		if _, err := c.Mgr.FindBinary(name); err != nil {
			missing = append(missing, name)
		}
	}
	return StatusResult{
		Success:      true,
		Version:      Version,
		GoVersion:    runtime.Version(),
		Platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		InstallDir:   c.Mgr.InstallDir,
		InstalledBin: installed,
		MissingBin:   missing,
		Runtimes:     c.DetectRuntimes(),
	}
}

// ---- 共享类型 ----

// FormatRequest 格式化/压缩/高亮的统一请求体 (Wails 与 Web UI 共用)
type FormatRequest struct {
	Language           string `json:"language"`
	Code               string `json:"code"`
	TabWidth           int    `json:"tab_width"`  // -1=Tab 缩进, >0=空格缩进宽度, 0=使用配置默认值
	Style              string `json:"style"`
	FontSize           int    `json:"font_size"` // 高亮字体大小 (px), 0=使用配置默认值
	LineNumbers        bool   `json:"line_numbers"`
	CompatHTML         bool   `json:"compat_html"` // 兼容性HTML输出 (内联样式+空格保护+<br>换行)
	FormatterBackend   string `json:"formatter_backend"`
	CompressorBackend  string `json:"compressor_backend"`
	HighlighterBackend string `json:"highlighter_backend"`
}

// RunRequest 完整流水线 (format+compress+highlight) 的请求体
type RunRequest struct {
	Language           string `json:"language"`
	Code               string `json:"code"`
	TabWidth           int    `json:"tab_width"`  // -1=Tab 缩进, >0=空格缩进宽度, 0=使用配置默认值
	Style              string `json:"style"`
	FontSize           int    `json:"font_size"` // 高亮字体大小 (px), 0=使用配置默认值
	LineNumbers        bool   `json:"line_numbers"`
	CompatHTML         bool   `json:"compat_html"` // 兼容性HTML输出 (内联样式+空格保护+<br>换行)
	NoFormat           bool   `json:"no_format"`
	NoCompress         bool   `json:"no_compress"`
	NoHighlight        bool   `json:"no_highlight"`
	FormatterBackend   string `json:"formatter_backend"`
	CompressorBackend  string `json:"compressor_backend"`
	HighlighterBackend string `json:"highlighter_backend"`
}

// RuntimeInfo 描述一个运行时的安装状态
type RuntimeInfo struct {
	Name        string `json:"name"`
	Exe         string `json:"exe"`
	Installed   bool   `json:"installed"`
	Installable bool   `json:"installable"`
	Version     string `json:"version,omitempty"`
	Path        string `json:"path,omitempty"`
}

// BinInfo 描述一个二进制工具的详细信息
type BinInfo struct {
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Languages     []string `json:"languages"`
	Installed     bool     `json:"installed"`
	Path          string   `json:"path,omitempty"`
	CustomPath    bool     `json:"custom_path,omitempty"`
	Runtime       string   `json:"runtime"`
	RuntimeReady  bool     `json:"runtime_ready"`
	RunCmd        string   `json:"run_cmd,omitempty"`
	Size          int64    `json:"size,omitempty"`
	Source        string   `json:"source"`         // 安装位置: "app", "system", "missing"
	ConfigSource  string   `json:"config_source"`  // 配置来源: "preset", "download"
	Editable      bool     `json:"editable"`       // 是否可在 UI 中编辑 (仅 download 类型可编辑)
	SystemPath    string   `json:"system_path,omitempty"`
}

// LanguageInfo 描述一个语言支持的格式化/压缩/高亮工具
type LanguageInfo struct {
	Lang        string `json:"lang"`
	Formatter   string `json:"formatter"`
	Compressor  string `json:"compressor"`
	Highlighter string `json:"highlighter"`
}

// BinActionResult 二进制操作 (安装/卸载/验证) 的统一返回类型
type BinActionResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Result  string `json:"result,omitempty"`
}

// StatusResult 应用运行状态信息 (供 Wails/Server 共用)
type StatusResult struct {
	Success      bool          `json:"success"`
	Version      string        `json:"version"`
	GoVersion    string        `json:"go_version"`
	Platform     string        `json:"platform"`
	InstallDir   string        `json:"install_dir"`
	InstalledBin []string      `json:"installed_bins"`
	MissingBin   []string      `json:"missing_bins"`
	Runtimes     []RuntimeInfo `json:"runtimes"`
}

// ---- 注册表构建 ----

// BuildRegistry 构建包含 native formatter/compressor、external 适配器和 highlighter 的完整注册表。
// 所有入口 (CLI / Wails / Web UI) 共用此函数，保证行为一致。
// cfg 为已加载的用户配置 (含本地 config.json 覆盖)，外部工具注册以此为准；
// 若 cfg 为 nil 则回退到默认配置。
func BuildRegistry(mgr *binary.Manager, cfg *config.Config) *registry.Registry {
	if cfg == nil {
		cfg = config.Default()
	}

	reg := registry.New()

	// 设置语言别名 (来自 config.json 的 aliases 字段)
	if cfg.Aliases != nil {
		reg.SetAliases(cfg.Aliases)
	}

	// Native 格式化器 (配置驱动: 从 cfg.Languages 注册 backend=native 的格式化器)
	// 工厂映射将 config 中的 tool 名绑定到 Go 实现，新增 native 格式化器只需在此添加一行
	for lang, langCfg := range cfg.Languages {
		if langCfg.Formatter == nil || langCfg.Formatter.Backend != "native" {
			continue
		}
		if factory, ok := nativeFormatterFactories[langCfg.Formatter.Tool]; ok {
			reg.RegisterFormatter(lang, factory())
		}
	}

	// 外部格式化器 (基于 cfg 中的 cmd 模板注册，反映用户本地配置覆盖)
	for lang, langCfg := range cfg.Languages {
		if langCfg.Formatter == nil {
			continue
		}
		tc := langCfg.Formatter
		if tc.Backend == "tool" {
			reg.RegisterFormatter(lang, formattertool.NewCmdAdapter(tc.Tool, lang, tc, cfg, mgr, langCfg.Indent))
		}
	}

	// Native 压缩器 (配置驱动: 从 cfg.Languages 注册 backend=native 的压缩器)
	for lang, langCfg := range cfg.Languages {
		if langCfg.Compressor == nil || langCfg.Compressor.Backend != "native" {
			continue
		}
		if factory, ok := nativeCompressorFactories[langCfg.Compressor.Tool]; ok {
			reg.RegisterCompressor(lang, factory())
		}
	}

	// 外部压缩器
	for lang, langCfg := range cfg.Languages {
		if langCfg.Compressor == nil {
			continue
		}
		tc := langCfg.Compressor
		if tc.Backend == "tool" {
			reg.RegisterCompressor(lang, compressortool.NewCmdCompressAdapter(tc.Tool, lang, tc, cfg, mgr))
		}
	}

	// 高亮器 (chroma，配置驱动)
	// 从配置中构建语言 → chroma lexer 名的覆盖映射 (highlighter.options.lexer)
	lexerOverrides := buildChromaLexerOverrides(cfg)
	ch := highlighter.NewChromaHighlighter(lexerOverrides)
	// 为所有配置的语言注册高亮器 (chroma 原生支持所有主流语言名作为 lexer 别名)
	for lang := range cfg.Languages {
		reg.RegisterHighlighter(lang, ch)
	}
	// 为语言别名注册高亮器 (如 bash/sh/zsh → shell 的反向覆盖)
	for alias := range cfg.Aliases {
		reg.RegisterHighlighter(alias, ch)
	}
	// 为 "text" 注册 fallback 高亮器 (chroma 的 plaintext lexer)
	reg.RegisterHighlighter("text", ch)

	return reg
}

// nativeFormatterFactories 将 config 中的 tool 名 (formatter.tool) 绑定到 Go 原生格式化器实现。
// 新增 native 格式化器时只需在此添加一行映射，无需修改 BuildRegistry 逻辑。
var nativeFormatterFactories = map[string]func() formatter.Formatter{
	"json":       func() formatter.Formatter { return formatternative.NewJSONFormatter() },
	"yaml":       func() formatter.Formatter { return formatternative.NewYAMLFormatter() },
	"xml":        func() formatter.Formatter { return formatternative.NewXMLFormatter() },
	"html":       func() formatter.Formatter { return formatternative.NewHTMLFormatter() },
	"go":         func() formatter.Formatter { return formatternative.NewGoFormatter() },
	"sql":        func() formatter.Formatter { return formatternative.NewSQLFormatter() },
	"toml":       func() formatter.Formatter { return formatternative.NewTOMLFormatter() },
	"properties": func() formatter.Formatter { return formatternative.NewPropertiesFormatter() },
	"ini":        func() formatter.Formatter { return formatternative.NewINIFormatter() },
}

// NewNativeFormatter 根据工具名创建原生格式化器实例。
// 供 BuildRegistry 和外部测试统一调用，避免逻辑重复。
// 返回 nil 表示未知的工具名。
func NewNativeFormatter(toolName string) formatter.Formatter {
	if factory, ok := nativeFormatterFactories[toolName]; ok {
		return factory()
	}
	return nil
}

// nativeCompressorFactories 将 config 中的 tool 名 (compressor.tool) 绑定到 Go 原生压缩器实现。
var nativeCompressorFactories = map[string]func() compressor.Compressor{
	"json": func() compressor.Compressor { return compressornative.NewJSONCompressor() },
	"xml":  func() compressor.Compressor { return compressornative.NewXMLCompressor() },
	"html": func() compressor.Compressor { return compressornative.NewHTMLCompressor() },
	"js":   func() compressor.Compressor { return compressornative.NewJSCompressor() },
	"css":  func() compressor.Compressor { return compressornative.NewCSSCompressor() },
	"sql":  func() compressor.Compressor { return compressornative.NewSQLCompressor() },
}

// NewNativeCompressor 根据工具名创建原生压缩器实例。
// 供 BuildRegistry 和外部测试统一调用，避免逻辑重复。
// 返回 nil 表示未知的工具名。
func NewNativeCompressor(toolName string) compressor.Compressor {
	if factory, ok := nativeCompressorFactories[toolName]; ok {
		return factory()
	}
	return nil
}

// buildChromaLexerOverrides 从配置中提取语言 → chroma 词法分析器名的覆盖映射。
// 每个语言的 highlighter.options.lexer 字段指定该语言在 chroma 中的 lexer 名，
// 未配置则使用语言名本身 (chroma v2 已将主流语言名注册为 lexer 别名)。
func buildChromaLexerOverrides(cfg *config.Config) map[string]string {
	overrides := make(map[string]string)
	for lang, lc := range cfg.Languages {
		if lc.Highlighter == nil || lc.Highlighter.Options == nil {
			continue
		}
		if lexer, ok := lc.Highlighter.Options["lexer"].(string); ok && lexer != "" {
			overrides[lang] = lexer
		}
	}
	return overrides
}

// ---- 语言列表 ----

// ListLanguages 返回所有支持的语言及其工具信息。
func (c *Core) ListLanguages() []LanguageInfo {
	langs := c.Reg.SupportedLanguages()
	infos := make([]LanguageInfo, 0, len(langs))
	for _, lang := range langs {
		info := LanguageInfo{Lang: lang}
		if f := c.Reg.GetFormatter(lang, ""); f != nil {
			info.Formatter = fmt.Sprintf("%s/%s", f.Type().String(), f.Name())
		}
		if cc := c.Reg.GetCompressor(lang, ""); cc != nil {
			info.Compressor = fmt.Sprintf("%s/%s", cc.Type().String(), cc.Name())
		}
		if h := c.Reg.GetHighlighter(lang, ""); h != nil {
			info.Highlighter = h.Name()
		}
		infos = append(infos, info)
	}
	return infos
}

// ---- 二进制管理 ----

// ListBinaries 列出所有注册的二进制工具及其安装状态。
func (c *Core) ListBinaries() []BinInfo {
	// 预先检测运行时一次, 避免每个工具重复执行
	runtimeOK := map[string]bool{}
	for _, rd := range c.RuntimeDefs() {
		_, runtimeOK[rd.Name] = ResolveRuntimePath(c.Cfg, rd.Name)
	}

	names := binary.List()
	infos := make([]BinInfo, 0, len(names))
	for _, name := range names {
		meta, err := binary.Get(name)
		if err != nil {
			continue
		}

		info := BinInfo{
			Name:         meta.Name,
			Version:      meta.Version,
			Languages:    meta.Languages,
			Runtime:      string(meta.Runtime),
			RunCmd:       meta.RunCmd,
			CustomPath:   meta.Path != "",
			RuntimeReady: true,
			Source:       "missing",
			ConfigSource: string(meta.Source),
			Editable:     meta.Source == binary.SourceDownload || meta.Source == binary.SourceInstall,
		}

		if meta.Runtime != "" {
			info.RuntimeReady = runtimeOK[string(meta.Runtime)]
		}

		// 1. 检测 App 内安装 (仅检查安装目录，不含系统 PATH)
		if path, ok := c.Mgr.FindInInstallDir(name); ok {
			info.Installed = true
			info.Path = path
			info.Source = "app"
			if stat, err := os.Stat(path); err == nil {
				info.Size = stat.Size()
			}
		}

		// 2. 检测系统 PATH (仅当 App 内未安装时)
		if !info.Installed {
			exeName := meta.Executable
			if runtime.GOOS == "windows" && filepath.Ext(exeName) == "" {
				exeName += ".exe"
			}
			if sysPath, err := exec.LookPath(exeName); err == nil {
				info.SystemPath = sysPath
				info.Source = "system"
				info.Installed = true
			}
		}

		infos = append(infos, info)
	}
	return infos
}

// InstallBinary 安装指定名称的二进制工具。
// 运行时依赖工具 (node/python/ruby) 委托给安装脚本。
func (c *Core) InstallBinary(name string) BinActionResult {
	if name == "" {
		return BinActionResult{Success: false, Error: "缺少工具名称"}
	}
	meta, err := binary.Get(name)
	if err != nil {
		return BinActionResult{Success: false, Error: err.Error()}
	}
	if meta.Runtime != "" {
		return c.installViaScript(name)
	}
	if err := c.Mgr.DownloadBinary(name); err != nil {
		return BinActionResult{Success: false, Error: err.Error()}
	}
	return BinActionResult{Success: true, Result: fmt.Sprintf("工具 %s 安装成功", name)}
}

// UninstallBinary 卸载指定名称的二进制工具。
func (c *Core) UninstallBinary(name string) BinActionResult {
	if name == "" {
		return BinActionResult{Success: false, Error: "缺少工具名称"}
	}
	if err := c.Mgr.RemoveBinary(name); err != nil {
		return BinActionResult{Success: false, Error: err.Error()}
	}
	return BinActionResult{Success: true, Result: fmt.Sprintf("工具 %s 已卸载", name)}
}

// VerifyBinary 验证指定名称的二进制工具版本。
func (c *Core) VerifyBinary(name string) BinActionResult {
	if name == "" {
		return BinActionResult{Success: false, Error: "缺少工具名称"}
	}
	version, err := c.Mgr.VerifyBinary(name)
	if err != nil {
		return BinActionResult{Success: false, Error: err.Error()}
	}
	return BinActionResult{Success: true, Result: fmt.Sprintf("版本: %s", version)}
}

// InstallRuntime 安装指定名称的运行时环境。
func (c *Core) InstallRuntime(name string) BinActionResult {
	if name == "" {
		return BinActionResult{Success: false, Error: "缺少运行时名称"}
	}
	if !c.Mgr.CanInstallRuntime(name) {
		return BinActionResult{Success: false, Error: fmt.Sprintf("运行时 %s 在当前平台不支持自动安装，请手动安装", name)}
	}
	output, err := c.Mgr.InstallRuntime(name)
	if err != nil {
		return BinActionResult{Success: false, Error: err.Error(), Result: output}
	}
	return BinActionResult{Success: true, Result: output}
}

// installViaScript 调用 install-bin.sh 安装运行时依赖工具
// name 由调用方保证非空 (InstallBinary 已校验)
func (c *Core) installViaScript(name string) BinActionResult {
	scriptPath := FindInstallScript()
	if scriptPath == "" {
		return BinActionResult{Success: false, Error: "未找到安装脚本 scripts/install-bin.sh"}
	}
	cmd := exec.Command("bash", scriptPath, "--tool="+name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return BinActionResult{
			Success: false,
			Error:   fmt.Sprintf("安装失败: %s\n%s", err.Error(), string(output)),
		}
	}
	return BinActionResult{
		Success: true,
		Result:  fmt.Sprintf("安装完成\n%s", string(output)),
	}
}

// FindInstallScript 定位 install-bin.sh 脚本路径。
func FindInstallScript() string {
	if exePath, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(filepath.Dir(exePath)), "scripts", "install-bin.sh")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(cwd, "scripts", "install-bin.sh")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// ---- 配置序列化/反序列化 ----

// ConfigToMap 将配置转为可序列化的 map (供前端 API 返回)。
func ConfigToMap(cfg *config.Config) map[string]interface{} {
	langsMap := map[string]interface{}{}
	for lang, langCfg := range cfg.Languages {
		langData := map[string]interface{}{}
		if langCfg.Formatter != nil && (langCfg.Formatter.Backend != "" || langCfg.Formatter.Tool != "") {
			fmtMap := map[string]interface{}{
				"backend": string(langCfg.Formatter.Backend),
				"tool":    langCfg.Formatter.Tool,
			}
			if len(langCfg.Formatter.Cmd) > 0 {
				fmtMap["cmd"] = langCfg.Formatter.Cmd
			}
			langData["formatter"] = fmtMap
		}
		if langCfg.Compressor != nil {
			cmpMap := map[string]interface{}{
				"backend": string(langCfg.Compressor.Backend),
				"tool":    langCfg.Compressor.Tool,
			}
			if len(langCfg.Compressor.Cmd) > 0 {
				cmpMap["cmd"] = langCfg.Compressor.Cmd
			}
			langData["compressor"] = cmpMap
		}
		if langCfg.Indent != nil {
			indentMap := map[string]interface{}{
				"tab_width": langCfg.Indent.TabWidth,
			}
			if langCfg.Indent.CustomIndentFrom != nil {
				indentMap["customIndentFrom"] = map[string]interface{}{
					"tab_width": langCfg.Indent.CustomIndentFrom.TabWidth,
				}
			}
			langData["indent"] = indentMap
		}
		if langCfg.Detection != nil {
			detMap := map[string]interface{}{
				"extensions": langCfg.Detection.Extensions,
			}
			if len(langCfg.Detection.Shebangs) > 0 {
				detMap["shebangs"] = langCfg.Detection.Shebangs
			}
			if len(langCfg.Detection.Content) > 0 {
				detMap["content"] = langCfg.Detection.Content
			}
			if langCfg.Detection.Priority != 0 {
				detMap["priority"] = langCfg.Detection.Priority
			}
			langData["detection"] = detMap
		}
		if langCfg.Highlighter != nil {
			hlMap := map[string]interface{}{
				"backend": string(langCfg.Highlighter.Backend),
				"tool":    langCfg.Highlighter.Tool,
			}
			if len(langCfg.Highlighter.Options) > 0 {
				hlMap["options"] = langCfg.Highlighter.Options
			}
			langData["highlighter"] = hlMap
		}
		langsMap[lang] = langData
	}

	return map[string]interface{}{
		"server": map[string]interface{}{
			"port":      cfg.Server.Port,
			"auto_port": cfg.Server.AutoPort,
		},
		"window": map[string]interface{}{
			"width":  cfg.Window.Width,
			"height": cfg.Window.Height,
		},
		"binary": map[string]interface{}{
			"runtimes": cfg.Binary.Runtimes,
			"tools":    cfg.Binary.Tools,
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
		"languages": langsMap,
	}
}

// UpdateConfigFromMap 从前端传入的 map 更新配置字段并持久化。
func UpdateConfigFromMap(cfg *config.Config, data map[string]interface{}) error {
	if srvMap, ok := data["server"].(map[string]interface{}); ok {
		if v, ok := srvMap["port"].(float64); ok {
			cfg.Server.Port = int(v)
		}
		if v, ok := srvMap["auto_port"].(bool); ok {
			cfg.Server.AutoPort = v
		}
	}
	if winMap, ok := data["window"].(map[string]interface{}); ok {
		if v, ok := winMap["width"].(float64); ok && v > 0 {
			cfg.Window.Width = int(v)
		}
		if v, ok := winMap["height"].(float64); ok && v > 0 {
			cfg.Window.Height = int(v)
		}
	}
	if binMap, ok := data["binary"].(map[string]interface{}); ok {
		if rtArr, ok := binMap["runtimes"].([]interface{}); ok {
			for _, rtItem := range rtArr {
				rtMap, ok := rtItem.(map[string]interface{})
				if !ok {
					continue
				}
				rtName, _ := rtMap["name"].(string)
				rtPath, _ := rtMap["path"].(string)
				for i := range cfg.Binary.Runtimes {
					if cfg.Binary.Runtimes[i].Name == rtName {
						cfg.Binary.Runtimes[i].Path = rtPath
						break
					}
				}
			}
		}
	}
	if fmtMap, ok := data["format"].(map[string]interface{}); ok {
		if v, ok := fmtMap["tab_width"].(float64); ok {
			cfg.Format.TabWidth = int(v)
		}
		if v, ok := fmtMap["line_ending"].(string); ok {
			cfg.Format.LineEnding = v
		}
	}
	if hlMap, ok := data["highlight"].(map[string]interface{}); ok {
		if v, ok := hlMap["style"].(string); ok && v != "" {
			cfg.Highlight.Style = v
		}
		if v, ok := hlMap["font_size"].(float64); ok {
			cfg.Highlight.FontSize = int(v)
		}
		if v, ok := hlMap["line_numbers"].(bool); ok {
			cfg.Highlight.LineNumbers = v
		}
		if v, ok := hlMap["compat_html"].(bool); ok {
			cfg.Highlight.CompatHTML = v
		}
	}
	if langsMap, ok := data["languages"].(map[string]interface{}); ok {
		for lang, langVal := range langsMap {
			langData, ok := langVal.(map[string]interface{})
			if !ok {
				continue
			}
			langCfg, exists := cfg.Languages[lang]
			if !exists {
				langCfg = config.LangConfig{}
			}
			if fmtRaw, ok := langData["formatter"]; ok {
				langCfg.Formatter = ParseToolConfig(fmtRaw)
			}
			if cmpRaw, ok := langData["compressor"]; ok {
				if cmpMap, ok := cmpRaw.(map[string]interface{}); ok && cmpMap != nil {
					langCfg.Compressor = ParseToolConfig(cmpRaw)
				} else if cmpRaw == nil {
					langCfg.Compressor = nil
				}
			}
			cfg.Languages[lang] = langCfg
		}
	}
	return config.Save(cfg, "")
}

// ParseToolConfig 将 map[string]interface{} 解析为 *config.ToolConfig。
func ParseToolConfig(raw interface{}) *config.ToolConfig {
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	tc := &config.ToolConfig{}
	if v, ok := m["backend"].(string); ok {
		tc.Backend = v
	}
	if v, ok := m["tool"].(string); ok {
		tc.Tool = v
	}
	if cmdRaw, ok := m["cmd"].([]interface{}); ok {
		cmd := make([]string, 0, len(cmdRaw))
		for _, c := range cmdRaw {
			if s, ok := c.(string); ok && s != "" {
				cmd = append(cmd, s)
			}
		}
		if len(cmd) > 0 {
			tc.Cmd = cmd
		}
	}
	if opts, ok := m["options"].(map[string]interface{}); ok && len(opts) > 0 {
		tc.Options = opts
	}
	return tc
}
