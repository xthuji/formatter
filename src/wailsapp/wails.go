package wailsapp

import (
	"context"
	"fmt"
	"os"

	"github.com/formatter/formatter/src/appcommon"
	"github.com/formatter/formatter/src/config"
	iocore "github.com/formatter/formatter/src/io"
	"github.com/formatter/formatter/src/ui"
)

// 共享类型别名 (保持 Wails 生成的 TypeScript 绑定不变)
type (
	RuntimeInfo     = appcommon.RuntimeInfo
	BinInfo         = appcommon.BinInfo
	LanguageInfo    = appcommon.LanguageInfo
	BinActionResult = appcommon.BinActionResult
	FormatRequest   = appcommon.FormatRequest
	RunRequest      = appcommon.RunRequest
	StatusResult    = appcommon.StatusResult
)

// Wails 绑定专用的包装类型
type FormatResult struct {
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
	Result   string `json:"result,omitempty"`
	Language string `json:"language,omitempty"`
}

type LanguagesResult struct {
	Success bool           `json:"success"`
	Data    []LanguageInfo `json:"data"`
}

type BinListResult struct {
	Success bool      `json:"success"`
	Data    []BinInfo `json:"data"`
}

type ConfigResult struct {
	Success bool                   `json:"success"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Error   string                 `json:"error,omitempty"`
	Result  string                 `json:"result,omitempty"`
}

type DetectResult struct {
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
	Language string `json:"language,omitempty"`
}

type App struct {
	*appcommon.Core
}

func NewApp() *App {
	return &App{Core: appcommon.NewCore("")}
}

// startup 在 Wails 窗口启动时同时启动嵌入式 Web Server，
// 使配置端口 (config.json → server.port) 上的 Web UI 也可通过浏览器访问。
// Web Server 在后台 goroutine 中运行，不阻塞 Wails 主线程；
// noOpen=true 跳过自动打开窗口 (Wails 原生窗口已展示)。
func (a *App) startup(ctx context.Context) {
	go func() {
		server, err := ui.NewServer("", true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[wails] 启动嵌入式 Web Server 失败: %v\n", err)
			return
		}
		server.SetNoOpen(true)
		if err := server.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "[wails] Web Server 运行结束: %v\n", err)
		}
	}()
}

// GetWindowConfig 返回 App 桌面窗口尺寸配置 (供 main 包启动时使用)
func (a *App) GetWindowConfig() config.WindowConfig {
	return a.Cfg.Window
}

func (a *App) shutdown(ctx context.Context) {}

func (a *App) Format(req FormatRequest) FormatResult {
	result, err := a.ExecutePipeline(context.Background(), req.Code, a.FormatParams(appcommon.PipelineOptions{
		Language:           req.Language,
		TabWidth:           req.TabWidth,
		FormatterBackend:   req.FormatterBackend,
		CompressorBackend:  req.CompressorBackend,
		HighlighterBackend: req.HighlighterBackend,
	}))
	if err != nil {
		return FormatResult{Success: false, Error: err.Error()}
	}
	return FormatResult{Success: true, Result: result, Language: req.Language}
}

func (a *App) Compress(req FormatRequest) FormatResult {
	result, err := a.ExecutePipeline(context.Background(), req.Code, a.CompressParams(appcommon.PipelineOptions{
		Language:           req.Language,
		FormatterBackend:   req.FormatterBackend,
		CompressorBackend:  req.CompressorBackend,
		HighlighterBackend: req.HighlighterBackend,
	}))
	if err != nil {
		return FormatResult{Success: false, Error: err.Error()}
	}
	return FormatResult{Success: true, Result: result, Language: req.Language}
}

func (a *App) Highlight(req FormatRequest) FormatResult {
	result, err := a.ExecutePipeline(context.Background(), req.Code, a.HighlightParams(appcommon.PipelineOptions{
		Language:           req.Language,
		Style:              req.Style,
		FontSize:           req.FontSize,
		LineNumbers:        req.LineNumbers,
		CompatHTML:         req.CompatHTML,
		FormatterBackend:   req.FormatterBackend,
		CompressorBackend:  req.CompressorBackend,
		HighlighterBackend: req.HighlighterBackend,
	}))
	if err != nil {
		return FormatResult{Success: false, Error: err.Error()}
	}
	return FormatResult{Success: true, Result: result, Language: req.Language}
}

func (a *App) Run(req RunRequest) FormatResult {
	result, err := a.ExecutePipeline(context.Background(), req.Code, a.RunParams(appcommon.PipelineOptions{
		Language:           req.Language,
		TabWidth:           req.TabWidth,
		Style:              req.Style,
		FontSize:           req.FontSize,
		LineNumbers:        req.LineNumbers,
		CompatHTML:         req.CompatHTML,
		NoFormat:           req.NoFormat,
		NoCompress:         req.NoCompress,
		NoHighlight:        req.NoHighlight,
		FormatterBackend:   req.FormatterBackend,
		CompressorBackend:  req.CompressorBackend,
		HighlighterBackend: req.HighlighterBackend,
	}))
	if err != nil {
		return FormatResult{Success: false, Error: err.Error()}
	}
	return FormatResult{Success: true, Result: result, Language: req.Language}
}

// Detect 根据代码内容自动检测编程语言
func (a *App) Detect(code string) DetectResult {
	lang := iocore.DetectLanguage("", []byte(code))
	return DetectResult{Success: true, Language: lang}
}

func (a *App) GetLanguages() LanguagesResult {
	return LanguagesResult{Success: true, Data: a.ListLanguages()}
}

func (a *App) GetStatus() StatusResult {
	return a.Core.GetStatus()
}

func (a *App) GetBinaries() BinListResult {
	return BinListResult{Success: true, Data: a.ListBinaries()}
}

func (a *App) InstallBinary(name string) BinActionResult {
	return a.Core.InstallBinary(name)
}

func (a *App) UninstallBinary(name string) BinActionResult {
	return a.Core.UninstallBinary(name)
}

func (a *App) VerifyBinary(name string) BinActionResult {
	return a.Core.VerifyBinary(name)
}

func (a *App) InstallRuntime(name string) BinActionResult {
	return a.Core.InstallRuntime(name)
}

func (a *App) GetConfig() ConfigResult {
	return ConfigResult{Success: true, Data: appcommon.ConfigToMap(a.Cfg)}
}

func (a *App) SaveConfig(data map[string]interface{}) ConfigResult {
	if err := appcommon.UpdateConfigFromMap(a.Cfg, data); err != nil {
		return ConfigResult{Success: false, Error: err.Error()}
	}
	return ConfigResult{Success: true, Result: "配置已更新"}
}

// ---- Runtime CRUD ----

func (a *App) AddRuntime(data map[string]interface{}) ConfigResult {
	if err := a.Core.AddRuntime(data); err != nil {
		return ConfigResult{Success: false, Error: err.Error()}
	}
	return ConfigResult{Success: true, Result: "运行时已添加"}
}

func (a *App) UpdateRuntime(data map[string]interface{}) ConfigResult {
	if err := a.Core.UpdateRuntime(data); err != nil {
		return ConfigResult{Success: false, Error: err.Error()}
	}
	return ConfigResult{Success: true, Result: "运行时已更新"}
}

func (a *App) DeleteRuntime(name string) ConfigResult {
	if err := a.Core.DeleteRuntime(name); err != nil {
		return ConfigResult{Success: false, Error: err.Error()}
	}
	return ConfigResult{Success: true, Result: "运行时已删除"}
}

// ---- Tool CRUD ----

func (a *App) AddTool(data map[string]interface{}) ConfigResult {
	if err := a.Core.AddTool(data); err != nil {
		return ConfigResult{Success: false, Error: err.Error()}
	}
	return ConfigResult{Success: true, Result: "工具已添加"}
}

func (a *App) UpdateTool(data map[string]interface{}) ConfigResult {
	if err := a.Core.UpdateTool(data); err != nil {
		return ConfigResult{Success: false, Error: err.Error()}
	}
	return ConfigResult{Success: true, Result: "工具已更新"}
}

func (a *App) DeleteTool(name string) ConfigResult {
	if err := a.Core.DeleteTool(name); err != nil {
		return ConfigResult{Success: false, Error: err.Error()}
	}
	return ConfigResult{Success: true, Result: "工具已删除"}
}

// ---- Language CRUD ----

func (a *App) AddLanguage(data map[string]interface{}) ConfigResult {
	if err := a.Core.AddLanguage(data); err != nil {
		return ConfigResult{Success: false, Error: err.Error()}
	}
	return ConfigResult{Success: true, Result: "语言已添加"}
}

func (a *App) UpdateLanguage(data map[string]interface{}) ConfigResult {
	if err := a.Core.UpdateLanguage(data); err != nil {
		return ConfigResult{Success: false, Error: err.Error()}
	}
	return ConfigResult{Success: true, Result: "语言已更新"}
}

func (a *App) DeleteLanguage(name string) ConfigResult {
	if err := a.Core.DeleteLanguage(name); err != nil {
		return ConfigResult{Success: false, Error: err.Error()}
	}
	return ConfigResult{Success: true, Result: "语言已删除"}
}
