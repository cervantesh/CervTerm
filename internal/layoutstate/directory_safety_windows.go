//go:build windows

package layoutstate

import "os"

// Windows confidentiality and mutation resistance are provided by the directory ACL.
func directoryModeSafe(info os.FileInfo) bool {
	return info != nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
}
