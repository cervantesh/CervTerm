package applog

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
)

const EnvLogFile = "CERVTERM_LOG_FILE"

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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	log.SetOutput(io.MultiWriter(os.Stderr, file))
	log.Printf("CervTerm diagnostics logging to %s", path)
	return file, nil
}

// RecoverAndExit records an unexpected panic with a stack trace before exiting.
func RecoverAndExit(context string) {
	if recovered := recover(); recovered != nil {
		log.Printf("panic in %s: %v\n%s", context, recovered, debug.Stack())
		_, _ = fmt.Fprintf(os.Stderr, "CervTerm crashed; see diagnostics log for details: %v\n", recovered)
		os.Exit(2)
	}
}
