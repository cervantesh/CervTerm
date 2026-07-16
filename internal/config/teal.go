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
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve Teal config %q: %w", path, err)
	}
	dir := filepath.Dir(absolutePath)
	source := filepath.Base(absolutePath)
	check := exec.Command(tl, "check", "-I", dir, source)
	check.Dir = dir
	if out, err := check.CombinedOutput(); err != nil {
		return "", fmt.Errorf("tl check failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	generate := exec.Command(tl, "gen", "-I", dir, source)
	generate.Dir = dir
	if out, err := generate.CombinedOutput(); err != nil {
		return "", fmt.Errorf("tl gen failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSuffix(absolutePath, filepath.Ext(absolutePath)) + ".lua", nil
}
