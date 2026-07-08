//go:build !windows

package pty

import (
	"io"
	"os"
	"os/exec"

	creackpty "github.com/creack/pty"
)

type localSession struct {
	cmd  *exec.Cmd
	file *os.File
}

func NewLocal(rows, cols uint16) (Session, error) {
	return NewLocalWithOptions(rows, cols, Options{})
}

func NewLocalWithOptions(rows, cols uint16, opts Options) (Session, error) {
	shell := opts.ShellProgram
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell, opts.ShellArgs...)
	cmd.Dir = opts.WorkingDirectory
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	for key, value := range opts.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	f, err := creackpty.StartWithSize(cmd, &creackpty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		return nil, err
	}
	return &localSession{cmd: cmd, file: f}, nil
}

func (s *localSession) Reader() io.Reader           { return s.file }
func (s *localSession) Write(p []byte) (int, error) { return s.file.Write(p) }
func (s *localSession) Resize(size Size) error {
	return creackpty.Setsize(s.file, &creackpty.Winsize{Rows: size.Rows, Cols: size.Cols})
}
func (s *localSession) Close() error {
	_ = s.cmd.Process.Kill()
	return s.file.Close()
}
