//go:build glfw && windows

package glfwgl

import (
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	errIMMNativeCall         = errors.New("IMM native call failed")
	imm32DLL                 = windows.NewLazySystemDLL("imm32.dll")
	immGetContext            = imm32DLL.NewProc("ImmGetContext")
	immReleaseContext        = imm32DLL.NewProc("ImmReleaseContext")
	immGetCompositionStringW = imm32DLL.NewProc("ImmGetCompositionStringW")
	immSetCandidateWindow    = imm32DLL.NewProc("ImmSetCandidateWindow")
	immGetContextCall        = func(hwnd uintptr) uintptr {
		result, _, _ := immGetContext.Call(hwnd)
		return result
	}
	immReleaseContextCall = func(hwnd, context uintptr) uintptr {
		result, _, _ := immReleaseContext.Call(hwnd, context)
		return result
	}
	immGetCompositionStringCall = func(context uintptr, index uint32, pointer, size uintptr) uintptr {
		result, _, _ := immGetCompositionStringW.Call(context, uintptr(index), pointer, size)
		return result
	}
	immSetCandidateWindowCall = func(context uintptr, form *immCandidateForm) uintptr {
		result, _, _ := immSetCandidateWindow.Call(context, uintptr(unsafe.Pointer(form)))
		return result
	}
)

const (
	candidateFormDefault = 0x0000
	candidateFormExclude = 0x0080
)

type immPoint struct{ X, Y int32 }
type immRect struct{ Left, Top, Right, Bottom int32 }
type immCandidateForm struct {
	Index   uint32
	Style   uint32
	Current immPoint
	Area    immRect
}

type windowsIMMContextAPI struct{}

func (windowsIMMContextAPI) Acquire(hwnd uintptr) (context uintptr, err error) {
	defer recoverIMMNative(&err)
	context = immGetContextCall(hwnd)
	if context == 0 {
		return 0, errIMMNativeCall
	}
	return context, nil
}

func (windowsIMMContextAPI) Release(hwnd, context uintptr) (err error) {
	defer recoverIMMNative(&err)
	result := immReleaseContextCall(hwnd, context)
	if result == 0 {
		return errIMMNativeCall
	}
	return nil
}

func (windowsIMMContextAPI) Read(context uintptr, index uint32, destination []byte) (result int32, err error) {
	defer recoverIMMNative(&err)
	var pointer uintptr
	if len(destination) > 0 {
		pointer = uintptr(unsafe.Pointer(&destination[0]))
	}
	value := immGetCompositionStringCall(context, index, pointer, uintptr(len(destination)))
	runtime.KeepAlive(destination)
	return int32(uint32(value)), nil
}

func (windowsIMMContextAPI) SetCandidate(context uintptr, rect nativeCandidateRect, visible bool) (err error) {
	defer recoverIMMNative(&err)
	form := immCandidateForm{Style: candidateFormDefault}
	if visible {
		x, y, width, height := int64(rect.X), int64(rect.Y), int64(rect.Width), int64(rect.Height)
		const maxInt32 = int64(1<<31 - 1)
		if x < 0 || y < 0 || width <= 0 || height <= 0 || x > maxInt32-width || y > maxInt32-height {
			return errIMMCandidateInvalid
		}
		form.Style = candidateFormExclude
		form.Current = immPoint{X: int32(x), Y: int32(y + height)}
		form.Area = immRect{Left: int32(x), Top: int32(y), Right: int32(x + width), Bottom: int32(y + height)}
	}
	result := immSetCandidateWindowCall(context, &form)
	if result == 0 {
		return errIMMNativeCall
	}
	return nil
}

func recoverIMMNative(err *error) {
	if recovered := recover(); recovered != nil {
		*err = errIMMCallbackPanic
	}
}

var _ immContextAPI = windowsIMMContextAPI{}
