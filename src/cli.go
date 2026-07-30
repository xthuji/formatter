package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/formatter/formatter/src/appcommon"
	"github.com/formatter/formatter/src/binary"
	"github.com/formatter/formatter/src/config"
	iocore "github.com/formatter/formatter/src/io"
	"github.com/formatter/formatter/src/pipeline"
	"github.com/formatter/formatter/src/ui"
)

type GlobalOptions struct {
	Lang               string
	Input              string
	Output             string
	Clipboard          bool
	TabWidth           int // -1=Tab 缩进, >0=空格缩进宽度, 0=使用配置默认值
	LineEnding         string
	Style              string
	FontSize           int
	LineNumbers        bool
	CompatHTML         bool // 兼容性HTML输出 (内联样式+空格保护+<br>换行)
	FormatterBackend   string
	CompressorBackend  string
	HighlighterBackend string
	ConfigPath         string
	BinDir             string
	Verbose            bool
	Quiet              bool
}

func NewRootCmd() *cobra.Command {
	opts := &GlobalOptions{}

	rootCmd := &cobra.Command{
		Use:   "formatter",
		Short: "Multi-language code formatter, compressor, and highlighter",
		Long: `Formatter is a multi-language code processing tool that supports:
  - Formatting code in 17+ languages
  - Compressing/minifying code
  - Syntax highlighting for HTML/terminal output

Examples:
  formatter format main.py
  formatter run app.js --style monokai -o app.html
  formatter compress styles.css
  formatter langs`,
		Version:      appcommon.Version,
		SilenceUsage: true,
	}

	rootCmd.PersistentFlags().StringVarP(&opts.Lang, "lang", "l", "auto", "Specify language (auto-detect if not set)")
	rootCmd.PersistentFlags().StringVarP(&opts.Input, "input", "i", "", "Input file (default: stdin)")
	rootCmd.PersistentFlags().StringVarP(&opts.Output, "output", "o", "", "Output file (default: stdout)")
	rootCmd.PersistentFlags().BoolVarP(&opts.Clipboard, "clipboard", "c", false, "Use clipboard for input/output")
	rootCmd.PersistentFlags().IntVar(&opts.TabWidth, "tab-width", 0, "Tab width (-1=use tabs, 0=use per-language or global config)")
	rootCmd.PersistentFlags().StringVar(&opts.LineEnding, "line-ending", "", "Line ending (lf, crlf)")
	rootCmd.PersistentFlags().StringVar(&opts.Style, "style", "github", "Highlight style theme")
	rootCmd.PersistentFlags().IntVar(&opts.FontSize, "font-size", 0, "Highlight font size in px (0=use config default)")
	rootCmd.PersistentFlags().BoolVar(&opts.LineNumbers, "line-numbers", false, "Show line numbers in highlight output")
	rootCmd.PersistentFlags().BoolVar(&opts.CompatHTML, "compat-html", false, "Compatibility HTML output for OneNote/Quiver")
	rootCmd.PersistentFlags().StringVar(&opts.FormatterBackend, "formatter-backend", "", "Force formatter backend (native/tool)")
	rootCmd.PersistentFlags().StringVar(&opts.CompressorBackend, "compressor-backend", "", "Force compressor backend (native/tool)")
	rootCmd.PersistentFlags().StringVar(&opts.HighlighterBackend, "highlighter-backend", "", "Force highlighter backend")
	rootCmd.PersistentFlags().StringVar(&opts.ConfigPath, "config", "", "Config file path")
	rootCmd.PersistentFlags().StringVar(&opts.BinDir, "bin-dir", "", "Binary tools directory")
	rootCmd.PersistentFlags().BoolVarP(&opts.Verbose, "verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().BoolVarP(&opts.Quiet, "quiet", "q", false, "Quiet mode")

	rootCmd.AddCommand(
		newFormatCmd(opts),
		newCompressCmd(opts),
		newHighlightCmd(opts),
		newRunCmd(opts),
		newBinaryCmd(opts),
		newLangsCmd(opts),
		newVersionCmd(),
		newServeCmd(opts),
	)

	return rootCmd
}

// buildCore 创建共享的 Core 实例，处理配置路径和二进制目录
func buildCore(opts *GlobalOptions) (*appcommon.Core, error) {
	// 二进制目录通过环境变量传递 (NewCore → NewManager 会读取)
	if opts.BinDir != "" {
		os.Setenv("FORMATTER_BIN_HOME", opts.BinDir)
	}

	// 统一使用 NewCore 构建 (homeDir 为空时由 NewManager 自动探测)
	// NewCore 内部完成: 配置加载 → binary.LoadFromConfig → NewManagerWithConfig → BuildRegistry → pipeline.New
	core := appcommon.NewCore(opts.BinDir)

	// 若指定了自定义配置路径，需用该配置重新初始化注册表和引擎
	if opts.ConfigPath != "" {
		cfg, err := config.Load(opts.ConfigPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[cli] 配置加载失败，使用默认配置: %v\n", err)
			cfg = config.Default()
		}
		binary.LoadFromConfig(cfg)
		core.Cfg = cfg
		core.Mgr = binary.NewManagerWithConfig(opts.BinDir, cfg)
		core.Reg = appcommon.BuildRegistry(core.Mgr, cfg)
		core.Engine = pipeline.New(core.Reg, cfg)
	}

	return core, nil
}

// executeCmd 执行流水线命令，处理 IO 和参数构建
func executeCmd(core *appcommon.Core, opts *GlobalOptions, args []string, params pipeline.ExecuteParams) error {
	reader, writer, err := buildReaderWriter(opts, args)
	if err != nil {
		return err
	}

	// 读取输入
	data, detectedLang, err := reader.Read()
	if err != nil {
		return err
	}
	if params.Language == "" || params.Language == "auto" {
		if detectedLang != "" {
			params.Language = detectedLang
		}
	}

	// 使用 BufferReader/BufferWriter 执行流水线
	params.Reader = appcommon.NewBufferReader(data)
	buf := appcommon.NewBufferWriter()
	params.Writer = buf
	if err := core.Engine.Execute(context.Background(), params); err != nil {
		return err
	}

	// 写入输出
	return writer.Write([]byte(buf.String()), params.EnableHighlight)
}

func newFormatCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "format [file]",
		Short: "Format code only",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			core, err := buildCore(opts)
			if err != nil {
				return err
			}
			return executeCmd(core, opts, args, core.FormatParams(appcommon.PipelineOptions{
				Language:           opts.Lang,
				TabWidth:           opts.TabWidth,
				LineEnding:         opts.LineEnding,
				FormatterBackend:   opts.FormatterBackend,
				CompressorBackend:  opts.CompressorBackend,
				HighlighterBackend: opts.HighlighterBackend,
			}))
		},
	}
	return cmd
}

func newCompressCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compress [file]",
		Short: "Compress/minify code only",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			core, err := buildCore(opts)
			if err != nil {
				return err
			}
			return executeCmd(core, opts, args, core.CompressParams(appcommon.PipelineOptions{
				Language:           opts.Lang,
				FormatterBackend:   opts.FormatterBackend,
				CompressorBackend:  opts.CompressorBackend,
				HighlighterBackend: opts.HighlighterBackend,
			}))
		},
	}
	return cmd
}

func newHighlightCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "highlight [file]",
		Short: "Highlight code only",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			core, err := buildCore(opts)
			if err != nil {
				return err
			}
			return executeCmd(core, opts, args, core.HighlightParams(appcommon.PipelineOptions{
				Language:           opts.Lang,
				Style:              opts.Style,
				FontSize:           opts.FontSize,
				LineNumbers:        opts.LineNumbers,
				CompatHTML:         opts.CompatHTML,
				FormatterBackend:   opts.FormatterBackend,
				CompressorBackend:  opts.CompressorBackend,
				HighlighterBackend: opts.HighlighterBackend,
			}))
		},
	}
	return cmd
}

func newRunCmd(opts *GlobalOptions) *cobra.Command {
	var (
		noFormat    bool
		noCompress  bool
		noHighlight bool
	)

	cmd := &cobra.Command{
		Use:   "run [file]",
		Short: "Run format + compress + highlight pipeline",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			core, err := buildCore(opts)
			if err != nil {
				return err
			}
			return executeCmd(core, opts, args, core.RunParams(appcommon.PipelineOptions{
				Language:           opts.Lang,
				TabWidth:           opts.TabWidth,
				LineEnding:         opts.LineEnding,
				Style:              opts.Style,
				FontSize:           opts.FontSize,
				LineNumbers:        opts.LineNumbers,
				CompatHTML:         opts.CompatHTML,
				NoFormat:           noFormat,
				NoCompress:         noCompress,
				NoHighlight:        noHighlight,
				FormatterBackend:   opts.FormatterBackend,
				CompressorBackend:  opts.CompressorBackend,
				HighlighterBackend: opts.HighlighterBackend,
			}))
		},
	}
	cmd.Flags().BoolVar(&noFormat, "no-format", false, "Skip formatting step")
	cmd.Flags().BoolVar(&noCompress, "no-compress", true, "Skip compression step (default: skip, 美化&高亮 不含压缩)")
	cmd.Flags().BoolVar(&noHighlight, "no-highlight", false, "Skip highlight step")
	return cmd
}

