//go:build linux

package mux

import "cervterm/internal/ownerthread"

func currentOwnerThreadID() uint64 { return uint64(ownerthread.Current()) }
