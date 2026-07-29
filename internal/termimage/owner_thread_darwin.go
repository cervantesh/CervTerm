//go:build darwin

package termimage

import "cervterm/internal/ownerthread"

func currentStoreOwnerThreadID() uint64 { return uint64(ownerthread.Current()) }
