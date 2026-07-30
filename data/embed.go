// Package data 提供 //go:embed 嵌入的默认配置。
// 放在此处是因为 //go:embed 不支持 ".." 路径，
// 将 embed 声明与被嵌入的 config.json 放在同一目录可自然解决该限制。
package data

import _ "embed"

// ConfigJSON 是 data/config.json 的嵌入字节切片，
// 由 main 包启动时通过 config.SetDefaultConfig() 注入。
//go:embed config.json
var ConfigJSON []byte

// VersionText 是 data/version.txt 的嵌入字节切片，
// 由 main 包启动时通过 appcommon.SetVersion() 注入。
// version.txt 是全应用唯一的版本号数据源 (Go/前端/构建脚本均从此获取)。
//go:embed version.txt
var VersionText []byte
