package tool

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/formatter/formatter/src/binary"
	"github.com/formatter/formatter/src/config"
	"github.com/formatter/formatter/src/compressor"
)

// CmdCompressAdapter 是配置驱动的外部二进制压缩适配器。
// 命令参数模板从配置文件读取，由 config.ToolConfig.ExpandCmdArgs 解析。
type CmdCompressAdapter struct {
	toolName  string
	lang      string
	cfg       *config.ToolConfig // 工具配置 (包含 cmd 模板)
	fullCfg   *config.Config     // 完整配置 (用于解析 {ext} 等语言相关占位符)
	binaryMgr *binary.Manager
}

func NewCmdCompressAdapter(toolName, lang string, cfg *config.ToolConfig, fullCfg *config.Config, mgr *binary.Manager) *CmdCompressAdapter {
	return &CmdCompressAdapter{
		toolName:  toolName,
		lang:      lang,
		cfg:       cfg,
		fullCfg:   fullCfg,
		binaryMgr: mgr,
	}
}

func (c *CmdCompressAdapter) Compress(input []byte, lang string, opts compressor.CompressOptions) ([]byte, error) {
	// 所有参数从配置文件的 cmd 模板构建，无硬编码回退
	args := c.buildArgs(opts)

	// 使用通用命令构建 (自动处理运行时前缀)
	cmdPath, cmdArgs, err := c.binaryMgr.BuildToolCmd(c.toolName, args...)
	if err != nil {
		return nil, fmt.Errorf("build tool command: %w", err)
	}

	cmd := exec.Command(cmdPath, cmdArgs...)
	cmd.Stdin = bytes.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s execution failed: %s: %w", c.toolName, stderr.String(), err)
	}

	return stdout.Bytes(), nil
}

func (c *CmdCompressAdapter) buildArgs(opts compressor.CompressOptions) []string {
	if c.cfg == nil || len(c.cfg.Cmd) == 0 {
		return nil
	}

	// 使用完整配置的浅拷贝，保留 Languages 字段以正确解析 {ext} 等占位符
	cfgCopy := *c.fullCfg
	cfgCopy.Format.TabWidth = 2 // 压缩器通常不需要 tab width，但模板可能引用

	args := c.cfg.ExpandCmdArgs(&cfgCopy, c.lang)

	// 应用 opts.Extra 覆盖 (与 formatter cmd_adapter 一致)
	for k, v := range opts.Extra {
		placeholder := "{" + k + "}"
		val := fmt.Sprintf("%v", v)
		for i, arg := range args {
			args[i] = strings.ReplaceAll(arg, placeholder, val)
		}
	}

	return args
}

func (c *CmdCompressAdapter) SupportedLanguages() []string {
	return []string{c.lang}
}

func (c *CmdCompressAdapter) Name() string {
	return "external-" + c.toolName
}

func (c *CmdCompressAdapter) Type() compressor.BackendType {
	return compressor.ToolBinary
}
