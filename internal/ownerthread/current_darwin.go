//go:build darwin

package ownerthread

import "syscall"

func current() ID {
	id, _, errno := syscall.RawSyscall(syscall.SYS_THREAD_SELFID, 0, 0, 0)
	if errno != 0 {
		return 0
	}
	return ID(id)
}
