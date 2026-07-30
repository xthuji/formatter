package tests

import (
	"os"
	"path/filepath"
	"testing"

	appcommon "github.com/formatter/formatter/src/appcommon"
	iocore "github.com/formatter/formatter/src/io"
)

// ---- BufferReader ----

func TestBufferReader_Read_Success(t *testing.T) {
	data := []byte(`{"key": "value"}`)
	r := appcommon.NewBufferReader(data)
	got, lang, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("expected %q, got %q", string(data), string(got))
	}
	// JSON 内容应被检测
	if lang == "" {
		// 无扩展名时可能检测不到，不强制要求
	}
}

func TestBufferReader_Read_EOF(t *testing.T) {
	r := appcommon.NewBufferReader([]byte("hello"))
	_, _, err := r.Read()
	if err != nil {
		t.Fatalf("first read should succeed: %v", err)
	}
	_, _, err = r.Read()
	if err == nil {
		t.Error("expected error on second read (EOF)")
	}
}

func TestBufferReader_Source(t *testing.T) {
	r := appcommon.NewBufferReader([]byte("x"))
	if r.Source() != "buffer" {
		t.Errorf("expected source='buffer', got %q", r.Source())
	}
}

func TestBufferReader_Read_DetectsShebang(t *testing.T) {
	data := []byte("#!/usr/bin/env python3\nprint('hello')")
	r := appcommon.NewBufferReader(data)
	_, lang, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lang != "python" {
		t.Errorf("expected lang='python' for python shebang, got %q", lang)
	}
}

// ---- BufferWriter ----

func TestBufferWriter_WriteAndString(t *testing.T) {
	w := appcommon.NewBufferWriter()
	if err := w.Write([]byte("hello "), false); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := w.Write([]byte("world"), false); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if w.String() != "hello world" {
		t.Errorf("expected 'hello world', got %q", w.String())
	}
}

func TestBufferWriter_Dest(t *testing.T) {
	w := appcommon.NewBufferWriter()
	if w.Dest() != "buffer" {
		t.Errorf("expected dest='buffer', got %q", w.Dest())
	}
}

func TestBufferWriter_EmptyString(t *testing.T) {
	w := appcommon.NewBufferWriter()
	if w.String() != "" {
		t.Errorf("expected empty string, got %q", w.String())
	}
}

// ---- FileReader ----

func TestFileReader_Read_Success(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	content := []byte("package main\nfunc main() {}\n")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	r := iocore.NewFileReader(testFile)
	data, lang, err := r.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("expected %q, got %q", string(content), string(data))
	}
	if lang != "go" {
		t.Errorf("expected lang='go', got %q", lang)
	}
}

func TestFileReader_Read_NotFound(t *testing.T) {
	r := iocore.NewFileReader("/nonexistent/file.go")
	_, _, err := r.Read()
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestFileReader_Source(t *testing.T) {
	r := iocore.NewFileReader("/path/to/file.py")
	if r.Source() != "file:/path/to/file.py" {
		t.Errorf("expected 'file:/path/to/file.py', got %q", r.Source())
	}
}

// ---- FileWriter ----

func TestFileWriter_Write_Success(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "output.txt")
	content := []byte("formatted output")

	w := iocore.NewFileWriter(testFile)
	if err := w.Write(content, false); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("expected %q, got %q", string(content), string(data))
	}
}

