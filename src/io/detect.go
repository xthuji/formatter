package iocore

import (
	"bytes"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/formatter/formatter/src/config"
)

// compiledRule 是一条已编译的检测规则 (shebang 或内容正则)。
type compiledRule struct {
	lang string
	re   *regexp.Regexp
}

// detector 持有从配置编译的检测规则，按扩展名/shebang/内容三类组织。
type detector struct {
	extMap       map[string]string // 扩展名 (小写含点) -> 语言
	shebangRules []compiledRule    // shebang 正则
	contentRules []compiledRule    // 内容正则 (按 priority 排序)
}

// 活动检测器: 由 appcommon 在配置加载/变更后通过 SetDetectionRules 注入。
// 未注入时为空检测器，DetectLanguage 将始终返回空字符串。
var (
	detectorMu  sync.RWMutex
	active      = &detector{extMap: map[string]string{}}
)

// SetDetectionRules 根据配置中的语言定义重建检测规则。
// 由 appcommon 在启动 (NewCore) 和配置变更 (reload) 后调用。
// 无效的正则会被跳过并忽略，不影响其它规则。
func SetDetectionRules(languages map[string]config.LangConfig) {
	d := &detector{extMap: make(map[string]string)}

	type langPrio struct {
		lang     string
		priority int
	}
	var ordered []langPrio

	for lang, lc := range languages {
		if lc.Detection == nil {
			continue
		}
		dc := lc.Detection

		// 扩展名映射 (统一小写)
		for _, ext := range dc.Extensions {
			e := strings.ToLower(strings.TrimSpace(ext))
			if e != "" {
				d.extMap[e] = lang
			}
		}

		// shebang 正则
		for _, s := range dc.Shebangs {
			if re, err := regexp.Compile(s); err == nil {
				d.shebangRules = append(d.shebangRules, compiledRule{lang, re})
			}
		}

		// 收集有内容规则的语言，稍后按优先级排序
		if len(dc.Content) > 0 {
			ordered = append(ordered, langPrio{lang, dc.Priority})
		}
	}

	// 按 (priority, lang) 排序: priority 小者先匹配; 同优先级按语言名字母序保证确定性
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].priority != ordered[j].priority {
			return ordered[i].priority < ordered[j].priority
		}
		return ordered[i].lang < ordered[j].lang
	})
	for _, op := range ordered {
		for _, c := range languages[op.lang].Detection.Content {
			if re, err := regexp.Compile(c); err == nil {
				d.contentRules = append(d.contentRules, compiledRule{op.lang, re})
			}
		}
	}

	detectorMu.Lock()
	active = d
	detectorMu.Unlock()
}

// DetectLanguage 根据文件路径和/或内容检测编程语言，返回内部语言标识。
//
// 检测策略 (按优先级):
//  1. 扩展名: 文件扩展名直接映射 (扩展名优先于 shebang，符合"按文件类型格式化"语义)
//  2. shebang: 内容首行以 #! 开头时，匹配 shebang 正则
//  3. 内容强信号正则: 对内容前 2048 字节按优先级顺序匹配
//
// 所有规则均来自 config.json 的 languages.*.detection 配置，代码中不硬编码任何语言规则。
// 未识别或未配置检测规则时返回空字符串。
func DetectLanguage(filePath string, content []byte) string {
	detectorMu.RLock()
	d := active
	detectorMu.RUnlock()

	// 1. 扩展名优先
	if filePath != "" {
		ext := strings.ToLower(filepath.Ext(filePath))
		if lang, ok := d.extMap[ext]; ok {
			return lang
		}
	}

	// 2. shebang
	if lang := d.detectByShebang(content); lang != "" {
		return lang
	}

	// 3. 内容强信号正则
	if lang := d.detectByContent(content); lang != "" {
		return lang
	}

	return ""
}

// detectByShebang 检测首行 shebang。仅当内容以 #! 开头时匹配。
func (d *detector) detectByShebang(content []byte) string {
	if len(content) == 0 || content[0] != '#' {
		return ""
	}
	firstLine := content
	if idx := bytes.IndexByte(content, '\n'); idx >= 0 {
		firstLine = content[:idx]
	}
	line := string(firstLine)
	for _, r := range d.shebangRules {
		if r.re.MatchString(line) {
			return r.lang
		}
	}
	return ""
}

// detectByContent 对内容前 2048 字节按优先级顺序匹配强信号正则。
func (d *detector) detectByContent(content []byte) string {
	snippet := content
	if len(snippet) > 2048 {
		snippet = snippet[:2048]
	}
	text := string(snippet)
	for _, r := range d.contentRules {
		if r.re.MatchString(text) {
			return r.lang
		}
	}
	return ""
}
