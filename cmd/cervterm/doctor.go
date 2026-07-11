package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"cervterm/internal/applog"
	"cervterm/internal/buildinfo"
	"cervterm/internal/config"
	"cervterm/internal/fontglyph"
)

type doctorOptions struct {
	ConfigPath    string
	LogPath       string
	EmojiWarnings []string
	ContentScale  string
}

func runDoctor(opts doctorOptions) int {
	fmt.Println("CervTerm doctor")
	fmt.Printf("version: %s\n", buildinfo.Info())
	fmt.Printf("platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)

	if exe, err := os.Executable(); err == nil {
		fmt.Printf("executable: %s\n", exe)
	} else {
		fmt.Printf("executable: unavailable (%v)\n", err)
	}
	if cwd, err := os.Getwd(); err == nil {
		fmt.Printf("working-directory: %s\n", cwd)
	}

	printConfigDoctor(opts.ConfigPath)
	printLogDoctor(opts.LogPath)
	printEnvironmentDoctor()
	if opts.ContentScale == "" {
		fmt.Println("content-scale: unavailable in headless build")
	} else {
		fmt.Printf("content-scale: %s\n", opts.ContentScale)
	}
	printEmojiDoctor(opts.EmojiWarnings)
	fmt.Println("support: attach this output, the diagnostics log, and screenshots/captures when filing an issue")
	return 0
}

func printConfigDoctor(configPath string) {
	fmt.Println("config:")
	if strings.TrimSpace(configPath) != "" {
		fmt.Printf("  override: %s\n", configPath)
	} else if discovered := config.DiscoverPath(); discovered != "" {
		fmt.Printf("  discovered: %s\n", discovered)
	} else {
		fmt.Println("  discovered: none (defaults will be used)")
	}
	fmt.Println("  candidates:")
	for _, candidate := range config.CandidatePaths() {
		status := "missing"
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			status = "present"
		}
		fmt.Printf("    - %s [%s]\n", candidate, status)
	}

	cfg, loadedPath, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("  load: error: %v\n", err)
		return
	}
	if loadedPath == "" {
		fmt.Println("  load: defaults")
	} else {
		fmt.Printf("  load: %s\n", loadedPath)
	}
	if cfg.Shell.Program == "" {
		fmt.Println("  shell: platform default")
	} else {
		fmt.Printf("  shell: %s %s\n", cfg.Shell.Program, strings.Join(cfg.Shell.Args, " "))
	}
	if cfg.Shell.WorkingDirectory != "" {
		fmt.Printf("  shell-working-directory: %s\n", cfg.Shell.WorkingDirectory)
	}
	printFontDoctor(cfg.Font.Family)
}

func printFontDoctor(family string) {
	resolution := fontglyph.ResolveSystemFont(family)
	fmt.Printf("  font-family: %s\n", family)
	if !resolution.Found {
		fmt.Println("  font-resolution: not found (using embedded Go Mono)")
		fmt.Println("  warning: configured font family was not found")
		return
	}
	fmt.Printf("  font-regular: %s\n", resolution.Regular)
	for label, path := range map[string]string{"bold": resolution.Bold, "italic": resolution.Italic, "bold-italic": resolution.BoldItalic} {
		if path != "" {
			fmt.Printf("  font-%s: %s\n", label, path)
		}
	}
}

func printLogDoctor(logPath string) {
	fmt.Println("diagnostics:")
	resolved := applog.ResolvePath(logPath)
	if resolved == "" {
		fmt.Println("  log-file: stderr only")
	} else {
		fmt.Printf("  log-file: %s\n", resolved)
	}
	if env := strings.TrimSpace(os.Getenv(applog.EnvLogFile)); env != "" {
		fmt.Printf("  %s: %s\n", applog.EnvLogFile, env)
	}
}

func printEnvironmentDoctor() {
	fmt.Println("environment:")
	for _, key := range []string{"TERM", "COLORTERM", "SHELL", "ComSpec", "APPDATA", "XDG_CONFIG_HOME"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			fmt.Printf("  %s: %s\n", key, value)
		}
	}
}

func printEmojiDoctor(warnings []string) {
	if len(warnings) == 0 {
		fmt.Println("emoji-fonts: no warnings reported by this build")
		return
	}
	fmt.Println("emoji-fonts:")
	for _, warning := range warnings {
		fmt.Printf("  warning: %s\n", warning)
	}
}
