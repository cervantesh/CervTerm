package applog

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePathUsesEnvOverride(t *testing.T) {
	t.Setenv(EnvLogFile, filepath.Join(t.TempDir(), "custom.log"))
	got := ResolvePath("")
	if !strings.HasSuffix(got, "custom.log") {
		t.Fatalf("ResolvePath env override = %q", got)
	}
}

func TestResolvePathAllowsStderrOnly(t *testing.T) {
	for _, value := range []string{"-", "stderr", "none", " STDERR "} {
		if got := ResolvePath(value); got != "" {
			t.Fatalf("ResolvePath(%q) = %q, want stderr-only", value, got)
		}
	}
}

func TestSetupAppendsToLogFile(t *testing.T) {
	var previous bytes.Buffer
	log.SetOutput(&previous)
	path := filepath.Join(t.TempDir(), "nested", "cervterm.log")
	file, err := Setup(path)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	log.Print("diagnostic marker")
	if err := file.Close(); err != nil {
		t.Fatalf("close log: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Contains(data, []byte("diagnostic marker")) {
		t.Fatalf("log file missing marker: %q", string(data))
	}
}
