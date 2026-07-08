//go:build !glfw

package main

import (
	"flag"
	"fmt"

	"cervterm/internal/buildinfo"
)

func main() {
	showVersion := flag.Bool("version", false, "print CervTerm version")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildinfo.Version)
		return
	}
	fmt.Println("CervTerm: headless build is active.")
	fmt.Println("Run tests with: go test ./...")
	fmt.Println("Run optional GLFW/OpenGL frontend with: go run -tags glfw ./cmd/cervterm")
}
