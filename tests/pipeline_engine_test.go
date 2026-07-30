package tests

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/formatter/formatter/src/compressor"
	"github.com/formatter/formatter/src/config"
	"github.com/formatter/formatter/src/formatter"
	"github.com/formatter/formatter/src/highlighter"
	iocore "github.com/formatter/formatter/src/io"
	"github.com/formatter/formatter/src/pipeline"
	"github.com/formatter/formatter/src/registry"

	natcomp "github.com/formatter/formatter/src/compressor/native"
	natfmt "github.com/formatter/formatter/src/formatter/native"
)

type mockReader struct {
	data   []byte
	lang   string
	err    error
	source string
}

func (r *mockReader) Read() ([]byte, string, error) {
	return r.data, r.lang, r.err
}

func (r *mockReader) Source() string {
	return r.source
}

var _ iocore.InputReader = (*mockReader)(nil)

type mockWriter struct {
	buf         *bytes.Buffer
	highlighted bool
	err         error
	dest        string
}

func (w *mockWriter) Write(content []byte, highlighted bool) error {
	if w.err != nil {
		return w.err
	}
	w.buf.Write(content)
	w.highlighted = highlighted
	return nil
}

func (w *mockWriter) Dest() string {
	return w.dest
}

var _ iocore.OutputWriter = (*mockWriter)(nil)

func setupTestRegistry() *registry.Registry {
	reg := registry.New()

	reg.RegisterFormatter("json", natfmt.NewJSONFormatter())
	reg.RegisterFormatter("yaml", natfmt.NewYAMLFormatter())
	reg.RegisterFormatter("xml", natfmt.NewXMLFormatter())
	reg.RegisterFormatter("html", natfmt.NewHTMLFormatter())

	reg.RegisterCompressor("json", natcomp.NewJSONCompressor())
	reg.RegisterCompressor("xml", natcomp.NewXMLCompressor())

	hl := highlighter.NewChromaHighlighter(nil)
	// 为测试中使用的语言注册高亮器
	for _, lang := range []string{"json", "yaml", "xml", "html", "go", "javascript", "css", "python", "shell", "sql"} {
		reg.RegisterHighlighter(lang, hl)
	}

	return reg
}

