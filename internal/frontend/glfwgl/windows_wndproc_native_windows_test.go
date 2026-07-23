//go:build glfw && windows

package glfwgl

import (
	"errors"
	"reflect"
	"testing"
)

func preserveWndProcNativeCalls() func() {
	setLast, getLast := wndProcSetLastErrorCall, wndProcGetLastErrorCall
	get, set := wndProcGetCall, wndProcSetCall
	chain, callback := wndProcChainCall, wndProcNewCallbackCall
	wndProcRegistryMu.Lock()
	registry, shared := wndProcRegistry, wndProcSharedCallback
	wndProcRegistry, wndProcSharedCallback = make(map[uintptr]wndProcCallback), 0
	wndProcRegistryMu.Unlock()
	return func() {
		wndProcSetLastErrorCall, wndProcGetLastErrorCall = setLast, getLast
		wndProcGetCall, wndProcSetCall = get, set
		wndProcChainCall, wndProcNewCallbackCall = chain, callback
		wndProcRegistryMu.Lock()
		wndProcRegistry, wndProcSharedCallback = registry, shared
		wndProcRegistryMu.Unlock()
	}
}

func TestNativeWndProcGetSetLastErrorSemantics(t *testing.T) {
	defer preserveWndProcNativeCalls()()
	backend := nativeWndProcBackend{}
	var cleared []uintptr
	lastError := uintptr(99)
	wndProcSetLastErrorCall = func(value uintptr) { cleared = append(cleared, value); lastError = value }
	wndProcGetLastErrorCall = func() uintptr { return lastError }
	callError := uintptr(0)
	getValue := uintptr(0)
	wndProcGetCall = func(hwnd uintptr) uintptr {
		if hwnd != 5 {
			t.Fatalf("get hwnd=%d", hwnd)
		}
		lastError = callError
		return getValue
	}
	setValue := uintptr(0)
	wndProcSetCall = func(hwnd, procedure uintptr) uintptr {
		if hwnd != 5 || procedure != 100 {
			t.Fatalf("set hwnd=%d proc=%d", hwnd, procedure)
		}
		lastError = callError
		return setValue
	}
	if value, err := backend.GetWindowProc(5); err != nil || value != 0 {
		t.Fatalf("zero-success get=%d err=%v", value, err)
	}
	callError = 5
	if _, err := backend.GetWindowProc(5); !errors.Is(err, errWndProcNativeCall) {
		t.Fatalf("zero-failure get err=%v", err)
	}
	getValue = 9
	if value, err := backend.GetWindowProc(5); err != nil || value != 9 {
		t.Fatalf("nonzero stale-error get=%d err=%v", value, err)
	}
	callError, setValue = 0, 0
	if value, err := backend.SetWindowProc(5, 100); err != nil || value != 0 {
		t.Fatalf("zero-success set=%d err=%v", value, err)
	}
	callError = 5
	if _, err := backend.SetWindowProc(5, 100); !errors.Is(err, errWndProcNativeCall) {
		t.Fatalf("zero-failure set err=%v", err)
	}
	setValue = 9
	if value, err := backend.SetWindowProc(5, 100); err != nil || value != 9 {
		t.Fatalf("nonzero stale-error set=%d err=%v", value, err)
	}
	if want := []uintptr{0, 0, 0, 0, 0, 0}; !reflect.DeepEqual(cleared, want) {
		t.Fatalf("last-error clears=%v want=%v", cleared, want)
	}
}

