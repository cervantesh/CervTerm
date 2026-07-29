//go:build windows

package ownerthread

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestCurrentMatchesWindowsThreadIdentity(t *testing.T) {
	if got, want := Current(), ID(windows.GetCurrentThreadId()); got == 0 || got != want {
		t.Fatalf("Current()=%d GetCurrentThreadId()=%d", got, want)
	}
}
