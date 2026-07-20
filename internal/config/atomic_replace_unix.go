//go:build !windows

package config

import (
	"os"
	"path/filepath"
)

func atomicReplaceFile(staged, destination string) error {
	if err := os.Rename(staged, destination); err != nil {
		return err
	}
	return syncParentDirectory(destination)
}

func syncParentDirectory(path string) error {
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
