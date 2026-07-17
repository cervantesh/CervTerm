//go:build !glfw

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"cervterm/internal/applog"
	"cervterm/internal/buildinfo"
	"cervterm/internal/config"
)

func main() {
	configPath := flag.String("config", "", "path to cervterm.lua or cervterm.tl")
	compositionFlags := registerCompositionFlags(flag.CommandLine)
	explainFlags := registerExplainConfigFlags(flag.CommandLine)
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
	candidateOptions, err := compositionFlags.candidateOptions(os.Args[1:], os.LookupEnv)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if explainFlags.requested() {
		os.Exit(runExplainConfig(configDiagnosticOptions{ConfigPath: *configPath, Candidate: candidateOptions, Fields: append([]string(nil), explainFlags.fields...)}))
	}
	if *doctor {
		os.Exit(runDoctor(doctorOptions{ConfigPath: *configPath, LogPath: *logPath, CandidateOptions: candidateOptions}))
	}
	if compositionFlags.explicitlyRequested() {
		fmt.Fprintln(os.Stderr, "--environment, --profile, and --config-override require the GLFW frontend build")
		os.Exit(2)
	}
	logFile, err := applog.Setup(applog.ResolvePath(*logPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "logging setup failed: %v\n", err)
		os.Exit(1)
	}
	if logFile != nil {
		defer logFile.Close()
	}
	defer applog.RecoverAndExit("headless main")
	if *capturePath != "" {
		if err := runVTCapture(vtCaptureOptions{Path: *capturePath, Program: *captureProgram, Args: captureProgramArgs, Rows: uint16(max(1, *captureRows)), Cols: uint16(max(1, *captureCols)), Timeout: *captureTimeout}); err != nil {
			log.Printf("capture failed: %v", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	fmt.Println("CervTerm: headless build is active.")
	fmt.Println("Run tests with: go test ./...")
	fmt.Println("Run optional GLFW/OpenGL frontend with: go run -tags glfw ./cmd/cervterm")
}
