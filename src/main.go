package main

import (
	"embed"
	"fmt"
	"os"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/formatter/formatter/data"
	"github.com/formatter/formatter/src/appcommon"
	"github.com/formatter/formatter/src/config"
	"github.com/formatter/formatter/src/wailsapp"
)

//go:embed all:ui/static
var frontendFS embed.FS

// cliCommands 是支持的所有 CLI 子命令，用于判断是否以 CLI 模式运行
var cliCommands = map[string]bool{
	"format": true, "compress": true, "highlight": true, "run": true,
	"binary": true, "langs": true, "version": true, "serve": true,
	"help": true, "-h": true, "--help": true,
}

// globalFlagsWithValue 需要跳过值的全局标志集合
var globalFlagsWithValue = map[string]bool{
	"-l": true, "--lang": true,
	"-i": true, "--input": true,
	"-o": true, "--output": true,
	"--tab-width": true,
	"--line-ending": true,
	"--style": true,
	"--formatter-backend": true,
	"--compressor-backend": true,
	"--highlighter-backend": true,
	"--config": true,
	"--bin-dir": true,
}

func main() {
	// 注入默认配置 (源自 //go:embed data/config.json)
	config.SetDefaultConfig(data.ConfigJSON)
	// 注入版本号 (源自 //go:embed data/version.txt，全应用唯一版本数据源)
	appcommon.SetVersion(string(data.VersionText))

	// 检测是否为 CLI 模式：扫描参数，跳过全局标志及其值，查找是否包含已知 CLI 子命令
	if isCLIMode() {
		if err := NewRootCmd().Execute(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 否则以 Wails 桌面应用模式运行
	runWails()
}

// isCLIMode 检测是否应该以 CLI 模式运行
// 遍历 os.Args，跳过全局标志（带值的标志额外跳过一个参数），查找是否有已知 CLI 子命令
// 若第一个位置参数不是已知子命令，报错退出而非降级到 GUI
func isCLIMode() bool {
	if len(os.Args) < 2 {
		return false
	}
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		// 遇到非标志参数，检查是否为 CLI 子命令
		if !strings.HasPrefix(arg, "-") {
			if cliCommands[arg] {
				return true
			}
			// 未知子命令: 报错而非静默降级到 GUI
			fmt.Fprintf(os.Stderr, "Error: unknown command %q. Run 'formatter help' for usage.\n", arg)
			os.Exit(1)
		}
		// 是标志，检查是否需要跳过下一个参数（标志的值）
		if globalFlagsWithValue[arg] {
			i++ // 跳过标志的值
		}
	}
	return false
}

func runWails() {
	app := wailsapp.NewApp()

	// 从配置文件读取窗口尺寸
	w := app.GetWindowConfig()

	opts := &options.App{
		Title:            "Formatter - 代码格式化工具",
		Width:            w.Width,
		Height:           w.Height,
		AssetServer:      &assetserver.Options{Assets: frontendFS},
		BackgroundColour: &options.RGBA{R: 240, G: 242, B: 245, A: 1},
		Bind: []interface{}{
			app,
		},
	}

	err := wails.Run(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
