//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"
)

var (
	errWndProcHostInvalid      = errors.New("WndProc host is invalid")
	errWndProcPriorMissing     = errors.New("prior WndProc is missing")
	errWndProcInstallConflict  = errors.New("WndProc changed during installation")
	errWndProcOwnershipLost    = errors.New("WndProc ownership was lost")
	errWndProcReleaseInstalled = errors.New("installed WndProc callback cannot be released")
	errWndProcCallbackPanic    = errors.New("WndProc callback panic")
)

type wndProcCallback func(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr

type wndProcBackend interface {
	GetWindowProc(hwnd uintptr) (uintptr, error)
	SetWindowProc(hwnd, procedure uintptr) (uintptr, error)
	CallWindowProc(procedure, hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr
	NewCallback(hwnd uintptr, callback wndProcCallback) (uintptr, error)
	ReleaseCallback(hwnd, callback uintptr) error
}

type wndProcDecoder interface {
	handleMessage(message uint32, lParam uintptr) (bool, error)
}

type windowsWndProcHost struct {
	backend     wndProcBackend
	decoder     wndProcDecoder
	report      func(error)
	hwnd        uintptr
	prior       uintptr
	callback    wndProcCallback
	callbackPtr uintptr
	installed   bool
	active      bool
	released    bool
}

func (host *windowsWndProcHost) install() (err error) {
	if host == nil {
		return errWndProcHostInvalid
	}
	defer host.recoverOperation(&err)
	if host.installed {
		return nil
	}
	if host.backend == nil || host.decoder == nil || host.hwnd == 0 || host.released {
		return errWndProcHostInvalid
	}
	observed, err := host.backend.GetWindowProc(host.hwnd)
	if err != nil {
		return err
	}
	if observed == 0 {
		return errWndProcPriorMissing
	}
	host.callback = host.dispatch
	callbackPtr, err := host.backend.NewCallback(host.hwnd, host.callback)
	if err != nil || callbackPtr == 0 {
		host.callback = nil
		return errors.Join(errWndProcHostInvalid, err)
	}
	host.callbackPtr = callbackPtr
	replaced, err := host.backend.SetWindowProc(host.hwnd, callbackPtr)
	if err != nil {
		return errors.Join(err, host.discardCallback())
	}
	if replaced != observed {
		rollbackTarget := replaced
		if rollbackTarget == 0 {
			rollbackTarget = observed
		}
		displaced, rollbackErr := host.backend.SetWindowProc(host.hwnd, rollbackTarget)
		if rollbackErr != nil || displaced != callbackPtr {
			host.prior, host.installed, host.active = rollbackTarget, true, false
			return errors.Join(errWndProcInstallConflict, rollbackErr, ownershipMismatch(displaced, callbackPtr))
		}
		return errors.Join(errWndProcInstallConflict, host.discardCallback())
	}
	host.prior, host.installed, host.active = replaced, true, true
	return nil
}

func (host *windowsWndProcHost) deactivate() error {
	if host == nil {
		return nil
	}
	host.active = false
	return nil
}

func (host *windowsWndProcHost) restore() (err error) {
	defer host.recoverOperation(&err)
	if host == nil || !host.installed {
		return nil
	}
	host.active = false
	current, err := host.backend.GetWindowProc(host.hwnd)
	if err != nil {
		return err
	}
	if current != host.callbackPtr {
		return errWndProcOwnershipLost
	}
	replaced, err := host.backend.SetWindowProc(host.hwnd, host.prior)
	if err != nil {
		return err
	}
	if replaced != host.callbackPtr {
		displaced, rollbackErr := host.backend.SetWindowProc(host.hwnd, replaced)
		return errors.Join(errWndProcOwnershipLost, rollbackErr, ownershipMismatch(displaced, host.prior))
	}
	host.installed = false
	return nil
}

func (host *windowsWndProcHost) release() error {
	if host == nil || host.released {
		return nil
	}
	if host.installed {
		return errWndProcReleaseInstalled
	}
	if err := host.discardCallback(); err != nil {
		return err
	}
	host.active, host.released = false, true
	host.decoder = nil
	host.prior, host.hwnd = 0, 0
	return nil
}

func (host *windowsWndProcHost) discardCallback() error {
	if host.callbackPtr == 0 {
		host.callback = nil
		return nil
	}
	if err := host.backend.ReleaseCallback(host.hwnd, host.callbackPtr); err != nil {
		return err
	}
	host.callback, host.callbackPtr = nil, 0
	return nil
}

func (host *windowsWndProcHost) dispatch(hwnd uintptr, message uint32, wParam, lParam uintptr) (result uintptr) {
	var backend wndProcBackend
	var prior uintptr
	if host != nil {
		backend, prior = host.backend, host.prior
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			host.safeReport(fmt.Errorf("%w", errWndProcCallbackPanic))
			if isIMMCompositionMessage(message) {
				result = 0
			} else {
				result = host.safeChainThrough(backend, prior, hwnd, message, wParam, lParam)
			}
		}
	}()
	if host == nil || !host.active || !host.installed || host.released || hwnd != host.hwnd || host.decoder == nil {
		return host.safeChainThrough(backend, prior, hwnd, message, wParam, lParam)
	}
	handled, err := host.decoder.handleMessage(message, lParam)
	if err != nil {
		host.safeReport(err)
	}
	if handled {
		return 0
	}
	return host.safeChainThrough(backend, prior, hwnd, message, wParam, lParam)
}

func (host *windowsWndProcHost) safeChainThrough(backend wndProcBackend, prior, hwnd uintptr, message uint32, wParam, lParam uintptr) (result uintptr) {
	defer func() {
		if recovered := recover(); recovered != nil {
			host.safeReport(errWndProcCallbackPanic)
			result = 0
		}
	}()
	if backend == nil || prior == 0 {
		return 0
	}
	return backend.CallWindowProc(prior, hwnd, message, wParam, lParam)
}

func (host *windowsWndProcHost) safeReport(err error) {
	if host == nil || err == nil || host.report == nil {
		return
	}
	defer func() { _ = recover() }()
	host.report(err)
}

func (host *windowsWndProcHost) recoverOperation(err *error) {
	if recovered := recover(); recovered != nil {
		*err = errors.Join(*err, errWndProcCallbackPanic)
	}
}

func ownershipMismatch(got, want uintptr) error {
	if got == want {
		return nil
	}
	return errWndProcOwnershipLost
}

func isIMMCompositionMessage(message uint32) bool {
	return message == wmIMEStartComposition || message == wmIMEComposition || message == wmIMEEndComposition
}

var _ wndProcDecoder = (*immDecoder)(nil)
