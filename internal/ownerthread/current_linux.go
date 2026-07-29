//go:build linux

package ownerthread

import "syscall"

func current() ID { return ID(syscall.Gettid()) }
