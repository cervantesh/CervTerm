//go:build glfw

package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"

	"cervterm/internal/applog"
	"cervterm/internal/buildinfo"
	"cervterm/internal/config"
	"cervterm/internal/fontglyph"
	"cervterm/internal/frontend/glfwgl"
	"cervterm/internal/script"
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
		var warnings []string
		for _, warning := range fontglyph.DiagnoseEmojiFonts().Warnings {
			warnings = append(warnings, warning)
		}
		os.Exit(runDoctor(doctorOptions{ConfigPath: *configPath, LogPath: *logPath, EmojiWarnings: warnings, ContentScale: "not probed in diagnostic mode", CandidateOptions: candidateOptions}))
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
	// Opt-in profiling endpoint: CERVTERM_PPROF=localhost:6060 exposes
	// /debug/pprof. Only loopback addresses are accepted; off by default.
	if addr := os.Getenv("CERVTERM_PPROF"); addr != "" {
		host, _, err := net.SplitHostPort(addr)
		if err != nil || (host != "localhost" && host != "127.0.0.1" && host != "::1") {
			log.Printf("pprof: refusing non-loopback bind %q; use a loopback address like 127.0.0.1:6060", addr)
		} else {
			go func() {
				log.Printf("pprof listening on http://%s/debug/pprof/", addr)
				if err := http.ListenAndServe(addr, nil); err != nil {
					log.Printf("pprof server stopped: %v", err)
				}
			}()
		}
	}
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

	path := *configPath
	if path == "" {
		path = config.DiscoverPath()
	}
	if err := validateCompositionTarget(compositionFlags, path, 0); err != nil {
		log.Fatal(err)
	}
	loaded := script.VersionedSource{Config: config.Defaults(), AuthoredVersion: 1}
	if path != "" {
		var loadErr error
		loaded, loadErr = script.LoadVersioned(path, loaded.Config, candidateOptions)
		if loadErr != nil {
			log.Fatal(loadErr)
		}
		log.Printf("loaded config v%d: %s", loaded.AuthoredVersion, path)
	}
	if err := validateCompositionTarget(compositionFlags, path, loaded.AuthoredVersion); err != nil {
		closeVersionedSource(&loaded)
		log.Fatal(err)
	}
	if err := loaded.Config.Validate(); err != nil {
		if loaded.Candidate != nil {
			loaded.Candidate.Close()
		} else if loaded.Runtime != nil {
			loaded.Runtime.Close()
		}
		if loaded.LegacyTransition != nil {
			_ = loaded.LegacyTransition.Rollback()
		}
		log.Fatal(err)
	}
	if err := glfwgl.RunWithVersioned(loaded, path); err != nil {
		log.Fatal(err)
	}
}
