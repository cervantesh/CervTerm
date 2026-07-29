//go:build windows && !amd64

package ownerthread

import "golang.org/x/sys/windows"

func current() ID { return ID(windows.GetCurrentThreadId()) }
