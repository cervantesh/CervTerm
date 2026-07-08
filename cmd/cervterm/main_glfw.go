//go:build glfw

package main

import (
	"flag"
	"fmt"
	"log"

	"cervterm/internal/buildinfo"
	"cervterm/internal/config"
	"cervterm/internal/frontend/glfwgl"
)

func main() {
	configPath := flag.String("config", "", "path to cervterm.lua or cervterm.tl")
	showVersion := flag.Bool("version", false, "print CervTerm version")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildinfo.Version)
		return
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