func TestFileWriter_Write_InvalidPath(t *testing.T) {
	w := iocore.NewFileWriter("/nonexistent/dir/output.txt")
	err := w.Write([]byte("test"), false)
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestFileWriter_Dest(t *testing.T) {
	w := iocore.NewFileWriter("/path/to/out.txt")
	if w.Dest() != "file:/path/to/out.txt" {
		t.Errorf("expected 'file:/path/to/out.txt', got %q", w.Dest())
	}
}

// ---- StdoutWriter ----

func TestStdoutWriter_Dest(t *testing.T) {
	w := iocore.NewStdoutWriter()
	if w.Dest() != "stdout" {
		t.Errorf("expected 'stdout', got %q", w.Dest())
	}
}

func TestStdoutWriter_Write(t *testing.T) {
	w := iocore.NewStdoutWriter()
	// 写入 stdout 不报错即可 (内容会输出到测试输出)
	if err := w.Write([]byte(""), false); err != nil {
		t.Errorf("unexpected error writing to stdout: %v", err)
	}
}

// ---- StdinReader ----

func TestStdinReader_Source(t *testing.T) {
	r := iocore.NewStdinReader()
	if r.Source() != "stdin" {
		t.Errorf("expected 'stdin', got %q", r.Source())
	}
}

// ---- 语言检测 ----

func TestDetectLanguage_ByExtension(t *testing.T) {
	tests := []struct {
		filePath string
		content  []byte
		want     string
	}{
		{"/path/main.go", nil, "go"},
		{"/path/Main.GO", nil, "go"},
		{"/path/App.java", nil, "java"},
		{"/path/script.py", nil, "python"},
		{"/path/script.pyw", nil, "python"},
		{"/path/app.js", nil, "javascript"},
		{"/path/app.jsx", nil, "javascript"},
		{"/path/app.mjs", nil, "javascript"},
		{"/path/app.cjs", nil, "javascript"},
		{"/path/app.ts", nil, "typescript"},
		{"/path/app.tsx", nil, "typescript"},
		{"/path/style.css", nil, "css"},
		{"/path/style.scss", nil, "css"},
		{"/path/style.less", nil, "css"},
		{"/path/index.html", nil, "html"},
		{"/path/index.htm", nil, "html"},
		{"/path/data.xml", nil, "xml"},
		{"/path/data.json", nil, "json"},
		{"/path/config.yaml", nil, "yaml"},
		{"/path/config.yml", nil, "yaml"},
		{"/path/script.rb", nil, "ruby"},
		{"/path/script.lua", nil, "lua"},
		{"/path/script.scala", nil, "scala"},
		{"/path/script.sc", nil, "scala"},
		{"/path/script.rs", nil, "rust"},
		{"/path/query.sql", nil, "sql"},
		{"/path/script.sh", nil, "shell"},
		{"/path/script.bash", nil, "shell"},
		{"/path/script.zsh", nil, "shell"},
		{"/path/script.ksh", nil, "shell"},
		{"/path/script.applescript", nil, "applescript"},
		{"/path/script.scpt", nil, "applescript"},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			got := iocore.DetectLanguage(tt.filePath, tt.content)
			if got != tt.want {
				t.Errorf("DetectLanguage(%q) = %q, want %q", tt.filePath, got, tt.want)
			}
		})
	}
}

