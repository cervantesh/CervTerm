//go:build glfw && windows

package glfwgl

import (
	"errors"
	"testing"
	"unsafe"
)

func TestWindowsIMMABIConstantsAndLayout(t *testing.T) {
	if wmIMEStartComposition != 0x010d || wmIMEEndComposition != 0x010e || wmIMEComposition != 0x010f ||
		gcsCompStr != 0x0008 || gcsCompAttr != 0x0010 || gcsCursorPos != 0x0080 || gcsResultStr != 0x0800 ||
		candidateFormDefault != 0 || candidateFormExclude != 0x80 {
		t.Fatal("IMM ABI constants drifted")
	}
	var form immCandidateForm
	if unsafe.Sizeof(form) != 32 || unsafe.Offsetof(form.Style) != 4 || unsafe.Offsetof(form.Current) != 8 || unsafe.Offsetof(form.Area) != 16 {
		t.Fatalf("candidate form size=%d style=%d current=%d area=%d", unsafe.Sizeof(form), unsafe.Offsetof(form.Style), unsafe.Offsetof(form.Current), unsafe.Offsetof(form.Area))
	}
}

func TestWindowsIMMNativeSignedReadAndCandidateForm(t *testing.T) {
	oldGet, oldRelease := immGetContextCall, immReleaseContextCall
	oldRead, oldCandidate := immGetCompositionStringCall, immSetCandidateWindowCall
	defer func() {
		immGetContextCall, immReleaseContextCall = oldGet, oldRelease
		immGetCompositionStringCall, immSetCandidateWindowCall = oldRead, oldCandidate
	}()
	immGetContextCall = func(hwnd uintptr) uintptr {
		if hwnd != 9 {
			t.Fatalf("hwnd=%d", hwnd)
		}
		return 7
	}
	immReleaseContextCall = func(hwnd, context uintptr) uintptr {
		if hwnd != 9 || context != 7 {
			t.Fatalf("release hwnd=%d context=%d", hwnd, context)
		}
		return 1
	}
	immGetCompositionStringCall = func(context uintptr, index uint32, pointer, size uintptr) uintptr {
		if context != 7 || index != gcsCompStr || pointer != 0 || size != 0 {
			t.Fatalf("read args context=%d index=%x pointer=%d size=%d", context, index, pointer, size)
		}
		return uintptr(uint32(0xffffffff))
	}
	var captured immCandidateForm
	immSetCandidateWindowCall = func(context uintptr, form *immCandidateForm) uintptr {
		if context != 7 {
			t.Fatalf("candidate context=%d", context)
		}
		captured = *form
		return 1
	}
	api := windowsIMMContextAPI{}
	context, err := api.Acquire(9)
	if err != nil || context != 7 {
		t.Fatalf("acquire=%d err=%v", context, err)
	}
	if value, err := api.Read(context, gcsCompStr, nil); err != nil || value != -1 {
		t.Fatalf("read=%d err=%v", value, err)
	}
	if err := api.SetCandidate(context, nativeCandidateRect{X: 2, Y: 3, Width: 4, Height: 5}, true); err != nil {
		t.Fatal(err)
	}
	if captured.Style != candidateFormExclude || captured.Current != (immPoint{X: 2, Y: 8}) || captured.Area != (immRect{Left: 2, Top: 3, Right: 6, Bottom: 8}) {
		t.Fatalf("candidate=%#v", captured)
	}
	if err := api.Release(9, context); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsIMMNativeContainsPanicsAndReturnFailures(t *testing.T) {
	oldGet, oldRelease := immGetContextCall, immReleaseContextCall
	defer func() { immGetContextCall, immReleaseContextCall = oldGet, oldRelease }()
	api := windowsIMMContextAPI{}
	immGetContextCall = func(uintptr) uintptr { panic("boom") }
	if _, err := api.Acquire(1); !errors.Is(err, errIMMCallbackPanic) {
		t.Fatalf("panic err=%v", err)
	}
	immGetContextCall = func(uintptr) uintptr { return 0 }
	if _, err := api.Acquire(1); !errors.Is(err, errIMMNativeCall) {
		t.Fatalf("zero err=%v", err)
	}
	immReleaseContextCall = func(uintptr, uintptr) uintptr { return 0 }
	if err := api.Release(1, 2); !errors.Is(err, errIMMNativeCall) {
		t.Fatalf("release err=%v", err)
	}
}

func TestWindowsIMMNativeContainsEveryAdapterPanicAndCandidateFailure(t *testing.T) {
	oldRelease, oldRead, oldCandidate := immReleaseContextCall, immGetCompositionStringCall, immSetCandidateWindowCall
	defer func() {
		immReleaseContextCall, immGetCompositionStringCall, immSetCandidateWindowCall = oldRelease, oldRead, oldCandidate
	}()
	api := windowsIMMContextAPI{}
	immReleaseContextCall = func(uintptr, uintptr) uintptr { panic("release") }
	if err := api.Release(1, 2); !errors.Is(err, errIMMCallbackPanic) {
		t.Fatalf("release panic err=%v", err)
	}
	immGetCompositionStringCall = func(uintptr, uint32, uintptr, uintptr) uintptr { panic("read") }
	if _, err := api.Read(2, gcsCompStr, nil); !errors.Is(err, errIMMCallbackPanic) {
		t.Fatalf("read panic err=%v", err)
	}
	immSetCandidateWindowCall = func(uintptr, *immCandidateForm) uintptr { panic("candidate") }
	if err := api.SetCandidate(2, nativeCandidateRect{Width: 1, Height: 1}, true); !errors.Is(err, errIMMCallbackPanic) {
		t.Fatalf("candidate panic err=%v", err)
	}
	immSetCandidateWindowCall = func(uintptr, *immCandidateForm) uintptr { return 0 }
	if err := api.SetCandidate(2, nativeCandidateRect{Width: 1, Height: 1}, true); !errors.Is(err, errIMMNativeCall) {
		t.Fatalf("candidate zero err=%v", err)
	}
	called := false
	immSetCandidateWindowCall = func(uintptr, *immCandidateForm) uintptr { called = true; return 1 }
	if err := api.SetCandidate(2, nativeCandidateRect{Width: 0, Height: 1}, true); !errors.Is(err, errIMMCandidateInvalid) || called {
		t.Fatalf("invalid candidate err=%v called=%v", err, called)
	}
}
