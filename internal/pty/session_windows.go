//go:build windows

package pty

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		shell = "cmd.exe"
	}
	shell, err = resolveWindowsProgram(shell)
	if err != nil {
		_ = cp.Close()
		return nil, err
	}
	args := opts.ShellArgs
	if len(args) == 0 {
		args = []string{shell}
	} else if args[0] == opts.ShellProgram {
		args[0] = shell
	} else if args[0] != shell {
		args = append([]string{shell}, args...)
	}

	// Pass the full parent environment (PATH etc.) to the child, mirroring the
	// Unix path. conpty.Spawn treats a nil ProcAttr as an EMPTY environment (only
	// SYSTEMROOT survives), which leaves the shell without a PATH — every external
	// program (git, node, python, npx...) then fails to launch.
	env := append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	// Advertise ANSI/VT color support the way Windows console apps detect it. Many
	// (Django, and CLIs using colorama/supports-color) gate coloring on ANSICON
	// rather than the Unix TERM/COLORTERM, so without this they print monochrome
	// even though CervTerm renders SGR fine. The value is the ansicon convention
	// (COLUMNSxROWS). Don't override a real ansicon the user is already running under.
	if !hasEnvKey(env, "ANSICON") {
		env = append(env, fmt.Sprintf("ANSICON=%dx%d", cols, rows))
	}
	for key, value := range opts.Env {
		env = append(env, key+"="+value)
	}
	_, handle, err := cp.Spawn(shell, args, &syscall.ProcAttr{Env: env})
	if err != nil {
		_ = cp.Close()
		return nil, err
	}

	return &localSession{pty: cp, handle: handle}, nil
}

// hasEnvKey reports whether env already contains a "KEY=..." entry for key,
// matched case-insensitively like Windows environment variables.
func hasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, kv := range env {
		if len(kv) >= len(prefix) && strings.EqualFold(kv[:len(prefix)], prefix) {
			return true
		}
	}
	return false
}

func resolveWindowsProgram(program string) (string, error) {
	if program == "" {
		return program, nil
	}
	if filepath.IsAbs(program) || filepath.Dir(program) != "." {
		return program, nil
	}
	resolved, err := exec.LookPath(program)
	if err != nil {
		return program, err
	}
	return resolved, nil
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
	err := s.pty.Close()
	if s.handle != 0 {
		_ = syscall.CloseHandle(syscall.Handle(s.handle))
		s.handle = 0
	}
	return err
}
