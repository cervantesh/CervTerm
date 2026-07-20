//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"
	"reflect"
)

var (
	errWndProcHostInvalid      = errors.New("WndProc host is invalid")
	errWndProcPriorMissing     = errors.New("prior WndProc is missing")
	errWndProcInstallConflict  = errors.New("WndProc changed during installation")
	errWndProcOwnershipLost    = errors.New("WndProc ownership was lost")
	errWndProcReleaseInstalled = errors.New("installed WndProc callback cannot be released")
	errWndProcCallbackPanic    = errors.New("WndProc callback panic")
	errWndProcHandlerLimit     = errors.New("WndProc handler limit reached")
	errWndProcHandlerDuplicate = errors.New("WndProc handler is already registered")
	errWndProcHandlerMissing   = errors.New("WndProc handler registration is stale")
	errWndProcHandlerExhausted = errors.New("WndProc handler ID exhausted")
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

const maxWndProcHandlers = 8

type wndProcHandlerID uint64

type wndProcHandlerEntry struct {
	id      wndProcHandlerID
	handler wndProcDecoder
	active  bool
	legacy  bool
}

type windowsWndProcHost struct {
	backend       wndProcBackend
	decoder       wndProcDecoder
	report        func(error)
	hwnd          uintptr
	prior         uintptr
	callback      wndProcCallback
	callbackPtr   uintptr
	installed     bool
	active        bool
	released      bool
	handlers      []*wndProcHandlerEntry
	nextHandlerID wndProcHandlerID
	dispatchDepth int
}

func (host *windowsWndProcHost) registerHandler(handler wndProcDecoder) (wndProcHandlerID, error) {
	if host == nil || handler == nil || host.released {
		return 0, errWndProcHostInvalid
	}
	if err := host.seedLegacyHandler(); err != nil {
		return 0, err
	}
	return host.appendHandler(handler, false)
}

func (host *windowsWndProcHost) unregisterHandler(id wndProcHandlerID) error {
	if host == nil || id == 0 || host.released {
		return errWndProcHandlerMissing
	}
	for _, entry := range host.handlers {
		if entry != nil && entry.id == id && entry.active {
			entry.active = false
			host.compactHandlers()
			return nil
		}
	}
	return errWndProcHandlerMissing
}

func (host *windowsWndProcHost) seedLegacyHandler() error {
	if host == nil || host.decoder == nil {
		return nil
	}
	for _, entry := range host.handlers {
		if entry != nil && entry.legacy {
			return nil
		}
	}
	_, err := host.appendHandler(host.decoder, true)
	return err
}

func (host *windowsWndProcHost) appendHandler(handler wndProcDecoder, legacy bool) (wndProcHandlerID, error) {
	handlerValue := reflect.ValueOf(handler)
	if !handlerValue.IsValid() || !handlerValue.Comparable() {
		return 0, errWndProcHostInvalid
	}
	for _, entry := range host.handlers {
		if entry != nil && entry.active && sameWndProcHandler(entry.handler, handler) {
			return 0, errWndProcHandlerDuplicate
		}
	}
	if len(host.handlers) >= maxWndProcHandlers {
		return 0, errWndProcHandlerLimit
	}
	if host.nextHandlerID == ^wndProcHandlerID(0) {
		return 0, errWndProcHandlerExhausted
	}
	host.nextHandlerID++
	if host.nextHandlerID == 0 {
		return 0, errWndProcHandlerExhausted
	}
	entry := &wndProcHandlerEntry{id: host.nextHandlerID, handler: handler, active: true, legacy: legacy}
	host.handlers = append(host.handlers, entry)
	return entry.id, nil
}

func sameWndProcHandler(left, right wndProcDecoder) bool {
	if reflect.TypeOf(left) != reflect.TypeOf(right) {
		return false
	}
	leftValue := reflect.ValueOf(left)
	return leftValue.IsValid() && leftValue.Comparable() && leftValue.Interface() == reflect.ValueOf(right).Interface()
}

func (host *windowsWndProcHost) compactHandlers() {
	kept := host.handlers[:0]
	for _, entry := range host.handlers {
		if entry != nil && entry.active {
			kept = append(kept, entry)
		}
	}
	for index := len(kept); index < len(host.handlers); index++ {
		host.handlers[index] = nil
	}
	host.handlers = kept
}

func (host *windowsWndProcHost) install() (err error) {
	if host == nil {
		return errWndProcHostInvalid
	}
	defer host.recoverOperation(&err)
	if host.installed {
		return nil
	}
	if host.backend == nil || host.hwnd == 0 || host.released {
		return errWndProcHostInvalid
	}
	if err := host.seedLegacyHandler(); err != nil {
		return err
	}
	if len(host.handlers) == 0 {
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
	host.handlers = nil
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
	if host == nil || !host.active || !host.installed || host.released || hwnd != host.hwnd || len(host.handlers) == 0 {
		return host.safeChainThrough(backend, prior, hwnd, message, wParam, lParam)
	}
	host.dispatchDepth++
	defer func() {
		host.dispatchDepth--
		if host.dispatchDepth == 0 {
			host.compactHandlers()
		}
	}()
	for _, entry := range append([]*wndProcHandlerEntry(nil), host.handlers...) {
		if entry == nil || !entry.active || entry.handler == nil {
			continue
		}
		handled, handlerErr, panicked := host.safeHandle(entry.handler, message, lParam)
		if handlerErr != nil {
			host.safeReport(handlerErr)
		}
		if host.released || !host.active || !host.installed {
			break
		}
		if handled || (panicked && entry.legacy && isIMMCompositionMessage(message)) {
			return 0
		}
	}
	return host.safeChainThrough(backend, prior, hwnd, message, wParam, lParam)
}

func (host *windowsWndProcHost) safeHandle(handler wndProcDecoder, message uint32, lParam uintptr) (handled bool, err error, panicked bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			host.safeReport(errWndProcCallbackPanic)
			handled, err, panicked = false, nil, true
		}
	}()
	handled, err = handler.handleMessage(message, lParam)
	return handled, err, false
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
