//go:build windows

package glfwgl

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type initialWindowSetAttributeArgs struct {
	hwnd      uintptr
	attribute uint32
	value     int32
	valueSize uint32
}

func TestWindowsInitialWindowRevealAdapterCallArguments(t *testing.T) {
	var calls []initialWindowSetAttributeArgs
	reveal := windowsInitialWindowReveal{
		hwnd: 0x1234,
		setAttribute: func(hwnd uintptr, attribute uint32, value *int32, valueSize uint32) (uintptr, error) {
			calls = append(calls, initialWindowSetAttributeArgs{
				hwnd:      hwnd,
				attribute: attribute,
				value:     *value,
				valueSize: valueSize,
			})
			return 0, nil
		},
	}

	supported, err := reveal.Conceal()
	if err != nil || !supported {
		t.Fatalf("Conceal() supported=%t err=%v", supported, err)
	}
	if err := reveal.Restore(); err != nil {
		t.Fatalf("Restore() err=%v", err)
	}

	want := []initialWindowSetAttributeArgs{
		{hwnd: 0x1234, attribute: 13, value: 1, valueSize: 4},
		{hwnd: 0x1234, attribute: 13, value: 0, valueSize: 4},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("DwmSetWindowAttribute calls=%+v want=%+v", calls, want)
	}
}

func TestWindowsInitialWindowRevealChecksExactHRESULT(t *testing.T) {
	loadErr := errors.New("load")
	for _, test := range []struct {
		name    string
		hresult uintptr
		callErr error
		want    string
	}{
		{name: "load", callErr: loadErr, want: "load"},
		{name: "S_FALSE is not accepted", hresult: 1, want: "0x00000001"},
		{name: "failure HRESULT", hresult: 0x80004005, want: "0x80004005"},
	} {
		t.Run(test.name, func(t *testing.T) {
			reveal := windowsInitialWindowReveal{
				hwnd: 1,
				setAttribute: func(uintptr, uint32, *int32, uint32) (uintptr, error) {
					return test.hresult, test.callErr
				},
			}
			_, err := reveal.Conceal()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Conceal() err=%v want substring %q", err, test.want)
			}
		})
	}
}

func TestWindowsInitialWindowRevealChecksUncloakHRESULT(t *testing.T) {
	reveal := windowsInitialWindowReveal{
		hwnd: 1,
		setAttribute: func(_ uintptr, _ uint32, value *int32, _ uint32) (uintptr, error) {
			if *value != 0 {
				t.Fatalf("uncloak value=%d want=0", *value)
			}
			return 0x80070005, nil
		},
	}
	err := reveal.Restore()
	if err == nil || !strings.Contains(err.Error(), "DWMWA_CLOAK=false") || !strings.Contains(err.Error(), "0x80070005") {
		t.Fatalf("Restore() err=%v", err)
	}
}

func TestWindowsInitialWindowRevealRejectsMissingHWNDBeforeSyscall(t *testing.T) {
	called := false
	reveal := windowsInitialWindowReveal{
		setAttribute: func(uintptr, uint32, *int32, uint32) (uintptr, error) {
			called = true
			return 0, nil
		},
	}
	_, err := reveal.Conceal()
	if err == nil || !strings.Contains(err.Error(), "missing HWND") {
		t.Fatalf("Conceal() err=%v", err)
	}
	if called {
		t.Fatal("DwmSetWindowAttribute called with a missing HWND")
	}
}
