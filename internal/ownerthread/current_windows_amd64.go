//go:build windows && amd64

package ownerthread

// current reads NT_TIB/TEB ClientId.UniqueThread directly. On Windows/amd64 the
// TEB is addressed by GS and UniqueThread is the pointer-sized value at 0x48.
// This is the exact value returned by GetCurrentThreadId without syscall-wrapper
// overhead on every owner sink.
func current() ID
