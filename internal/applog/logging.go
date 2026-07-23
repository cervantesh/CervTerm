package applog

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
)

const EnvLogFile = "CERVTERM_LOG_FILE"

var ErrSetup = errors.New("diagnostics logging setup failed")

// DefaultPath returns CervTerm's best-effort local diagnostic log path.
func DefaultPath() string {
	if dir, err := os.UserCacheDir(); err == nil && dir != "" {
		return filepath.Join(dir, "cervterm", "cervterm.log")
	}
	return filepath.Join(os.TempDir(), "cervterm", "cervterm.log")
}

// ResolvePath maps the CLI flag/env value to a concrete log path.
// Empty means use CERVTERM_LOG_FILE when set, otherwise the default log file.
// "-", "stderr", and "none" leave logging on stderr only.
func ResolvePath(flagValue string) string {
	value := strings.TrimSpace(flagValue)
	if value == "" {
		value = strings.TrimSpace(os.Getenv(EnvLogFile))
	}
	if value == "" {
		return DefaultPath()
	}
	lower := strings.ToLower(value)
	if lower == "-" || lower == "stderr" || lower == "none" {
		return ""
	}
	return value
}

// Setup configures the standard logger to write diagnostics to stderr and,
// when a path is configured, append to a local log file. The caller owns the
// returned file and should close it during shutdown.
func Setup(path string) (*os.File, error) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	if path == "" {
		log.SetOutput(os.Stderr)
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, setupError("create-directory")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, setupError("open-file")
	}
	log.SetOutput(io.MultiWriter(os.Stderr, file))
	log.Print("CervTerm diagnostics logging enabled (path redacted)")
	return file, nil
}

func setupError(stage string) error {
	return fmt.Errorf("%w (%s)", ErrSetup, stage)
}

// RecoverAndExit records a bounded, value-free panic classification and a
// source-path-redacted stack before exiting.
func RecoverAndExit(context string) {
	if recovered := recover(); recovered != nil {
		log.Print(formatPanicReport(context, recovered, debug.Stack()))
		_, _ = fmt.Fprintln(os.Stderr, "CervTerm crashed; see diagnostics log for details")
		os.Exit(2)
	}
}

func formatPanicReport(context string, recovered any, stack []byte) string {
	return fmt.Sprintf("panic context=%s class=%s\n%s", safePanicContext(context), panicClass(recovered), redactStackPaths(stack))
}

func safePanicContext(context string) string {
	switch context {
	case "headless main":
		return "headless-main"
	case "glfw main":
		return "glfw-main"
	default:
		return "unknown"
	}
}

func panicClass(recovered any) string {
	switch recovered.(type) {
	case runtime.Error:
		return "runtime-error"
	case error:
		return "error"
	case string:
		return "string"
	default:
		return "value"
	}
}

func redactStackPaths(stack []byte) string {
	lines := strings.Split(strings.TrimSpace(string(stack)), "\n")
	for index, line := range lines {
		if !strings.HasPrefix(line, "\t") {
			continue
		}
		trimmed := strings.TrimSpace(line)
		suffix := ""
		if offset := strings.LastIndex(trimmed, " +0x"); offset >= 0 {
			suffix = trimmed[offset:]
			trimmed = trimmed[:offset]
		}
		separator := strings.LastIndex(trimmed, ":")
		if separator < 0 {
			lines[index] = "\t<source>" + suffix
			continue
		}
		file := path.Base(strings.ReplaceAll(trimmed[:separator], "\\", "/"))
		lineNumber := trimmed[separator+1:]
		lines[index] = "\t<source>/" + file + ":" + lineNumber + suffix
	}
	return strings.Join(lines, "\n")
}
