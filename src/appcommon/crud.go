package appcommon

import (
	"encoding/json"
	"fmt"

	"github.com/formatter/formatter/src/binary"
	"github.com/formatter/formatter/src/config"
	iocore "github.com/formatter/formatter/src/io"
	"github.com/formatter/formatter/src/pipeline"
)

// reload 在配置变更 (CRUD) 后重建二进制注册表、格式化/压缩注册表和流水线引擎。
func (c *Core) reload() {
	binary.LoadFromConfig(c.Cfg)
	c.Reg = BuildRegistry(c.Mgr, c.Cfg)
	c.Engine = pipeline.New(c.Reg, c.Cfg)
	// 语言配置变更后同步刷新检测规则
	iocore.SetDetectionRules(c.Cfg.Languages)
}

// saveAndReload 保存配置并重建引擎 (所有 CRUD 操作共用的收尾步骤)。
func (c *Core) saveAndReload() error {
	if err := config.Save(c.Cfg, ""); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	c.reload()
	return nil
}

// ---- Runtime CRUD ----

// AddRuntime 添加一个新的运行时环境定义。
func (c *Core) AddRuntime(data map[string]interface{}) error {
	rd, err := parseRuntimeDef(data)
	if err != nil {
		return fmt.Errorf("解析运行时数据失败: %w", err)
	}
	if rd.Name == "" {
		return fmt.Errorf("运行时名称不能为空")
	}
	for _, existing := range c.Cfg.Binary.Runtimes {
		if existing.Name == rd.Name {
			return fmt.Errorf("运行时 %s 已存在", rd.Name)
		}
	}
	c.Cfg.Binary.Runtimes = append(c.Cfg.Binary.Runtimes, rd)
	return c.saveAndReload()
}

// UpdateRuntime 更新指定的运行时环境定义 (按 name 匹配)。
func (c *Core) UpdateRuntime(data map[string]interface{}) error {
	rd, err := parseRuntimeDef(data)
	if err != nil {
		return fmt.Errorf("解析运行时数据失败: %w", err)
	}
	if rd.Name == "" {
		return fmt.Errorf("运行时名称不能为空")
	}
	for i := range c.Cfg.Binary.Runtimes {
		if c.Cfg.Binary.Runtimes[i].Name == rd.Name {
			c.Cfg.Binary.Runtimes[i] = rd
			return c.saveAndReload()
		}
	}
	return fmt.Errorf("运行时 %s 不存在", rd.Name)
}

// DeleteRuntime 删除指定名称的运行时环境定义。
func (c *Core) DeleteRuntime(name string) error {
	for i, existing := range c.Cfg.Binary.Runtimes {
		if existing.Name == name {
			c.Cfg.Binary.Runtimes = append(c.Cfg.Binary.Runtimes[:i], c.Cfg.Binary.Runtimes[i+1:]...)
			return c.saveAndReload()
		}
	}
	return fmt.Errorf("运行时 %s 不存在", name)
}

// ---- Tool CRUD ----

// AddTool 添加一个新的二进制工具定义。
func (c *Core) AddTool(data map[string]interface{}) error {
	td, err := parseToolDef(data)
	if err != nil {
		return fmt.Errorf("解析工具数据失败: %w", err)
	}
	if td.Name == "" {
		return fmt.Errorf("工具名称不能为空")
	}
	for _, existing := range c.Cfg.Binary.Tools {
		if existing.Name == td.Name {
			return fmt.Errorf("工具 %s 已存在", td.Name)
		}
	}
	c.Cfg.Binary.Tools = append(c.Cfg.Binary.Tools, td)
	return c.saveAndReload()
}

// UpdateTool 更新指定的二进制工具定义 (按 name 匹配)。
func (c *Core) UpdateTool(data map[string]interface{}) error {
	td, err := parseToolDef(data)
	if err != nil {
		return fmt.Errorf("解析工具数据失败: %w", err)
	}
	if td.Name == "" {
		return fmt.Errorf("工具名称不能为空")
	}
	for i := range c.Cfg.Binary.Tools {
		if c.Cfg.Binary.Tools[i].Name == td.Name {
			c.Cfg.Binary.Tools[i] = td
			return c.saveAndReload()
		}
	}
	return fmt.Errorf("工具 %s 不存在", td.Name)
}

// DeleteTool 删除指定名称的二进制工具定义。
func (c *Core) DeleteTool(name string) error {
	for i, existing := range c.Cfg.Binary.Tools {
		if existing.Name == name {
			c.Cfg.Binary.Tools = append(c.Cfg.Binary.Tools[:i], c.Cfg.Binary.Tools[i+1:]...)
			return c.saveAndReload()
		}
	}
	return fmt.Errorf("工具 %s 不存在", name)
}

// ---- Language CRUD ----

// AddLanguage 添加一个新的编程语言配置。
func (c *Core) AddLanguage(data map[string]interface{}) error {
	lang, ok := data["lang"].(string)
	if !ok || lang == "" {
		return fmt.Errorf("语言名称不能为空")
	}
	if _, exists := c.Cfg.Languages[lang]; exists {
		return fmt.Errorf("语言 %s 已存在", lang)
	}
	lc, err := parseLangConfig(data, config.LangConfig{})
	if err != nil {
		return fmt.Errorf("解析语言配置失败: %w", err)
	}
	c.Cfg.Languages[lang] = lc
	return c.saveAndReload()
}

