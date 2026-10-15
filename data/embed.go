// Package data 提供 //go:embed 嵌入的默认配置。
// 放在此处是因为 //go:embed 不支持 ".." 路径，
// 将 embed 声明与被嵌入的 config.json 放在同一目录可自然解决该限制。
package data

import _ "embed"

// ConfigJSON 是 data/config.json 的嵌入字节切片，
// 由 main 包启动时通过 config.SetDefaultConfig() 注入。
//go:embed config.json
var ConfigJSON []byte

// 版本号不再在此嵌入：改为项目根目录 VERSION 文件 + 构建时 ldflags 注入
// (见 scripts/run_tools.sh 的 -X appcommon.Version)，与 xgames 的版本管理方式一致。
