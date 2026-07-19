//go:build !windows

package layoutstate

import (
	"os"

	"golang.org/x/sys/unix"
)

func openLayoutFile(path string) (storeFile, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
