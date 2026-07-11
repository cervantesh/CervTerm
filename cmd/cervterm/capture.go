package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"cervterm/internal/pty"
)

type captureArgs []string

func (a *captureArgs) String() string { return strings.Join(*a, " ") }
func (a *captureArgs) Set(value string) error {
	*a = append(*a, value)
	return nil
}

type vtCaptureOptions struct {
	Path    string
	Program string
	Args    []string
	Rows    uint16
	Cols    uint16
	Timeout time.Duration
}

func runVTCapture(opts vtCaptureOptions) error {
	if opts.Path == "" {
		return errors.New("capture path is required")
	}
	if opts.Rows == 0 {
		opts.Rows = 24
	}
	if opts.Cols == 0 {
		opts.Cols = 80
	}
	out, err := os.Create(opts.Path)
	if err != nil {
		return err
	}
	defer out.Close()

	session, err := pty.NewLocalWithOptions(opts.Rows, opts.Cols, pty.Options{ShellProgram: opts.Program, ShellArgs: opts.Args})
	if err != nil {
		return err
	}
	defer session.Close()

	fmt.Fprintf(os.Stderr, "recording raw PTY output to %s (%dx%d); exit the child program or press Ctrl+C to stop\n", opts.Path, opts.Cols, opts.Rows)
	readerDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.MultiWriter(os.Stdout, out), session.Reader())
		readerDone <- err
	}()
	go func() {
		_, _ = io.Copy(session, os.Stdin)
	}()

	var timer <-chan time.Time
	if opts.Timeout > 0 {
		t := time.NewTimer(opts.Timeout)
		defer t.Stop()
		timer = t.C
	}
	select {
	case err := <-readerDone:
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		return nil
	case <-timer:
		_ = session.Close()
		<-readerDone
		return nil
	}
}
