package appcommon

import "strings"

// Version 工具版本号。
// 数据源为项目根目录 VERSION 文件，由构建脚本通过 ldflags
// (-X formatter/src/appcommon.Version=<ver>) 注入，
// 与 xgames 的版本管理方式一致。默认值 "dev" 仅用于未经 ldflags 注入的开发/测试构建。
var Version = "dev"

// SetVersion 以编程方式注入版本号 (主要用于测试，从根目录 VERSION 文件读取)。
// 空字符串或纯空白将被忽略，保留现有值。
func SetVersion(v string) {
	v = strings.TrimSpace(v)
	if v != "" {
		Version = v
	}
}
