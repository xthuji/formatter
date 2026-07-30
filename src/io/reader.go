package iocore

import (
	"fmt"
	"os"

	"golang.design/x/clipboard"
)

type FileReader struct {
	path string
}

func NewFileReader(path string) *FileReader {
	return &FileReader{path: path}
}

func (r *FileReader) Read() ([]byte, string, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return nil, "", fmt.Errorf("read file %s: %w", r.path, err)
	}
	lang := DetectLanguage(r.path, data)
	return data, lang, nil
}

func (r *FileReader) Source() string {
	return "file:" + r.path
}

type StdinReader struct{}

func NewStdinReader() *StdinReader {
	return &StdinReader{}
}

func (r *StdinReader) Read() ([]byte, string, error) {
	data, err := os.ReadFile("/dev/stdin")
	if err != nil {
		return nil, "", fmt.Errorf("read stdin: %w", err)
	}
	return data, "", nil
}

func (r *StdinReader) Source() string {
	return "stdin"
}

type ClipboardReader struct{}

func NewClipboardReader() *ClipboardReader {
	return &ClipboardReader{}
}

func (r *ClipboardReader) Read() ([]byte, string, error) {
	data := clipboard.Read(clipboard.FmtText)
	if len(data) == 0 {
		return nil, "", fmt.Errorf("clipboard is empty")
	}
	return data, "", nil
}

func (r *ClipboardReader) Source() string {
	return "clipboard"
}