func TestNativeWndProcCallbackAndChainArguments(t *testing.T) {
	defer preserveWndProcNativeCalls()()
	backend := nativeWndProcBackend{}
	var captured wndProcCallback
	wndProcNewCallbackCall = func(callback wndProcCallback) uintptr { captured = callback; return 100 }
	callback := wndProcCallback(func(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
		return hwnd + uintptr(message) + wParam + lParam
	})
	pointer, err := backend.NewCallback(5, callback)
	if err != nil || pointer != 100 || captured == nil || captured(5, 2, 3, 4) != 14 {
		t.Fatalf("callback pointer=%d err=%v captured=%v", pointer, err, captured != nil)
	}
	if err := backend.ReleaseCallback(5, pointer); err != nil || captured(5, 2, 3, 4) != 0 {
		t.Fatalf("callback release err=%v", err)
	}
	var args [5]uintptr
	wndProcChainCall = func(procedure, hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
		args = [5]uintptr{procedure, hwnd, uintptr(message), wParam, lParam}
		return 77
	}
	if result := backend.CallWindowProc(9, 5, 42, 3, 4); result != 77 || args != ([5]uintptr{9, 5, 42, 3, 4}) {
		t.Fatalf("chain result=%d args=%v", result, args)
	}
	if gwlpWndProc != -4 {
		t.Fatalf("GWLP_WNDPROC=%d", gwlpWndProc)
	}
	if _, err := glfwWindowHWND(nil); !errors.Is(err, errWndProcHostInvalid) {
		t.Fatalf("nil GLFW window err=%v", err)
	}
}

func TestNativeWndProcContainsAdapterPanics(t *testing.T) {
	defer preserveWndProcNativeCalls()()
	backend := nativeWndProcBackend{}
	wndProcSetLastErrorCall = func(uintptr) { panic("last error") }
	if _, err := backend.GetWindowProc(5); !errors.Is(err, errWndProcCallbackPanic) {
		t.Fatalf("get panic err=%v", err)
	}
	wndProcSetLastErrorCall = func(uintptr) {}
	wndProcGetCall = func(uintptr) uintptr { panic("get") }
	if _, err := backend.GetWindowProc(5); !errors.Is(err, errWndProcCallbackPanic) {
		t.Fatalf("get call panic err=%v", err)
	}
	wndProcSetCall = func(uintptr, uintptr) uintptr { panic("set") }
	if _, err := backend.SetWindowProc(5, 100); !errors.Is(err, errWndProcCallbackPanic) {
		t.Fatalf("set panic err=%v", err)
	}
	wndProcNewCallbackCall = func(wndProcCallback) uintptr { panic("callback") }
	if _, err := backend.NewCallback(5, func(uintptr, uint32, uintptr, uintptr) uintptr { return 0 }); !errors.Is(err, errWndProcCallbackPanic) {
		t.Fatalf("callback panic err=%v", err)
	}
}

func TestNativeWndProcUsesOneBoundedSharedCallback(t *testing.T) {
	defer preserveWndProcNativeCalls()()
	allocations := 0
	var nativeCallback wndProcCallback
	wndProcNewCallbackCall = func(callback wndProcCallback) uintptr {
		allocations++
		nativeCallback = callback
		return 100
	}
	backend := nativeWndProcBackend{}
	first, err := backend.NewCallback(5, func(uintptr, uint32, uintptr, uintptr) uintptr { return 55 })
	if err != nil {
		t.Fatal(err)
	}
	second, err := backend.NewCallback(6, func(uintptr, uint32, uintptr, uintptr) uintptr { return 66 })
	if err != nil {
		t.Fatal(err)
	}
	if first != 100 || second != first || allocations != 1 || nativeCallback(5, 0, 0, 0) != 55 || nativeCallback(6, 0, 0, 0) != 66 {
		t.Fatalf("first=%d second=%d allocations=%d", first, second, allocations)
	}
	if err := backend.ReleaseCallback(5, first); err != nil {
		t.Fatal(err)
	}
	if nativeCallback(5, 0, 0, 0) != 0 || nativeCallback(6, 0, 0, 0) != 66 {
		t.Fatal("shared callback registry release was not HWND-local")
	}
	if err := backend.ReleaseCallback(6, second); err != nil {
		t.Fatal(err)
	}
}
