//go:build glfw

package glfwgl

import (
	"errors"

	"cervterm/internal/script"
)

// RunWithCandidate consumes candidate ownership even when frontend startup fails.
func RunWithCandidate(candidate *script.CandidateBundle, sourcePath string) error {
	if candidate == nil {
		return errors.New("candidate bundle is required")
	}
	activation, err := candidate.PrepareActivation()
	if err != nil {
		candidate.Close()
		return err
	}
	return runWithSource(candidate.Config(), nil, candidate, activation, nil, candidate.WatchPaths(), candidate.WatchHashes(), sourcePath, candidate.Options())
}

// RunWithVersioned consumes all ownership carried by a version-aware load.
func RunWithVersioned(loaded script.VersionedSource, sourcePath string) error {
	if loaded.Candidate != nil {
		return RunWithCandidate(loaded.Candidate, sourcePath)
	}
	return runWithSource(loaded.Config, loaded.Runtime, nil, nil, loaded.LegacyTransition, loaded.WatchPaths, loaded.WatchHashes, sourcePath, loaded.Options)
}
