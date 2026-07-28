//go:build windows

package glfwgl

import (
	"errors"
	"strings"
	"testing"
)

func TestWindowsInitialWindowCompositorFlushAdapter(t *testing.T) {
	calls := 0
	adapter := windowsInitialWindowCompositor{flush: func() (uintptr, error) {
		calls++
		return 0, nil
	}}
	if err := adapter.Flush(); err != nil || calls != 1 {
		t.Fatalf("Flush() calls=%d err=%v", calls, err)
	}
}

func TestWindowsInitialWindowCompositorReportsLoadAndHRESULTFailures(t *testing.T) {
	loadErr := errors.New("load")
	for _, test := range []struct {
		name string
		call func() (uintptr, error)
		want string
	}{
		{name: "load", call: func() (uintptr, error) { return 0, loadErr }, want: "load"},
		{name: "S_FALSE", call: func() (uintptr, error) { return 1, nil }, want: "0x00000001"},
		{name: "hresult", call: func() (uintptr, error) { return 0x80004005, nil }, want: "0x80004005"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := (windowsInitialWindowCompositor{flush: test.call}).Flush()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Flush() err=%v want substring %q", err, test.want)
			}
		})
	}
}
