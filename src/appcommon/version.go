package appcommon

import "strings"

// Version 工具版本号。
// 原始数据源为 data/version.txt (通过 //go:embed 嵌入)，
// 由 main 包启动时通过 SetVersion() 注入。
// 默认值 "1.0.0" 仅作为嵌入失败时的回退。
var Version = "1.0.0"

// SetVersion 注入版本号 (由 main 包调用，源自 //go:embed data/version.txt)。
// 空字符串或纯空白将被忽略，保留现有值。
func SetVersion(v string) {
	v = strings.TrimSpace(v)
	if v != "" {
		Version = v
	}
}
