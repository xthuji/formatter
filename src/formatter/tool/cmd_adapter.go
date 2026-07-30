package tool

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/formatter/formatter/src/binary"
	"github.com/formatter/formatter/src/config"
	"github.com/formatter/formatter/src/formatter"
)

// CmdAdapter 是配置驱动的通用外部二进制格式化适配器。
// 命令参数模板从配置文件读取，由 config.ToolConfig.ExpandCmdArgs 解析。
type CmdAdapter struct {
	toolName  string
	lang      string
	cfg       *config.ToolConfig // 工具配置 (包含 cmd 模板)
	fullCfg   *config.Config     // 完整配置 (用于解析 {ext} 等语言相关占位符)
	binaryMgr *binary.Manager
	indentCfg *config.IndentConfig // 语言缩进配置 (包含 customIndentFrom)
}

// NewCmdAdapter 创建一个配置驱动的格式化适配器。
// toolName: 二进制工具名称 (如 "stylua", "rustfmt")
// lang: 目标语言
// cfg: 工具配置 (包含 cmd 模板)
// fullCfg: 完整配置 (用于解析 {ext} 等语言相关占位符)
// mgr: 二进制管理器
// indentCfg: 语言缩进配置 (可选，用于 customIndentFrom 后处理)
func NewCmdAdapter(toolName, lang string, cfg *config.ToolConfig, fullCfg *config.Config, mgr *binary.Manager, indentCfg *config.IndentConfig) *CmdAdapter {
	return &CmdAdapter{
		toolName:  toolName,
		lang:      lang,
		cfg:       cfg,
		fullCfg:   fullCfg,
		binaryMgr: mgr,
		indentCfg: indentCfg,
	}
}

func (a *CmdAdapter) Format(input []byte, lang string, opts formatter.FormatOptions) ([]byte, error) {
	// 所有参数从配置文件的 cmd 模板构建，无硬编码回退
	args := a.buildArgs(opts)

	// 使用通用命令构建 (自动处理运行时前缀，如 java -jar, node 等)
	cmdPath, cmdArgs, err := a.binaryMgr.BuildToolCmd(a.toolName, args...)
	if err != nil {
		return nil, fmt.Errorf("build tool command: %w", err)
	}

	cmd := exec.Command(cmdPath, cmdArgs...)
	cmd.Stdin = bytes.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// 包含输入内容摘要以便诊断 (语言 + 前 80 字符)
		preview := string(input)
		if len(preview) > 80 {
			preview = preview[:80] + "..."
		}
		preview = strings.ReplaceAll(preview, "\n", "\\n")
		return nil, fmt.Errorf("%s execution failed (lang=%s, input=%d bytes, preview=%q): %s: %w",
			a.toolName, a.lang, len(input), preview, stderr.String(), err)
	}

	output := stdout.Bytes()

	// 如果配置了 customIndentFrom，执行缩进转换后处理
	if a.indentCfg != nil && a.indentCfg.CustomIndentFrom != nil {
		from := formatter.IndentSpec{
			TabWidth: a.indentCfg.CustomIndentFrom.TabWidth,
		}
		to := formatter.IndentSpec{
			TabWidth: opts.TabWidth,
		}
		converted := formatter.ConvertIndent(string(output), from, to)
		output = []byte(converted)
	}

	return output, nil
}

func (a *CmdAdapter) buildArgs(opts formatter.FormatOptions) []string {
	if a.cfg == nil || len(a.cfg.Cmd) == 0 {
		return nil
	}

	// 使用完整配置的浅拷贝，覆盖用户指定的缩进选项
	// 必须保留 Languages 字段，否则 {ext} 占位符会因找不到语言配置而回退为 "txt"
	cfgCopy := *a.fullCfg
	cfgCopy.Format.TabWidth = opts.TabWidth
	cfgCopy.Format.LineEnding = opts.LineEnding

	args := a.cfg.ExpandCmdArgs(&cfgCopy, a.lang)

	// 应用 opts.Extra 覆盖
	for k, v := range opts.Extra {
		placeholder := "{" + k + "}"
		val := fmt.Sprintf("%v", v)
		for i, arg := range args {
			args[i] = strings.ReplaceAll(arg, placeholder, val)
		}
	}

	return args
}

func (a *CmdAdapter) SupportedLanguages() []string {
	return []string{a.lang}
}

func (a *CmdAdapter) Name() string {
	return "external-" + a.toolName
}

func (a *CmdAdapter) Type() formatter.BackendType {
	return formatter.ToolBinary
}
