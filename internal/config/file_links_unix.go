//go:build !windows

package config

import (
	"fmt"
	"os"
	"syscall"
)

func fileHasMultipleLinks(path string, info os.FileInfo) (bool, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false, fmt.Errorf("inspect hard-link count for %q: unsupported file metadata", path)
	}
	return stat.Nlink > 1, nil
}
