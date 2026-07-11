package config

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func GenerateTeal(path string) (string, error) {
	tl, err := exec.LookPath("tl")
	if err != nil {
		return "", fmt.Errorf("Teal config %q requires the `tl` command; install Teal or generate cervterm.lua: %w", path, err)
	}
	dir := filepath.Dir(path)
	if out, err := exec.Command(tl, "check", "-I", dir, path).CombinedOutput(); err != nil {
		return "", fmt.Errorf("tl check failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if out, err := exec.Command(tl, "gen", "-I", dir, path).CombinedOutput(); err != nil {
		return "", fmt.Errorf("tl gen failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSuffix(path, filepath.Ext(path)) + ".lua", nil
}
