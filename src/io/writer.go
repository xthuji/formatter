package iocore

import (
	"fmt"
	"os"

	"golang.design/x/clipboard"
)

type FileWriter struct {
	path string
}

func NewFileWriter(path string) *FileWriter {
	return &FileWriter{path: path}
}

func (w *FileWriter) Write(content []byte, highlighted bool) error {
	if err := os.WriteFile(w.path, content, 0644); err != nil {
		return fmt.Errorf("write file %s: %w", w.path, err)
	}
	return nil
}

func (w *FileWriter) Dest() string {
	return "file:" + w.path
}

type StdoutWriter struct{}

func NewStdoutWriter() *StdoutWriter {
	return &StdoutWriter{}
}

func (w *StdoutWriter) Write(content []byte, highlighted bool) error {
	_, err := os.Stdout.Write(content)
	return err
}

func (w *StdoutWriter) Dest() string {
	return "stdout"
}

type ClipboardWriter struct{}

func NewClipboardWriter() *ClipboardWriter {
	return &ClipboardWriter{}
}

func (w *ClipboardWriter) Write(content []byte, highlighted bool) error {
	clipboard.Write(clipboard.FmtText, content)
	return nil
}

func (w *ClipboardWriter) Dest() string {
	return "clipboard"
}
