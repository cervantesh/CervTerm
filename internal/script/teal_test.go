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
	bundle, err := BuildCandidateBundle(filepath.Join(dir, "cervterm.tl"), config.Defaults(), CandidateOptions{})
	if err != nil {
		t.Fatalf("typed default candidate load failed: %v", err)
	}
	defer bundle.Close()
	cfg := bundle.Config()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("typed default should validate: %v", err)
	}
	if cfg.ColorScheme != "Shades of Purple" || cfg.Colors.Background != "#2D2B55" || cfg.IME.Enabled || cfg.Accessibility.Enabled || cfg.Accessibility.Scope != "visible" || cfg.Graphics.Kitty.Enabled || cfg.Graphics.Sixel.Enabled || cfg.Graphics.ITerm.Enabled || cfg.Graphics.Limits.GPUBytesPerContext != 268435456 {
		t.Fatalf("typed defaults drifted: scheme=%q background=%q ime=%v accessibility=%#v graphics=%#v", cfg.ColorScheme, cfg.Colors.Background, cfg.IME.Enabled, cfg.Accessibility, cfg.Graphics)
	}
}

func TestTypedGraphicsProtocolFlagsTealExample(t *testing.T) {
	if _, err := exec.LookPath("tl"); err != nil {
		t.Skip("tl not installed")
	}
	dir := t.TempDir()
	copyFile(t, filepath.Join("..", "..", "docs", "examples", "cervterm.d.tl"), filepath.Join(dir, "cervterm.d.tl"))
	source := `local cervterm = require("cervterm")
local config: cervterm.Config = {
	config_version = 2,
	graphics = {
		sixel = { enabled = true },
		iterm = { enabled = true },
	},
}
return config
`
	path := filepath.Join(dir, "cervterm.tl")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle, err := BuildCandidateBundle(path, config.Defaults(), CandidateOptions{})
	if err != nil {
		t.Fatalf("typed graphics candidate load failed: %v", err)
	}
	defer bundle.Close()
	cfg := bundle.Config()
	if !cfg.Graphics.Sixel.Enabled || !cfg.Graphics.ITerm.Enabled || cfg.Graphics.Kitty.Enabled {
		t.Fatalf("typed graphics flags=%#v", cfg.Graphics)
	}
}

func TestTypedActionsTealExample(t *testing.T) {
	if _, err := exec.LookPath("tl"); err != nil {
		t.Skip("tl not installed")
	}
	dir := t.TempDir()
	copyFile(t, filepath.Join("..", "..", "docs", "examples", "cervterm.d.tl"), filepath.Join(dir, "cervterm.d.tl"))
	copyFile(t, filepath.Join("..", "..", "docs", "examples", "cervterm-actions-example.tl"), filepath.Join(dir, "cervterm.tl"))
	_, rt, err := Load(filepath.Join(dir, "cervterm.tl"), config.Defaults())
	if err != nil {
		t.Fatalf("typed actions Load failed: %v", err)
	}
	defer rt.Close()
	bindings := rt.Bindings()
	if len(bindings) != 4 || bindings[0].Label != "Copy selection" {
		t.Fatalf("bindings = %#v", bindings)
	}
}

func TestTealConfigUnsetTypeContract(t *testing.T) {
	if _, err := exec.LookPath("tl"); err != nil {
		t.Skip("tl not installed")
	}
	dir := t.TempDir()
	copyFile(t, filepath.Join("..", "..", "docs", "examples", "cervterm.d.tl"), filepath.Join(dir, "cervterm.d.tl"))
	source := `local cervterm = require("cervterm")
local cfg: cervterm.Config = {
  config_version = 2,
  font = { family = "Mono", size = cervterm.config.unset, ligatures = true },
  default_environment = "windows",
  default_profile = "work",
  environments = { windows = { font = { family = "Env", size = 12, ligatures = true } } },
  profiles = { work = { font = { family = cervterm.config.unset, size = 14, ligatures = false } } },
}
return cfg
`
	path := filepath.Join(dir, "cervterm.tl")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.GenerateTeal(path); err != nil {
		t.Fatalf("unset Teal contract failed: %v", err)
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