func TestEngine_FormatOnly(t *testing.T) {
	t.Parallel()
	reg := setupTestRegistry()
	cfg := config.Default()
	engine := pipeline.New(reg, cfg)

	reader := &mockReader{
		data:   []byte(`{"name":"test","age":30}`),
		source: "test",
	}
	writer := &mockWriter{buf: &bytes.Buffer{}, dest: "test"}

	err := engine.Execute(context.Background(), pipeline.ExecuteParams{
		Reader:       reader,
		Writer:       writer,
		Language:     "json",
		EnableFormat: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := writer.buf.String()
	if !strings.Contains(result, "name") {
		t.Errorf("expected formatted JSON containing 'name', got: %s", result)
	}
	if !strings.Contains(result, "\n") {
		t.Errorf("expected formatted output with newlines, got: %s", result)
	}
}

func TestEngine_CompressOnly(t *testing.T) {
	t.Parallel()
	reg := setupTestRegistry()
	cfg := config.Default()
	engine := pipeline.New(reg, cfg)

	reader := &mockReader{
		data:   []byte("{\n    \"name\": \"test\",\n    \"age\": 30\n}\n"),
		source: "test",
	}
	writer := &mockWriter{buf: &bytes.Buffer{}, dest: "test"}

	err := engine.Execute(context.Background(), pipeline.ExecuteParams{
		Reader:         reader,
		Writer:         writer,
		Language:       "json",
		EnableCompress: true,
		CompressOpts:   compressor.CompressOptions{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := writer.buf.String()
	if strings.Contains(result, "\n") {
		t.Errorf("expected compressed single-line output, got: %s", result)
	}
	if !strings.Contains(result, "{") {
		t.Errorf("expected valid JSON, got: %s", result)
	}
}

func TestEngine_FormatThenCompress(t *testing.T) {
	reg := setupTestRegistry()
	cfg := config.Default()
	engine := pipeline.New(reg, cfg)

	reader := &mockReader{
		data:   []byte(`{"name":"test","age":30}`),
		source: "test",
	}
	writer := &mockWriter{buf: &bytes.Buffer{}, dest: "test"}

	err := engine.Execute(context.Background(), pipeline.ExecuteParams{
		Reader:         reader,
		Writer:         writer,
		Language:       "json",
		EnableFormat:   true,
		EnableCompress: true,
		CompressOpts:   compressor.CompressOptions{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := writer.buf.String()
	if strings.Contains(result, "\n") {
		t.Errorf("expected final compressed output with no newlines, got: %s", result)
	}
	if !strings.Contains(result, "\"name\":\"test\"") {
		t.Errorf("expected compressed JSON content, got: %s", result)
	}
}

func TestEngine_FormatThenHighlight(t *testing.T) {
	reg := setupTestRegistry()
	cfg := config.Default()
	engine := pipeline.New(reg, cfg)

	reader := &mockReader{
		data:   []byte(`{"name":"test","value":42}`),
		source: "test",
	}
	writer := &mockWriter{buf: &bytes.Buffer{}, dest: "test"}

	err := engine.Execute(context.Background(), pipeline.ExecuteParams{
		Reader:          reader,
		Writer:          writer,
		Language:        "json",
		EnableFormat:    true,
		EnableHighlight: true,
		HighlightOpts:   highlighter.HighlightOptions{Style: "monokai", OutputType: "html"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := writer.buf.String()
	if len(result) == 0 {
		t.Error("expected non-empty highlighted output")
	}
	if !strings.Contains(result, "<") {
		t.Errorf("expected HTML output from highlighter, got: %s", result[:min(len(result), 100)])
	}
	if !writer.highlighted {
		t.Error("expected highlighted flag to be true")
	}
}

func TestEngine_CompressUnsupportedLanguage(t *testing.T) {
	reg := setupTestRegistry()
	cfg := config.Default()
	engine := pipeline.New(reg, cfg)

	reader := &mockReader{
		data:   []byte("test content"),
		source: "test",
	}
	writer := &mockWriter{buf: &bytes.Buffer{}, dest: "test"}

	err := engine.Execute(context.Background(), pipeline.ExecuteParams{
		Reader:         reader,
		Writer:         writer,
		Language:       "go",
		EnableCompress: true,
	})
	if err == nil {
		t.Fatal("expected error for unsupported compressor")
	}
	if !strings.Contains(err.Error(), "no compressor") {
		t.Errorf("expected 'no compressor' error, got: %v", err)
	}
}

func TestEngine_CompressSkipUnsupported(t *testing.T) {
	reg := setupTestRegistry()
	cfg := config.Default()
	engine := pipeline.New(reg, cfg)

	reader := &mockReader{
		data:   []byte("test content"),
		source: "test",
	}
	writer := &mockWriter{buf: &bytes.Buffer{}, dest: "test"}

	err := engine.Execute(context.Background(), pipeline.ExecuteParams{
		Reader:                  reader,
		Writer:                  writer,
		Language:                "go",
		EnableCompress:          true,
		CompressSkipUnsupported: true,
	})
	if err != nil {
		t.Fatalf("unexpected error with skip unsupported: %v", err)
	}
	if writer.buf.Len() == 0 {
		t.Error("expected output to be written even when compressor is skipped")
	}
}

func TestEngine_NoFormatter(t *testing.T) {
	reg := setupTestRegistry()
	cfg := config.Default()
	engine := pipeline.New(reg, cfg)

	reader := &mockReader{
		data:   []byte("content"),
		source: "test",
	}
	writer := &mockWriter{buf: &bytes.Buffer{}, dest: "test"}

	err := engine.Execute(context.Background(), pipeline.ExecuteParams{
		Reader:       reader,
		Writer:       writer,
		Language:     "unknown_lang",
		EnableFormat: true,
	})
	if err == nil {
		t.Fatal("expected error for unknown language")
	}
	if !strings.Contains(err.Error(), "no formatter") {
		t.Errorf("expected 'no formatter' error, got: %v", err)
	}
}

func TestEngine_ReaderError(t *testing.T) {
	reg := setupTestRegistry()
	cfg := config.Default()
	engine := pipeline.New(reg, cfg)

	reader := &mockReader{
		err:    context.DeadlineExceeded,
		source: "test",
	}
	writer := &mockWriter{buf: &bytes.Buffer{}, dest: "test"}

	err := engine.Execute(context.Background(), pipeline.ExecuteParams{
		Reader:       reader,
		Writer:       writer,
		Language:     "json",
		EnableFormat: true,
	})
	if err == nil {
		t.Fatal("expected error from reader")
	}
}

func TestEngine_AutoDetectLanguage(t *testing.T) {
	reg := setupTestRegistry()
	cfg := config.Default()
	engine := pipeline.New(reg, cfg)

	reader := &mockReader{
		data:   []byte(`{"name":"test"}`),
		lang:   "json",
		source: "test.json",
	}
	writer := &mockWriter{buf: &bytes.Buffer{}, dest: "test"}

	err := engine.Execute(context.Background(), pipeline.ExecuteParams{
		Reader:       reader,
		Writer:       writer,
		Language:     "auto",
		EnableFormat: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := writer.buf.String()
	if !strings.Contains(result, "name") {
		t.Errorf("expected formatted JSON, got: %s", result)
	}
}

func TestEngine_FormatJSON(t *testing.T) {
	reg := setupTestRegistry()
	cfg := config.Default()
	engine := pipeline.New(reg, cfg)

	reader := &mockReader{
		data:   []byte(`{"name":"test","age":30}`),
		source: "test.json",
	}
	writer := &mockWriter{buf: &bytes.Buffer{}, dest: "test"}

	err := engine.Execute(context.Background(), pipeline.ExecuteParams{
		Reader:       reader,
		Writer:       writer,
		Language:     "json",
		EnableFormat: true,
		FormatOpts:   formatter.FormatOptions{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := writer.buf.String()
	if !strings.Contains(result, "name") {
		t.Errorf("expected formatted JSON code, got: %s", result)
	}
}

func TestEngine_DefaultLanguage(t *testing.T) {
	reg := setupTestRegistry()
	cfg := config.Default()
	engine := pipeline.New(reg, cfg)

	reader := &mockReader{
		data:   []byte("hello world"),
		lang:   "",
		source: "test",
	}
	writer := &mockWriter{buf: &bytes.Buffer{}, dest: "test"}

	err := engine.Execute(context.Background(), pipeline.ExecuteParams{
		Reader:       reader,
		Writer:       writer,
		Language:     "auto",
		EnableFormat: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writer.buf.Len() == 0 {
		t.Error("expected output to be written")
	}
}

// ---- Registry 注册表 ----

type mockFormatter struct {
	name  string
	lang  string
	bt    formatter.BackendType
	langs []string
}

func (m *mockFormatter) Format(input []byte, lang string, opts formatter.FormatOptions) ([]byte, error) {
	return input, nil
}

func (m *mockFormatter) SupportedLanguages() []string { return m.langs }
func (m *mockFormatter) Name() string                 { return m.name }
func (m *mockFormatter) Type() formatter.BackendType  { return m.bt }

var _ formatter.Formatter = (*mockFormatter)(nil)

type mockCompressor struct {
	name  string
	bt    compressor.BackendType
	langs []string
}

func (m *mockCompressor) Compress(input []byte, lang string, opts compressor.CompressOptions) ([]byte, error) {
	return input, nil
}

func (m *mockCompressor) SupportedLanguages() []string { return m.langs }
func (m *mockCompressor) Name() string                 { return m.name }
func (m *mockCompressor) Type() compressor.BackendType { return m.bt }

var _ compressor.Compressor = (*mockCompressor)(nil)

type mockHighlighter struct {
	name  string
	langs []string
}

func (m *mockHighlighter) Highlight(input []byte, lang string, opts highlighter.HighlightOptions) (string, error) {
	return string(input), nil
}

func (m *mockHighlighter) SupportedLanguages() []string { return m.langs }
func (m *mockHighlighter) Name() string                 { return m.name }

var _ highlighter.Highlighter = (*mockHighlighter)(nil)

func TestRegistry_RegisterAndGetFormatter(t *testing.T) {
	reg := registry.New()
	f := &mockFormatter{name: "test-fmt", bt: formatter.NativeGo, langs: []string{"go"}}

	reg.RegisterFormatter("go", f)

	got := reg.GetFormatter("go", "")
	if got == nil {
		t.Fatal("expected formatter, got nil")
	}
	if got.Name() != "test-fmt" {
		t.Errorf("expected name 'test-fmt', got %q", got.Name())
	}
}

func TestRegistry_GetFormatter_NotFound(t *testing.T) {
	reg := registry.New()

	got := reg.GetFormatter("nonexistent", "")
	if got != nil {
		t.Errorf("expected nil for unregistered language, got %v", got)
	}
}

func TestRegistry_GetFormatter_WithBackendPreference(t *testing.T) {
	reg := registry.New()

	f1 := &mockFormatter{name: "native-fmt", bt: formatter.NativeGo, langs: []string{"json"}}
	f2 := &mockFormatter{name: "tool-fmt", bt: formatter.ToolBinary, langs: []string{"json"}}

	reg.RegisterFormatter("json", f1)
	reg.RegisterFormatter("json", f2)

	got := reg.GetFormatter("json", "tool")
	if got == nil {
		t.Fatal("expected formatter with backend preference, got nil")
	}
	if got.Name() != "tool-fmt" {
		t.Errorf("expected 'tool-fmt', got %q", got.Name())
	}
}

func TestRegistry_GetFormatter_FallbackWhenNoPreferred(t *testing.T) {
	reg := registry.New()

	f1 := &mockFormatter{name: "native-fmt", bt: formatter.NativeGo, langs: []string{"json"}}
	f2 := &mockFormatter{name: "tool-fmt", bt: formatter.ToolBinary, langs: []string{"json"}}

	reg.RegisterFormatter("json", f1)
	reg.RegisterFormatter("json", f2)

	got := reg.GetFormatter("json", "rust")
	if got == nil {
		t.Fatal("expected fallback formatter, got nil")
	}
	if got.Name() != "native-fmt" {
		t.Errorf("expected 'native-fmt' as fallback, got %q", got.Name())
	}
}

func TestRegistry_RegisterAndGetCompressor(t *testing.T) {
	reg := registry.New()
	c := &mockCompressor{name: "test-comp", bt: compressor.NativeGo, langs: []string{"json"}}

	reg.RegisterCompressor("json", c)

	got := reg.GetCompressor("json", "")
	if got == nil {
		t.Fatal("expected compressor, got nil")
	}
	if got.Name() != "test-comp" {
		t.Errorf("expected name 'test-comp', got %q", got.Name())
	}
}

func TestRegistry_GetCompressor_NotFound(t *testing.T) {
	reg := registry.New()
	got := reg.GetCompressor("nonexistent", "")
	if got != nil {
		t.Errorf("expected nil for unregistered language, got %v", got)
	}
}

func TestRegistry_GetCompressor_WithBackendPreference(t *testing.T) {
	reg := registry.New()

	c1 := &mockCompressor{name: "native-comp", bt: compressor.NativeGo, langs: []string{"xml"}}
	c2 := &mockCompressor{name: "tool-comp", bt: compressor.ToolBinary, langs: []string{"xml"}}

	reg.RegisterCompressor("xml", c1)
	reg.RegisterCompressor("xml", c2)

	got := reg.GetCompressor("xml", "tool")
	if got == nil {
		t.Fatal("expected compressor with backend preference, got nil")
	}
	if got.Name() != "tool-comp" {
		t.Errorf("expected 'tool-comp', got %q", got.Name())
	}
}

func TestRegistry_RegisterAndGetHighlighter(t *testing.T) {
	reg := registry.New()
	h := &mockHighlighter{name: "test-hl", langs: []string{"go"}}

	reg.RegisterHighlighter("go", h)

	got := reg.GetHighlighter("go", "")
	if got == nil {
		t.Fatal("expected highlighter, got nil")
	}
	if got.Name() != "test-hl" {
		t.Errorf("expected name 'test-hl', got %q", got.Name())
	}
}

func TestRegistry_GetHighlighter_NotFound(t *testing.T) {
	reg := registry.New()
	got := reg.GetHighlighter("nonexistent", "")
	if got != nil {
		t.Errorf("expected nil for unregistered language, got %v", got)
	}
}

func TestRegistry_GetHighlighter_WithBackendPreference(t *testing.T) {
	reg := registry.New()

	h1 := &mockHighlighter{name: "chroma", langs: []string{"go"}}
	h2 := &mockHighlighter{name: "pygments", langs: []string{"go"}}

	reg.RegisterHighlighter("go", h1)
	reg.RegisterHighlighter("go", h2)

	got := reg.GetHighlighter("go", "pygments")
	if got == nil {
		t.Fatal("expected highlighter with backend preference, got nil")
	}
	if got.Name() != "pygments" {
		t.Errorf("expected 'pygments', got %q", got.Name())
	}
}

func TestRegistry_SupportedLanguages(t *testing.T) {
	reg := registry.New()

	reg.RegisterFormatter("go", &mockFormatter{name: "f1", langs: []string{"go"}})
	reg.RegisterFormatter("json", &mockFormatter{name: "f2", langs: []string{"json"}})
	reg.RegisterCompressor("json", &mockCompressor{name: "c1", langs: []string{"json"}})
	reg.RegisterCompressor("xml", &mockCompressor{name: "c2", langs: []string{"xml"}})
	reg.RegisterHighlighter("go", &mockHighlighter{name: "h1", langs: []string{"go"}})
	reg.RegisterHighlighter("python", &mockHighlighter{name: "h2", langs: []string{"python"}})

	langs := reg.SupportedLanguages()
	if len(langs) != 4 {
		t.Errorf("expected 4 unique languages, got %d: %v", len(langs), langs)
	}

	expected := map[string]bool{"go": true, "json": true, "xml": true, "python": true}
	for _, lang := range langs {
		if !expected[lang] {
			t.Errorf("unexpected language: %s", lang)
		}
	}
	for lang := range expected {
		found := false
		for _, l := range langs {
			if l == lang {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected language: %s", lang)
		}
	}
}

func TestRegistry_SupportedLanguages_Empty(t *testing.T) {
	reg := registry.New()
	langs := reg.SupportedLanguages()
	if len(langs) != 0 {
		t.Errorf("expected empty supported languages, got %v", langs)
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	reg := registry.New()
	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			reg.RegisterFormatter("go", &mockFormatter{name: "fmt", bt: formatter.NativeGo, langs: []string{"go"}})
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			reg.GetFormatter("go", "")
			reg.SupportedLanguages()
		}
		done <- true
	}()

	<-done
	<-done
}

func TestBackendType_String(t *testing.T) {
	tests := []struct {
		bt   formatter.BackendType
		want string
	}{
		{formatter.NativeGo, "native"},
		{formatter.ToolBinary, "tool"},
		{formatter.BackendType(99), "unknown"},
	}
	for _, tt := range tests {
		got := tt.bt.String()
		if got != tt.want {
			t.Errorf("BackendType(%d).String() = %q, want %q", tt.bt, got, tt.want)
		}
	}
}

func TestCompressorBackendType_String(t *testing.T) {
	tests := []struct {
		bt   compressor.BackendType
		want string
	}{
		{compressor.NativeGo, "native"},
		{compressor.ToolBinary, "tool"},
		{compressor.BackendType(99), "unknown"},
	}
	for _, tt := range tests {
		got := tt.bt.String()
		if got != tt.want {
			t.Errorf("Compressor BackendType(%d).String() = %q, want %q", tt.bt, got, tt.want)
		}
	}
}