// UpdateLanguage 更新指定的编程语言配置。
func (c *Core) UpdateLanguage(data map[string]interface{}) error {
	lang, ok := data["lang"].(string)
	if !ok || lang == "" {
		return fmt.Errorf("语言名称不能为空")
	}
	lc, err := parseLangConfig(data, c.Cfg.Languages[lang])
	if err != nil {
		return fmt.Errorf("解析语言配置失败: %w", err)
	}
	c.Cfg.Languages[lang] = lc
	return c.saveAndReload()
}

// DeleteLanguage 删除指定名称的编程语言配置。
func (c *Core) DeleteLanguage(name string) error {
	if _, exists := c.Cfg.Languages[name]; !exists {
		return fmt.Errorf("语言 %s 不存在", name)
	}
	delete(c.Cfg.Languages, name)
	return c.saveAndReload()
}

// ---- Parsing helpers ----

func parseRuntimeDef(data map[string]interface{}) (config.RuntimeDef, error) {
	bytes, err := json.Marshal(data)
	if err != nil {
		return config.RuntimeDef{}, err
	}
	var rd config.RuntimeDef
	if err := json.Unmarshal(bytes, &rd); err != nil {
		return config.RuntimeDef{}, err
	}
	return rd, nil
}

func parseToolDef(data map[string]interface{}) (config.ToolDef, error) {
	bytes, err := json.Marshal(data)
	if err != nil {
		return config.ToolDef{}, err
	}
	var td config.ToolDef
	if err := json.Unmarshal(bytes, &td); err != nil {
		return config.ToolDef{}, err
	}
	return td, nil
}

// parseLangConfig 从 map 解析语言配置 (formatter/compressor/highlighter/indent/detection)。
// indent 与 detection 优先从 data 解析 (UI 编辑表单已支持这两项)；
// 若 data 中未提供则保留 existing 中的值，避免更新时丢失配置。
func parseLangConfig(data map[string]interface{}, existing config.LangConfig) (config.LangConfig, error) {
	lc := config.LangConfig{
		Indent:    existing.Indent,
		Detection: existing.Detection,
	}
	if fmtRaw, ok := data["formatter"]; ok && fmtRaw != nil {
		lc.Formatter = ParseToolConfig(fmtRaw)
	}
	if cmpRaw, ok := data["compressor"]; ok && cmpRaw != nil {
		lc.Compressor = ParseToolConfig(cmpRaw)
	}
	if hlRaw, ok := data["highlighter"]; ok && hlRaw != nil {
		lc.Highlighter = ParseToolConfig(hlRaw)
	}
	if indentRaw, ok := data["indent"]; ok && indentRaw != nil {
		newIndent := parseIndentConfig(indentRaw)
		// 保留 existing 中的 customIndentFrom (UI 编辑表单不包含此字段)
		if newIndent != nil && newIndent.CustomIndentFrom == nil && existing.Indent != nil {
			newIndent.CustomIndentFrom = existing.Indent.CustomIndentFrom
		}
		lc.Indent = newIndent
	}
	if detectRaw, ok := data["detection"]; ok && detectRaw != nil {
		lc.Detection = parseDetectionConfig(detectRaw)
	}
	return lc, nil
}

// parseIndentConfig 从 map 解析缩进配置 (单字段: tab_width, -1=Tab 缩进, >0=空格缩进宽度, 0=动态)。
// 兼容旧配置中的 use_tabs 字段: use_tabs=true 时转换为 tab_width=-1。
func parseIndentConfig(raw interface{}) *config.IndentConfig {
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	ic := &config.IndentConfig{}
	tabWidthSet := false
	if v, ok := m["tab_width"]; ok {
		switch n := v.(type) {
		case float64:
			ic.TabWidth = int(n)
			tabWidthSet = true
		case int:
			ic.TabWidth = n
			tabWidthSet = true
		}
	}
	// 兼容旧配置: 若未设置 tab_width 但显式 use_tabs=true, 则视为 Tab 缩进 (tab_width=-1)
	if !tabWidthSet {
		if v, ok := m["use_tabs"].(bool); ok && v {
			ic.TabWidth = -1
		}
	}
	// 解析 customIndentFrom (递归)
	if fromRaw, ok := m["customIndentFrom"]; ok && fromRaw != nil {
		ic.CustomIndentFrom = parseIndentConfig(fromRaw)
	}
	return ic
}

// parseDetectionConfig 从 map 解析语言检测规则配置。
func parseDetectionConfig(raw interface{}) *config.DetectionConfig {
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	dc := &config.DetectionConfig{}
	dc.Extensions = toStringSlice(m["extensions"])
	dc.Shebangs = toStringSlice(m["shebangs"])
	dc.Content = toStringSlice(m["content"])
	if v, ok := m["priority"]; ok {
		switch n := v.(type) {
		case float64:
			dc.Priority = int(n)
		case int:
			dc.Priority = n
		}
	}
	return dc
}

// toStringSlice 将 interface{} (通常为 []interface{}) 转换为 []string，跳过空值。
func toStringSlice(raw interface{}) []string {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok && s != "" {
			result = append(result, s)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
