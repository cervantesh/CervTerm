package pty

import "io"

type Size struct {
	Rows, Cols uint16
}

type Options struct {
	ShellProgram     string
	ShellArgs        []string
	WorkingDirectory string
	Env              map[string]string
}

type Session interface {
	Reader() io.Reader
	Write([]byte) (int, error)
	Resize(Size) error
	Close() error
}