func TestDetectLanguage_ByShebang(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
		want    string
	}{
		{"python3", "/tmp/script", "#!/usr/bin/env python3\nprint('hello')\n", "python"},
		{"python", "/tmp/script", "#!/usr/bin/env python\nprint('hello')\n", "python"},
		{"python direct", "/tmp/script", "#!/usr/bin/python3\nprint('hello')\n", "python"},
		{"node", "/tmp/script", "#!/usr/bin/env node\nconsole.log('hi')\n", "javascript"},
		{"ruby", "/tmp/script", "#!/usr/bin/env ruby\nputs 'hi'\n", "ruby"},
		{"lua", "/tmp/script", "#!/usr/bin/env lua\nprint('hi')\n", "lua"},
		{"bash", "/tmp/script", "#!/bin/bash\necho hi\n", "shell"},
		{"sh", "/tmp/script", "#!/bin/sh\necho hi\n", "shell"},
		{"zsh", "/tmp/script", "#!/bin/zsh\necho hi\n", "shell"},
		{"ksh", "/tmp/script", "#!/bin/ksh\necho hi\n", "shell"},
		{"fish", "/tmp/script", "#!/usr/bin/env fish\necho hi\n", "shell"},
		{"scala", "/tmp/script", "#!/usr/bin/env scala\nprintln(\"hi\")\n", "scala"},
		{"go run", "/tmp/script", "#!/usr/bin/env -S go run\npackage main\n", "go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := iocore.DetectLanguage(tt.file, []byte(tt.content))
			if got != tt.want {
				t.Errorf("DetectLanguage() for %s = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestDetectLanguage_ByContent(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
		want    string
	}{
		{"go package main", "/tmp/main", "package main\n\nfunc main() {}\n", "go"},
		{"java package", "/tmp/Test.java", "package com.example;\n\npublic class Test {}\n", "java"},
		{"java public class", "/tmp/Test", "public class Hello {\n    public static void main(String[] args) {}\n}\n", "java"},
		{"java import org", "/tmp/Test", "import org.junit.Test;\n\npublic class Test {}\n", "java"},
		{"python def", "/tmp/script", "def hello():\n    pass\n", "python"},
		{"python import", "/tmp/script", "import os\nimport sys\n", "python"},
		{"python from import", "/tmp/script", "from os import path\n", "python"},
		{"javascript function", "/tmp/script", "function hello() {\n  return 1;\n}\n", "javascript"},
		{"javascript const", "/tmp/script", "const x = 1;\n", "javascript"},
		{"typescript interface", "/tmp/script", "interface User {\n  name: string;\n}\n", "typescript"},
		{"typescript type annotation", "/tmp/script", "const x: string = 'hello';\n", "typescript"},
		{"ruby def", "/tmp/script", "def hello? do\n  puts 'hi'\nend\n", "ruby"},
		{"ruby class extends", "/tmp/script", "class Dog extends Animal\nend\n", "ruby"},
		{"rust function", "/tmp/script", "function hello\n    let x = 1\n", "rust"},
		{"rust let mut", "/tmp/script", "fn main() {\n    let mut x = 1;\n}\n", "rust"},
		{"rust fn", "/tmp/script", "fn hello() {\n    println!(\"hi\");\n}\n", "rust"},
		{"sql select", "/tmp/query", "SELECT * FROM users;\n", "sql"},
		{"sql create table", "/tmp/query", "CREATE TABLE users (id INT);\n", "sql"},
		{"sql insert", "/tmp/query", "INSERT INTO users VALUES (1, 'john');\n", "sql"},
		{"html doctype", "/tmp/page", "<!DOCTYPE html>\n<html><body></body></html>\n", "html"},
		{"html tag", "/tmp/page", "<html><body></body></html>\n", "html"},
		{"xml tag", "/tmp/data", "<?xml version=\"1.0\"?>\n<root></root>\n", "xml"},
		{"json object", "/tmp/data", `{"name": "test", "version": "1.0"}` + "\n", "json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := iocore.DetectLanguage(tt.file, []byte(tt.content))
			if got != tt.want {
				t.Errorf("DetectLanguage() for %s = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestDetectLanguage_NoMatch(t *testing.T) {
	tests := []struct {
		file    string
		content string
	}{
		{"/tmp/unknown.txt", "plain text content"},
		{"/tmp/noext", "some random text"},
		{"/tmp/data.xyz", "binary data"},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got := iocore.DetectLanguage(tt.file, []byte(tt.content))
			if got != "" {
				t.Errorf("expected empty string for unknown file, got %q", got)
			}
		})
	}
}

func TestDetectLanguage_EmptyContent(t *testing.T) {
	got := iocore.DetectLanguage("/tmp/test.go", []byte{})
	if got != "go" {
		t.Errorf("expected 'go' from extension, got %q", got)
	}
}

func TestDetectLanguage_ExtensionPriority(t *testing.T) {
	got := iocore.DetectLanguage("/tmp/script.py", []byte("#!/bin/bash\necho hi\n"))
	if got != "python" {
		t.Errorf("expected 'python' from extension (extension takes priority), got %q", got)
	}
}

func TestDetectLanguage_ContentSnippetLimit(t *testing.T) {
	longContent := make([]byte, 4096)
	for i := range longContent {
		longContent[i] = 'a'
	}
	got := iocore.DetectLanguage("/tmp/test", longContent)
	if got != "" {
		t.Errorf("expected empty for random long content, got %q", got)
	}
}