func newBinaryCmd(opts *GlobalOptions) *cobra.Command {
	binaryCmd := &cobra.Command{
		Use:   "binary",
		Short: "Manage external binary tools",
	}

	var installLang string
	var installTool string
	var installEmbeddedOnly bool

	binaryInstallCmd := &cobra.Command{
		Use:   "install",
		Short: "Install/download binary tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			core, err := buildCore(opts)
			if err != nil {
				return err
			}
			mgr := core.Mgr
			tools := resolveTools(installLang, installTool, installEmbeddedOnly)
			if len(tools) == 0 {
				if !opts.Quiet {
					fmt.Fprintln(os.Stdout, "No tools to install.")
				}
				return nil
			}
			for _, tool := range tools {
				if opts.Verbose {
					fmt.Fprintf(os.Stderr, "Installing %s...\n", tool)
				}
				if err := mgr.DownloadBinary(tool); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to install %s: %v\n", tool, err)
				} else if !opts.Quiet {
					fmt.Fprintf(os.Stdout, "Installed %s\n", tool)
				}
			}
			return nil
		},
	}
	binaryInstallCmd.Flags().StringVar(&installLang, "lang", "", "Install binaries for specific language")
	binaryInstallCmd.Flags().StringVar(&installTool, "tool", "", "Install specific tool(s), comma-separated")
	binaryInstallCmd.Flags().BoolVar(&installEmbeddedOnly, "embedded-only", false, "Only install tools marked as embedded (standalone binaries for App Resources/bin)")

	binaryCheckCmd := &cobra.Command{
		Use:   "check",
		Short: "Check binary tool status",
		RunE: func(cmd *cobra.Command, args []string) error {
			core, err := buildCore(opts)
			if err != nil {
				return err
			}
			installed, err := core.Mgr.ListInstalled()
			if err != nil {
				return err
			}
			if len(installed) == 0 {
				fmt.Fprintln(os.Stdout, "No binaries installed.")
				return nil
			}
			for _, name := range installed {
				binPath, err := core.Mgr.FindBinary(name)
				if err != nil {
					fmt.Fprintf(os.Stdout, "%s: ERROR - %v\n", name, err)
				} else {
					fmt.Fprintf(os.Stdout, "%s: %s\n", name, binPath)
				}
			}
			return nil
		},
	}

	binaryListCmd := &cobra.Command{
		Use:   "list",
		Short: "List installed binaries",
		RunE: func(cmd *cobra.Command, args []string) error {
			core, err := buildCore(opts)
			if err != nil {
				return err
			}
			installed, err := core.Mgr.ListInstalled()
			if err != nil {
				return err
			}
			if len(installed) == 0 {
				fmt.Fprintln(os.Stdout, "No binaries installed. Use 'formatter binary install' to download.")
				return nil
			}
			for _, name := range installed {
				binPath, err := core.Mgr.FindBinary(name)
				if err != nil {
					fmt.Fprintf(os.Stdout, "%s: (error)\n", name)
				} else {
					fmt.Fprintf(os.Stdout, "%s: %s\n", name, binPath)
				}
			}
			return nil
		},
	}

	var removeTool string
	binaryRemoveCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a binary tool",
		RunE: func(cmd *cobra.Command, args []string) error {
			core, err := buildCore(opts)
			if err != nil {
				return err
			}
			if removeTool == "" {
				return fmt.Errorf("--tool is required")
			}
			if err := core.Mgr.RemoveBinary(removeTool); err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "Removed %s\n", removeTool)
			return nil
		},
	}
	binaryRemoveCmd.Flags().StringVar(&removeTool, "tool", "", "Tool name to remove")

	binaryUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "Update binary tools to configured versions",
		RunE: func(cmd *cobra.Command, args []string) error {
			core, err := buildCore(opts)
			if err != nil {
				return err
			}
			installed, err := core.Mgr.ListInstalled()
			if err != nil {
				return err
			}
			for _, name := range installed {
				if opts.Verbose {
					fmt.Fprintf(os.Stderr, "Updating %s...\n", name)
				}
				if err := core.Mgr.DownloadBinary(name); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to update %s: %v\n", name, err)
				} else if !opts.Quiet {
					fmt.Fprintf(os.Stdout, "Updated %s\n", name)
				}
			}
			return nil
		},
	}

	binaryCmd.AddCommand(binaryInstallCmd, binaryCheckCmd, binaryListCmd, binaryRemoveCmd, binaryUpdateCmd)
	return binaryCmd
}

