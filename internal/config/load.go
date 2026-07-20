package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func Load(path string) (Config, string, error) {
	cfg := Defaults()
	resolved := path
	if resolved == "" {
		resolved = DiscoverPath()
	}
	if resolved == "" {
		return cfg, "", nil
	}
	source := resolved
	if strings.HasSuffix(resolved, ".tl") {
		luaPath, err := GenerateTeal(resolved)
		if err != nil {
			return cfg, resolved, err
		}
		resolved = luaPath
	}
	loaded, err := loadLua(resolved, source, cfg)
	if err != nil {
		return cfg, resolved, err
	}
	if err := loaded.Validate(); err != nil {
		return cfg, resolved, err
	}
	return loaded, resolved, nil
}

func DiscoverPath() string {
	for _, path := range CandidatePaths() {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func CandidatePaths() []string {
	var paths []string
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		paths = append(paths, filepath.Join(dir, "cervterm.tl"), filepath.Join(dir, "cervterm.lua"))
	}
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			base := filepath.Join(appData, "cervterm")
			paths = append(paths, filepath.Join(base, "cervterm.tl"), filepath.Join(base, "cervterm.lua"))
		}
		return paths
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			base = filepath.Join(home, ".config")
		}
	}
	if base != "" {
		base = filepath.Join(base, "cervterm")
		paths = append(paths, filepath.Join(base, "cervterm.tl"), filepath.Join(base, "cervterm.lua"))
	}
	return paths
}

func missingRequired(path string) error {
	if path == "" {
		return errors.New("config path is empty")
	}
	return fmt.Errorf("config file %q not found", path)
}
