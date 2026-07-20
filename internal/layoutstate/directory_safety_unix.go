//go:build !windows

package layoutstate

import (
	"os"
)

func directoryModeSafe(info os.FileInfo) bool {
	return info != nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 && info.Mode().Perm()&0o022 == 0
}
