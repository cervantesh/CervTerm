package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGenerateTealMissingTool(t *testing.T) {
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", "")
	_, err := GenerateTeal(filepath.Join(t.TempDir(), "cervterm.tl"))
	if err == nil {
		t.Fatalf("expected missing tl error")
	}
	t.Setenv("PATH", oldPath)
}

func TestGenerateTealHappyPathWithFakeTool(t *testing.T) {
	dir := t.TempDir()
	tlPath := filepath.Join(dir, "tl")
	if runtime.GOOS == "windows" {
		tlPath += ".bat"
		writeTestFile(t, tlPath, "@echo off\r\nif \"%1\"==\"check\" if \"%2\"==\"-I\" exit /b 0\r\nif \"%1\"==\"gen\" if \"%2\"==\"-I\" copy \"%4\" \"%~dpn4.lua\" >nul & exit /b 0\r\nexit /b 1\r\n")
	} else {
		writeTestFile(t, tlPath, "#!/bin/sh\nif [ \"$1\" = check ] && [ \"$2\" = -I ]; then exit 0; fi\nif [ \"$1\" = gen ] && [ \"$2\" = -I ]; then cp \"$4\" \"${4%.tl}.lua\"; exit 0; fi\nexit 1\n")
		if err := os.Chmod(tlPath, 0o755); err != nil {
			t.Fatalf("chmod fake tl: %v", err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	tealPath := filepath.Join(dir, "cervterm.tl")
	writeTestFile(t, tealPath, `return { window = { width = 1200, height = 800 } }`)
	luaPath, err := GenerateTeal(tealPath)
	if err != nil {
		t.Fatalf("GenerateTeal failed: %v", err)
	}
	if _, err := os.Stat(luaPath); err != nil {
		t.Fatalf("expected generated lua file: %v", err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}
