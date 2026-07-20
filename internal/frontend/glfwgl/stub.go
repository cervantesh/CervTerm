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

func RunWithSource(cfg config.Config, rt *script.Runtime, sourcePath string) error {
	return errors.New("glfw frontend requires building with -tags glfw")
}

// RunWithCandidate consumes candidate ownership consistently with GLFW builds.
func RunWithCandidate(candidate *script.CandidateBundle, sourcePath string) error {
	if candidate != nil {
		candidate.Close()
	}
	return errors.New("glfw frontend requires building with -tags glfw")
}

func RunWithVersioned(loaded script.VersionedSource, sourcePath string) error {
	if loaded.Candidate != nil {
		loaded.Candidate.Close()
	} else if loaded.Runtime != nil {
		loaded.Runtime.Close()
	}
	if loaded.LegacyTransition != nil {
		_ = loaded.LegacyTransition.Rollback()
	}
	return errors.New("glfw frontend requires building with -tags glfw")
}

type glyphAtlas struct{}
