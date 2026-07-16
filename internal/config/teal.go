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
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve Teal config %q: %w", path, err)
	}
	dir := filepath.Dir(absPath)
	base := filepath.Base(absPath)
	run := func(action string) ([]byte, error) {
		cmd := exec.Command(tl, action, "-I", dir, base)
		// Teal 0.24 emits a basename into the process working directory when the
		// source is on a different Windows drive. Running in the source directory
		// guarantees that generated Lua is adjacent to the selected .tl file.
		cmd.Dir = dir
		return cmd.CombinedOutput()
	}
	if out, err := run("check"); err != nil {
		return "", fmt.Errorf("tl check failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if out, err := run("gen"); err != nil {
		return "", fmt.Errorf("tl gen failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return filepath.Join(dir, strings.TrimSuffix(base, filepath.Ext(base))+".lua"), nil
}
