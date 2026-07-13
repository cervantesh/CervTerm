//go:build !glfw

package glfwgl

import (
	"errors"

	"cervterm/internal/config"
	"cervterm/internal/script"
)

// Run is unavailable in headless builds. Use `-tags glfw` to compile the
// optional GLFW/OpenGL frontend.
func Run() error {
	return errors.New("glfw frontend requires building with -tags glfw")
}

func RunWithConfig(cfg config.Config) error {
	return RunWithOptions(cfg, nil)
}

func RunWithOptions(cfg config.Config, rt *script.Runtime) error {
	return errors.New("glfw frontend requires building with -tags glfw")
}

type glyphAtlas struct{}
