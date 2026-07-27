//go:build windows

package platform

import "syscall"

func directWriteAvailable() bool {
	dll, err := syscall.LoadDLL("dwrite.dll")
	if err != nil {
		return false
	}
	defer dll.Release()
	proc, err := dll.FindProc("DWriteCreateFactory")
	return err == nil && proc != nil
}
