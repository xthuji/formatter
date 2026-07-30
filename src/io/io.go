// Package iocore 定义输入输出抽象接口
package iocore

// InputReader 输入读取接口
type InputReader interface {
	Read() ([]byte, string, error)
	Source() string
}

// OutputWriter 输出写入接口
type OutputWriter interface {
	Write(content []byte, highlighted bool) error
	Dest() string
}
