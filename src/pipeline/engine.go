package pipeline

import (
	"context"
	"fmt"

	"github.com/formatter/formatter/src/compressor"
	"github.com/formatter/formatter/src/config"
	"github.com/formatter/formatter/src/formatter"
	"github.com/formatter/formatter/src/highlighter"
	iocore "github.com/formatter/formatter/src/io"
	"github.com/formatter/formatter/src/registry"
)

type Engine struct {
	registry *registry.Registry
	cfg      *config.Config
}

func New(reg *registry.Registry, cfg *config.Config) *Engine {
	return &Engine{registry: reg, cfg: cfg}
}

type ExecuteParams struct {
	Reader                  iocore.InputReader
	Writer                  iocore.OutputWriter
	Language                string
	EnableFormat            bool
	EnableCompress          bool
	EnableHighlight         bool
	CompressSkipUnsupported bool
	FormatterBackend        string
	CompressorBackend       string
	HighlighterBackend      string
	FormatOpts              formatter.FormatOptions
	CompressOpts            compressor.CompressOptions
	HighlightOpts           highlighter.HighlightOptions
}

func (e *Engine) Execute(ctx context.Context, params ExecuteParams) error {
	input, detectedLang, err := params.Reader.Read()
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	lang := params.Language
	if lang == "auto" || lang == "" {
		lang = detectedLang
	}
	if lang == "" {
		lang = "text"
	}

	if params.EnableFormat {
		f := e.registry.GetFormatter(lang, params.FormatterBackend)
		if f == nil {
			return fmt.Errorf("no formatter for language: %s", lang)
		}
		input, err = f.Format(input, lang, params.FormatOpts)
		if err != nil {
			return fmt.Errorf("format failed: %w", err)
		}
	}

	if params.EnableCompress {
		c := e.registry.GetCompressor(lang, params.CompressorBackend)
		if c == nil {
			if !params.CompressSkipUnsupported {
				return fmt.Errorf("no compressor for language: %s", lang)
			}
		} else {
			input, err = c.Compress(input, lang, params.CompressOpts)
			if err != nil {
				return fmt.Errorf("compress failed: %w", err)
			}
		}
	}

	var highlightedContent string
	if params.EnableHighlight {
		h := e.registry.GetHighlighter(lang, params.HighlighterBackend)
		if h == nil {
			return fmt.Errorf("no highlighter for language: %s", lang)
		}
		highlightedContent, err = h.Highlight(input, lang, params.HighlightOpts)
		if err != nil {
			return fmt.Errorf("highlight failed: %w", err)
		}
	}

	output := input
	if params.EnableHighlight {
		output = []byte(highlightedContent)
	}
	return params.Writer.Write(output, params.EnableHighlight)
}
