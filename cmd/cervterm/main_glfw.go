//go:build glfw

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"cervterm/internal/applog"
	"cervterm/internal/buildinfo"
	"cervterm/internal/config"
	"cervterm/internal/fontglyph"
	"cervterm/internal/frontend/glfwgl"
)

func main() {
	configPath := flag.String("config", "", "path to cervterm.lua or cervterm.tl")
	showVersion := flag.Bool("version", false, "print CervTerm version")
	showBuildInfo := flag.Bool("build-info", false, "print CervTerm build information")
	printDefaultConfig := flag.Bool("print-default-config", false, "print default Lua configuration")
	doctor := flag.Bool("doctor", false, "print diagnostic environment report")
	logPath := flag.String("log-file", "", "diagnostic log path (default: user cache; use '-' for stderr only; env: CERVTERM_LOG_FILE)")
	capturePath := flag.String("capture-vt", "", "record raw PTY output to this .vt file")
	captureProgram := flag.String("capture-program", "", "program to run for --capture-vt (defaults to the configured shell)")
	var captureProgramArgs captureArgs
	flag.Var(&captureProgramArgs, "capture-arg", "argument for --capture-program; repeat for multiple args")
	captureTimeout := flag.Duration("capture-timeout", 0, "optional maximum duration for --capture-vt, e.g. 30s")
	captureRows := flag.Int("capture-rows", 24, "PTY rows for --capture-vt")
	captureCols := flag.Int("capture-cols", 80, "PTY columns for --capture-vt")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildinfo.Version)
		return
	}
	if *showBuildInfo {
		fmt.Println(buildinfo.Info())
		return
	}
	if *printDefaultConfig {
		fmt.Print(config.DefaultLua())
		return
	}
	if *doctor {
		var warnings []string
		for _, warning := range fontglyph.DiagnoseEmojiFonts().Warnings {
			warnings = append(warnings, warning)
		}
		os.Exit(runDoctor(doctorOptions{ConfigPath: *configPath, LogPath: *logPath, EmojiWarnings: warnings}))
	}
	logFile, err := applog.Setup(applog.ResolvePath(*logPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "logging setup failed: %v\n", err)
		os.Exit(1)
	}
	if logFile != nil {
		defer logFile.Close()
	}
	defer applog.RecoverAndExit("glfw main")
	for _, warning := range fontglyph.DiagnoseEmojiFonts().Warnings {
		log.Printf("emoji coverage warning: %s", warning)
	}
	if *capturePath != "" {
		if err := runVTCapture(vtCaptureOptions{Path: *capturePath, Program: *captureProgram, Args: captureProgramArgs, Rows: uint16(max(1, *captureRows)), Cols: uint16(max(1, *captureCols)), Timeout: *captureTimeout}); err != nil {
			log.Printf("capture failed: %v", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	cfg, loadedPath, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if loadedPath != "" {
		log.Printf("loaded config: %s", loadedPath)
	}
	if err := glfwgl.RunWithConfig(cfg); err != nil {
		log.Fatal(err)
	}
}
