//go:build !windows

package layoutstate

import (
	"errors"
	"os"
	"syscall"
)

func fileHasMultipleLinks(_ string, info os.FileInfo) (bool, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false, errors.New("unsupported file metadata")
	}
	return stat.Nlink > 1, nil
}