func newLangsCmd(opts *GlobalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "langs",
		Short: "List supported languages",
		RunE: func(cmd *cobra.Command, args []string) error {
			core, err := buildCore(opts)
			if err != nil {
				return err
			}
			langs := core.ListLanguages()
			fmt.Fprintln(os.Stdout, "Supported languages:")
			for _, info := range langs {
				fmt.Fprintf(os.Stdout, "  %s", info.Lang)
				if info.Formatter != "" {
					fmt.Fprintf(os.Stdout, " [fmt: %s]", info.Formatter)
				}
				if info.Compressor != "" {
					fmt.Fprintf(os.Stdout, " [cmp: %s]", info.Compressor)
				}
				if info.Highlighter != "" {
					fmt.Fprintf(os.Stdout, " [hl: %s]", info.Highlighter)
				}
				fmt.Fprintln(os.Stdout)
			}
			return nil
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(os.Stdout, "formatter v%s\n", appcommon.Version)
		},
	}
}

func newServeCmd(opts *GlobalOptions) *cobra.Command {
	var (
		addr    string
		noOpen  bool
		browser bool
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "启动 Web UI 界面",
		Long: `启动图形界面，零门槛操作：
  formatter serve                      # 启动并打开原生体验窗口
  formatter serve --browser             # 使用默认浏览器打开
  formatter serve --addr :8080          # 自定义端口
  formatter serve --no-open             # 启动但不自动打开窗口`,
		RunE: func(cmd *cobra.Command, args []string) error {
			server, err := ui.NewServer(addr, browser)
			if err != nil {
				return fmt.Errorf("init UI server: %w", err)
			}
			server.SetNoOpen(noOpen)
			return server.Start()
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "", "Web UI 监听地址 (默认使用配置文件端口)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "不自动打开窗口")
	cmd.Flags().BoolVar(&browser, "browser", false, "使用默认浏览器打开（不使用 Chrome app 模式）")
	return cmd
}

func buildReaderWriter(opts *GlobalOptions, args []string) (iocore.InputReader, iocore.OutputWriter, error) {
	var reader iocore.InputReader
	var writer iocore.OutputWriter

	inputPath := opts.Input
	if len(args) > 0 && inputPath == "" {
		inputPath = args[0]
	}

	if opts.Clipboard {
		reader = iocore.NewClipboardReader()
		writer = iocore.NewClipboardWriter()
	} else {
		if inputPath != "" {
			reader = iocore.NewFileReader(inputPath)
		} else {
			reader = iocore.NewStdinReader()
		}
		if opts.Output != "" {
			writer = iocore.NewFileWriter(opts.Output)
		} else {
			writer = iocore.NewStdoutWriter()
		}
	}

	return reader, writer, nil
}

// resolveTools 解析要安装的工具列表 (配置驱动)
func resolveTools(lang, tools string, embeddedOnly bool) []string {
	// 1. 按名称筛选
	if tools != "" {
		names := splitAndTrim(tools, ",")
		if embeddedOnly {
			embeddedSet := make(map[string]bool)
			for _, n := range binary.ListEmbedded() {
				embeddedSet[n] = true
			}
			filtered := make([]string, 0, len(names))
			for _, n := range names {
				if embeddedSet[n] {
					filtered = append(filtered, n)
				}
			}
			return filtered
		}
		return names
	}

	// 2. 从配置注册表获取所有工具
	allTools := binary.List()

	// 3. 按语言筛选
	if lang != "" {
		langTools := make([]string, 0)
		for _, name := range allTools {
			if meta, err := binary.Get(name); err == nil {
				for _, l := range meta.Languages {
					if l == lang {
						langTools = append(langTools, name)
						break
					}
				}
			}
		}
		allTools = langTools
	}

	// 4. 按 embedded 标记筛选
	if embeddedOnly {
		embeddedSet := make(map[string]bool)
		for _, n := range binary.ListEmbedded() {
			embeddedSet[n] = true
		}
		result := make([]string, 0, len(allTools))
		for _, name := range allTools {
			if embeddedSet[name] {
				result = append(result, name)
			}
		}
		return result
	}

	return allTools
}

func splitAndTrim(s, sep string) []string {
	parts := make([]string, 0)
	for _, p := range strings.Split(s, sep) {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}
