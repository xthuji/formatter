package appcommon

import (
	"fmt"

	iocore "github.com/formatter/formatter/src/io"
)

// BufferReader 实现 InputReader 接口，从内存字节切片读取输入。
// 供 Wails App 和 Web UI Server 共用。
type BufferReader struct {
	data  []byte
	index int
}

// NewBufferReader 创建从字节切片读取的 Reader。
func NewBufferReader(data []byte) *BufferReader {
	return &BufferReader{data: data}
}

func (r *BufferReader) Read() ([]byte, string, error) {
	if r.index >= len(r.data) {
		return nil, "", fmt.Errorf("EOF")
	}
	data := r.data[r.index:]
	r.index = len(r.data)
	// 从内容检测语言 (shebang + 内容模式)，无文件路径时跳过扩展名检测
	lang := iocore.DetectLanguage("", data)
	return data, lang, nil
}

func (r *BufferReader) Source() string { return "buffer" }

// BufferWriter 实现OutputWriter 接口，将输出写入内存缓冲区。
// 供 Wails App 和 Web UI Server 共用。
type BufferWriter struct {
	buf []byte
}

// NewBufferWriter 创建写入内存缓冲区的 Writer。
func NewBufferWriter() *BufferWriter {
	return &BufferWriter{}
}

func (w *BufferWriter) Write(content []byte, highlighted bool) error {
	w.buf = append(w.buf, content...)
	return nil
}

func (w *BufferWriter) Dest() string { return "buffer" }

func (w *BufferWriter) String() string { return string(w.buf) }
