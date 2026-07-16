package script

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cervterm/internal/config"
)

func TestLoadTealExample(t *testing.T) {
	if _, err := exec.LookPath("tl"); err != nil {
		t.Skip("tl not installed")
	}
	dir := t.TempDir()
	copyFile(t, filepath.Join("..", "..", "docs", "examples", "cervterm.d.tl"), filepath.Join(dir, "cervterm.d.tl"))
	copyFile(t, filepath.Join("..", "..", "docs", "examples", "cervterm-keys-example.tl"), filepath.Join(dir, "cervterm.tl"))

	_, rt, err := Load(filepath.Join(dir, "cervterm.tl"), config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	host := &fakeHost{}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if got := strings.Join(host.writes, ""); got != "echo hola desde teal\r" {
		t.Fatalf("writes = %q", got)
	}
}

func TestTypedDefaultTealExample(t *testing.T) {
	if _, err := exec.LookPath("tl"); err != nil {
		t.Skip("tl not installed")
	}
	dir := t.TempDir()
	copyFile(t, filepath.Join("..", "..", "docs", "examples", "cervterm.d.tl"), filepath.Join(dir, "cervterm.d.tl"))
	copyFile(t, filepath.Join("..", "..", "docs", "examples", "cervterm.tl"), filepath.Join(dir, "cervterm.tl"))
	cfg, rt, err := Load(filepath.Join(dir, "cervterm.tl"), config.Defaults())
	if err != nil {
		t.Fatalf("typed default Load failed: %v", err)
	}
	defer rt.Close()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("typed default should validate: %v", err)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}
