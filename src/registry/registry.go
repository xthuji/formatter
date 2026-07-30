package registry

import (
	"sort"
	"sync"

	"github.com/formatter/formatter/src/compressor"
	"github.com/formatter/formatter/src/formatter"
	"github.com/formatter/formatter/src/highlighter"
)

type Registry struct {
	mu           sync.RWMutex
	formatters   map[string][]formatter.Formatter
	compressors  map[string][]compressor.Compressor
	highlighters map[string][]highlighter.Highlighter
	aliases      map[string]string // 语言别名 → 规范语言名 (仅 formatter/compressor 生效)
}

func New() *Registry {
	return &Registry{
		formatters:   make(map[string][]formatter.Formatter),
		compressors:  make(map[string][]compressor.Compressor),
		highlighters: make(map[string][]highlighter.Highlighter),
		aliases:      make(map[string]string),
	}
}

// SetAliases 设置语言别名映射表 (来自 config.json 的 aliases 字段)。
// 仅对 formatter/compressor 查找生效，高亮器由 chroma 原生处理各语言变体。
func (r *Registry) SetAliases(aliases map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.aliases = aliases
}

// resolveLang 返回语言的规范名称 (若为别名则解析为目标语言)。
func (r *Registry) resolveLang(lang string) string {
	if canonical, ok := r.aliases[lang]; ok {
		return canonical
	}
	return lang
}

func (r *Registry) RegisterFormatter(lang string, f formatter.Formatter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.formatters[lang] = append(r.formatters[lang], f)
}

func (r *Registry) RegisterCompressor(lang string, c compressor.Compressor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.compressors[lang] = append(r.compressors[lang], c)
}

func (r *Registry) RegisterHighlighter(lang string, h highlighter.Highlighter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.highlighters[lang] = append(r.highlighters[lang], h)
}

func (r *Registry) GetFormatter(lang, preferBackend string) formatter.Formatter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := r.formatters[lang]
	if len(list) == 0 {
		// 别名解析: bash/sh/zsh → shell, yml → yaml
		list = r.formatters[r.resolveLang(lang)]
	}
	if preferBackend != "" {
		for _, f := range list {
			if f.Type().String() == preferBackend {
				return f
			}
		}
	}
	if len(list) > 0 {
		return list[0]
	}
	return nil
}

func (r *Registry) GetCompressor(lang, preferBackend string) compressor.Compressor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := r.compressors[lang]
	if len(list) == 0 {
		list = r.compressors[r.resolveLang(lang)]
	}
	if preferBackend != "" {
		for _, c := range list {
			if c.Type().String() == preferBackend {
				return c
			}
		}
	}
	if len(list) > 0 {
		return list[0]
	}
	return nil
}

func (r *Registry) GetHighlighter(lang, preferBackend string) highlighter.Highlighter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := r.highlighters[lang]
	if preferBackend != "" {
		// 高亮器固定为 chroma 后端，仅按 Name 匹配 (如 "chroma")
		for _, h := range list {
			if h.Name() == preferBackend {
				return h
			}
		}
	}
	if len(list) > 0 {
		return list[0]
	}
	return nil
}

func (r *Registry) SupportedLanguages() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	langSet := make(map[string]struct{})
	for lang := range r.formatters {
		langSet[lang] = struct{}{}
	}
	for lang := range r.compressors {
		langSet[lang] = struct{}{}
	}
	for lang := range r.highlighters {
		langSet[lang] = struct{}{}
	}
	langs := make([]string, 0, len(langSet))
	for lang := range langSet {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	return langs
}
