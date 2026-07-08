//go:build windows

package pty

import (
	"io"
	"os"
	"syscall"

	"github.com/ActiveState/termtest/conpty"
)

type localSession struct {
	pty    *conpty.ConPty
	handle uintptr
}

func NewLocal(rows, cols uint16) (Session, error) {
	return NewLocalWithOptions(rows, cols, Options{})
}

func NewLocalWithOptions(rows, cols uint16, opts Options) (Session, error) {
	cp, err := conpty.New(int16(cols), int16(rows))
	if err != nil {
		return nil, err
	}

	shell := opts.ShellProgram
	if shell == "" {
		shell = os.Getenv("CERVTERM_SHELL")
	}
	if shell == "" {
		shell = os.Getenv("COMSPEC")
	}
	if shell == "" {
		shell = "powershell.exe"
	}
	args := opts.ShellArgs
	if len(args) == 0 {
		args = []string{shell}
	}

	_, handle, err := cp.Spawn(shell, args, nil)
	if err != nil {
		_ = cp.Close()
		return nil, err
	}

	return &localSession{pty: cp, handle: handle}, nil
}

func (s *localSession) Reader() io.Reader { return s.pty.OutPipe() }

func (s *localSession) Write(p []byte) (int, error) {
	n, err := s.pty.Write(p)
	return int(n), err
}

func (s *localSession) Resize(size Size) error {
	return s.pty.Resize(size.Cols, size.Rows)
}

func (s *localSession) Close() error {
	if s.handle != 0 {
		_ = syscall.TerminateProcess(syscall.Handle(s.handle), 1)
		_ = syscall.CloseHandle(syscall.Handle(s.handle))
		s.handle = 0
	}
	return s.pty.Close()
}
