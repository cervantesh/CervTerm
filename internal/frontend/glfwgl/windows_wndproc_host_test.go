//go:build glfw

package glfwgl

import (
	"errors"
	"reflect"
	"testing"
)

type fakeWndProcBackend struct {
	current     uintptr
	callbackPtr uintptr
	callback    wndProcCallback
	getErr      error
	newErr      error
	releaseErr  error
	registered  uintptr
	setReturns  []uintptr
	setErrors   []error
	setTargets  []uintptr
	chainCalls  [][5]uintptr
	chainResult uintptr
	panicChain  bool
}

func (backend *fakeWndProcBackend) GetWindowProc(uintptr) (uintptr, error) {
	return backend.current, backend.getErr
}
func (backend *fakeWndProcBackend) SetWindowProc(_ uintptr, procedure uintptr) (uintptr, error) {
	backend.setTargets = append(backend.setTargets, procedure)
	previous := backend.current
	backend.current = procedure
	if len(backend.setReturns) > 0 {
		previous = backend.setReturns[0]
		backend.setReturns = backend.setReturns[1:]
	}
	if len(backend.setErrors) > 0 {
		err := backend.setErrors[0]
		backend.setErrors = backend.setErrors[1:]
		if err != nil {
			backend.current = previous
			return 0, err
		}
	}
	return previous, nil
}
func (backend *fakeWndProcBackend) CallWindowProc(procedure, hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	if backend.panicChain {
		panic("chain")
	}
	backend.chainCalls = append(backend.chainCalls, [5]uintptr{procedure, hwnd, uintptr(message), wParam, lParam})
	return backend.chainResult
}
func (backend *fakeWndProcBackend) NewCallback(hwnd uintptr, callback wndProcCallback) (uintptr, error) {
	backend.callback = callback
	if backend.newErr != nil {
		return 0, backend.newErr
	}
	if backend.callbackPtr == 0 {
		backend.callbackPtr = 100
	}
	backend.registered = hwnd
	return backend.callbackPtr, nil
}
func (backend *fakeWndProcBackend) ReleaseCallback(hwnd, callback uintptr) error {
	if backend.releaseErr != nil {
		return backend.releaseErr
	}
	if hwnd != backend.registered || callback != backend.callbackPtr {
		return errWndProcHostInvalid
	}
	backend.registered, backend.callback = 0, nil
	return nil
}

type fakeWndProcDecoder struct {
	handled bool
	err     error
	panic   bool
	hook    func()
	calls   [][2]uintptr
}

func (decoder *fakeWndProcDecoder) handleMessage(message uint32, lParam uintptr) (bool, error) {
	if decoder.panic {
		panic("decode")
	}
	if decoder.hook != nil {
		decoder.hook()
	}
	decoder.calls = append(decoder.calls, [2]uintptr{uintptr(message), lParam})
	return decoder.handled, decoder.err
}

type nonComparableWndProcDecoder struct{ values []int }

func (nonComparableWndProcDecoder) handleMessage(uint32, uintptr) (bool, error) { return false, nil }

func TestDormantWndProcHostInstallDispatchRestoreRelease(t *testing.T) {
	backend := &fakeWndProcBackend{current: 9, chainResult: 77}
	decoder := &fakeWndProcDecoder{}
	var reported []error
	host := &windowsWndProcHost{backend: backend, decoder: decoder, hwnd: 5, report: func(err error) { reported = append(reported, err) }}
	if err := host.install(); err != nil {
		t.Fatal(err)
	}
	if !host.installed || !host.active || host.prior != 9 || backend.current != 100 || backend.callback == nil {
		t.Fatalf("host=%#v current=%d callback=%v", host, backend.current, backend.callback != nil)
	}
	if got := backend.callback(5, 42, 3, 4); got != 77 {
		t.Fatalf("unhandled result=%d", got)
	}
	if want := [][5]uintptr{{9, 5, 42, 3, 4}}; !reflect.DeepEqual(backend.chainCalls, want) {
		t.Fatalf("chain=%v want=%v", backend.chainCalls, want)
	}
	decoder.handled, decoder.err = true, errors.New("decode")
	if got := backend.callback(5, wmIMEComposition, 6, 7); got != 0 || len(backend.chainCalls) != 1 || len(reported) != 1 {
		t.Fatalf("handled result=%d chain=%v reported=%v", got, backend.chainCalls, reported)
	}
	if err := host.deactivate(); err != nil {
		t.Fatal(err)
	}
	if got := backend.callback(5, wmIMEComposition, 8, 9); got != 77 || len(decoder.calls) != 2 {
		t.Fatalf("inactive result=%d decoder=%v", got, decoder.calls)
	}
	if err := host.restore(); err != nil || backend.current != 9 || host.installed {
		t.Fatalf("restore err=%v current=%d host=%#v", err, backend.current, host)
	}
	if err := host.release(); err != nil || !host.released || host.callback != nil || host.decoder != nil || host.hwnd != 0 {
		t.Fatalf("release err=%v host=%#v", err, host)
	}
	if err := host.restore(); err != nil || host.release() != nil {
		t.Fatalf("idempotent restore/release err=%v", err)
	}
}

func TestDormantWndProcHostInstallConflictRollsBack(t *testing.T) {
	backend := &fakeWndProcBackend{current: 9, setReturns: []uintptr{8, 100}}
	host := &windowsWndProcHost{backend: backend, decoder: &fakeWndProcDecoder{}, hwnd: 5}
	if err := host.install(); !errors.Is(err, errWndProcInstallConflict) {
		t.Fatalf("install conflict err=%v", err)
	}
	if backend.current != 8 || host.installed || host.callback != nil || !reflect.DeepEqual(backend.setTargets, []uintptr{100, 8}) {
		t.Fatalf("rollback host=%#v current=%d targets=%v", host, backend.current, backend.setTargets)
	}
}

func TestDormantWndProcHostRetainsCallbackWhenRollbackFails(t *testing.T) {
	rollbackErr := errors.New("rollback")
	backend := &fakeWndProcBackend{current: 9, setReturns: []uintptr{8, 100}, setErrors: []error{nil, rollbackErr}}
	host := &windowsWndProcHost{backend: backend, decoder: &fakeWndProcDecoder{}, hwnd: 5}
	if err := host.install(); !errors.Is(err, rollbackErr) || !errors.Is(err, errWndProcInstallConflict) {
		t.Fatalf("rollback failure err=%v", err)
	}
	if !host.installed || host.callback == nil || host.callbackPtr == 0 || host.active {
		t.Fatalf("unsafe callback ownership after rollback failure: %#v", host)
	}
	if err := host.release(); !errors.Is(err, errWndProcReleaseInstalled) || host.callback == nil {
		t.Fatalf("release installed err=%v host=%#v", err, host)
	}
}

func TestDormantWndProcHostDoesNotOverwriteLaterSubclass(t *testing.T) {
	backend := &fakeWndProcBackend{current: 9}
	host := &windowsWndProcHost{backend: backend, decoder: &fakeWndProcDecoder{}, hwnd: 5}
	if err := host.install(); err != nil {
		t.Fatal(err)
	}
	backend.current = 222
	if err := host.restore(); !errors.Is(err, errWndProcOwnershipLost) || backend.current != 222 || !host.installed || host.active {
		t.Fatalf("ownership restore err=%v current=%d host=%#v", err, backend.current, host)
	}
	if err := host.release(); !errors.Is(err, errWndProcReleaseInstalled) {
		t.Fatalf("lost ownership release err=%v", err)
	}
}

func TestDormantWndProcHostContainsDecoderChainAndReportPanics(t *testing.T) {
	backend := &fakeWndProcBackend{current: 9, chainResult: 77}
	decoder := &fakeWndProcDecoder{panic: true}
	reports := 0
	host := &windowsWndProcHost{backend: backend, decoder: decoder, hwnd: 5, report: func(error) { reports++; panic("report") }}
	if err := host.install(); err != nil {
		t.Fatal(err)
	}
	if got := backend.callback(5, wmIMEComposition, 0, 0); got != 0 || reports != 1 || len(backend.chainCalls) != 0 {
		t.Fatalf("IME panic result=%d reports=%d chain=%v", got, reports, backend.chainCalls)
	}
	if got := backend.callback(5, 42, 0, 0); got != 77 || reports != 2 || len(backend.chainCalls) != 1 {
		t.Fatalf("other panic result=%d reports=%d chain=%v", got, reports, backend.chainCalls)
	}
	backend.panicChain = true
	if got := backend.callback(99, 42, 0, 0); got != 0 || reports != 3 {
		t.Fatalf("chain panic result=%d reports=%d", got, reports)
	}
}

func TestCompositionBeforeUnbindContinuesAfterPanicAndAttachesHost(t *testing.T) {
	var order []string
	backend := &fakeWndProcBackend{current: 9}
	host := &windowsWndProcHost{backend: backend, decoder: &fakeWndProcDecoder{}, hwnd: 5}
	if err := host.install(); err != nil {
		t.Fatal(err)
	}
	coordinator := &compositionBeforeUnbind{
		cancel:     func() error { order = append(order, "cancel"); panic("cancel") },
		deactivate: func() error { order = append(order, "deactivate"); return nil },
	}
	if err := coordinator.attachWndProcHost(host); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.close(); !errors.Is(err, errCompositionCleanupPanic) {
		t.Fatalf("close err=%v", err)
	}
	if host.installed || !host.released || backend.current != 9 || !reflect.DeepEqual(order, []string{"cancel", "deactivate"}) {
		t.Fatalf("teardown order=%v host=%#v current=%d", order, host, backend.current)
	}
}

func TestDormantWndProcHostRetainsOwnershipOnUnexpectedRollbackDisplacement(t *testing.T) {
	backend := &fakeWndProcBackend{current: 9, setReturns: []uintptr{8, 77}}
	host := &windowsWndProcHost{backend: backend, decoder: &fakeWndProcDecoder{}, hwnd: 5}
	if err := host.install(); !errors.Is(err, errWndProcInstallConflict) || !errors.Is(err, errWndProcOwnershipLost) {
		t.Fatalf("unexpected rollback err=%v", err)
	}
	if !host.installed || host.callback == nil || host.callbackPtr == 0 {
		t.Fatalf("callback ownership discarded after unproven rollback: %#v", host)
	}
}

func TestDormantWndProcHostReentrantReleaseStillChainsCapturedPrior(t *testing.T) {
	backend := &fakeWndProcBackend{current: 9, chainResult: 77}
	decoder := &fakeWndProcDecoder{handled: true}
	host := &windowsWndProcHost{backend: backend, decoder: decoder, hwnd: 5}
	if err := host.install(); err != nil {
		t.Fatal(err)
	}
	callback := backend.callback
	decoder.hook = func() {
		if err := host.restore(); err != nil {
			t.Fatalf("reentrant restore: %v", err)
		}
		if err := host.release(); err != nil {
			t.Fatalf("reentrant release: %v", err)
		}
	}
	if got := callback(5, 42, 3, 4); got != 77 {
		t.Fatalf("reentrant chain result=%d", got)
	}
	if want := [][5]uintptr{{9, 5, 42, 3, 4}}; !reflect.DeepEqual(backend.chainCalls, want) {
		t.Fatalf("reentrant chain=%v want=%v", backend.chainCalls, want)
	}
}

func TestCompositionCleanupPanicAtEveryStageStillDestroysProjection(t *testing.T) {
	for _, panicStage := range []string{"cancel", "deactivate", "restore", "release"} {
		t.Run(panicStage, func(t *testing.T) {
			var order []string
			step := func(name string) func() error {
				return func() error {
					order = append(order, name)
					if name == panicStage {
						panic(name)
					}
					return nil
				}
			}
			bundle := &nativeProjectionBundle{
				host:         &fakeNativeWindow{id: "panic", log: &order},
				beforeUnbind: &compositionBeforeUnbind{cancel: step("cancel"), deactivate: step("deactivate"), restore: step("restore"), release: step("release")},
				unbind:       step("unbind"),
				resources:    []projectionResource{projectionResourceFunc(step("resource"))},
			}
			if err := bundle.close(); !errors.Is(err, errCompositionCleanupPanic) {
				t.Fatalf("close err=%v", err)
			}
			want := []string{"cancel", "deactivate", "restore", "release", "unbind", "resource", "destroy:panic"}
			if !reflect.DeepEqual(order, want) {
				t.Fatalf("order=%v want=%v", order, want)
			}
		})
	}
}

func TestDormantWndProcHostRestoreFailureCanRetrySafely(t *testing.T) {
	restoreErr := errors.New("restore")
	backend := &fakeWndProcBackend{current: 9}
	host := &windowsWndProcHost{backend: backend, decoder: &fakeWndProcDecoder{}, hwnd: 5}
	if err := host.install(); err != nil {
		t.Fatal(err)
	}
	backend.setErrors = []error{restoreErr}
	if err := host.restore(); !errors.Is(err, restoreErr) || !host.installed || host.active || backend.current != host.callbackPtr {
		t.Fatalf("failed restore err=%v host=%#v current=%d", err, host, backend.current)
	}
	if err := host.restore(); err != nil || host.installed || backend.current != 9 {
		t.Fatalf("retry restore err=%v host=%#v current=%d", err, host, backend.current)
	}
	if err := host.release(); err != nil {
		t.Fatal(err)
	}
}

func TestWndProcHostDeterministicHandlersConsumeOnce(t *testing.T) {
	backend := &fakeWndProcBackend{current: 9, chainResult: 77}
	var order []string
	legacy := &fakeWndProcDecoder{hook: func() { order = append(order, "legacy") }}
	second := &fakeWndProcDecoder{handled: true, hook: func() { order = append(order, "second") }}
	third := &fakeWndProcDecoder{hook: func() { order = append(order, "third") }}
	host := &windowsWndProcHost{backend: backend, decoder: legacy, hwnd: 5}
	secondID, err := host.registerHandler(second)
	if err != nil || secondID == 0 {
		t.Fatalf("register second id=%d err=%v", secondID, err)
	}
	if _, err := host.registerHandler(third); err != nil {
		t.Fatal(err)
	}
	if _, err := host.registerHandler(second); !errors.Is(err, errWndProcHandlerDuplicate) {
		t.Fatalf("duplicate err=%v", err)
	}
	if err := host.install(); err != nil {
		t.Fatal(err)
	}
	if got := backend.callback(5, 42, 0, 0); got != 0 || !reflect.DeepEqual(order, []string{"legacy", "second"}) || len(backend.chainCalls) != 0 {
		t.Fatalf("consumed got=%d order=%v chain=%v", got, order, backend.chainCalls)
	}
	order = nil
	second.handled = false
	if got := backend.callback(5, 43, 0, 0); got != 77 || !reflect.DeepEqual(order, []string{"legacy", "second", "third"}) || len(backend.chainCalls) != 1 {
		t.Fatalf("unhandled got=%d order=%v chain=%v", got, order, backend.chainCalls)
	}
	if err := host.unregisterHandler(secondID); err != nil {
		t.Fatal(err)
	}
	if err := host.unregisterHandler(secondID); !errors.Is(err, errWndProcHandlerMissing) {
		t.Fatalf("stale removal err=%v", err)
	}
}

func TestWndProcHostHandlerBoundAndExhaustion(t *testing.T) {
	host := &windowsWndProcHost{backend: &fakeWndProcBackend{current: 9}, decoder: &fakeWndProcDecoder{}, hwnd: 5}
	for index := 0; index < maxWndProcHandlers-1; index++ {
		if _, err := host.registerHandler(&fakeWndProcDecoder{}); err != nil {
			t.Fatalf("register %d: %v", index, err)
		}
	}
	if _, err := host.registerHandler(&fakeWndProcDecoder{}); !errors.Is(err, errWndProcHandlerLimit) {
		t.Fatalf("limit err=%v", err)
	}
	exhausted := &windowsWndProcHost{backend: &fakeWndProcBackend{current: 9}, hwnd: 5, nextHandlerID: ^wndProcHandlerID(0)}
	if _, err := exhausted.registerHandler(&fakeWndProcDecoder{}); !errors.Is(err, errWndProcHandlerExhausted) {
		t.Fatalf("exhaustion err=%v", err)
	}
}

func TestWndProcHostReentrantUnregisterSkipsPendingHandler(t *testing.T) {
	backend := &fakeWndProcBackend{current: 9, chainResult: 77}
	host := &windowsWndProcHost{backend: backend, hwnd: 5}
	var secondID wndProcHandlerID
	first := &fakeWndProcDecoder{}
	second := &fakeWndProcDecoder{}
	replacement := &fakeWndProcDecoder{}
	first.hook = func() {
		first.hook = nil
		if err := host.unregisterHandler(secondID); err != nil {
			t.Fatalf("reentrant unregister: %v", err)
		}
		if _, err := host.registerHandler(replacement); err != nil {
			t.Fatalf("reentrant replacement: %v", err)
		}
	}
	if _, err := host.registerHandler(first); err != nil {
		t.Fatal(err)
	}
	var err error
	secondID, err = host.registerHandler(second)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.install(); err != nil {
		t.Fatal(err)
	}
	if got := backend.callback(5, 42, 0, 0); got != 77 || len(first.calls) != 1 || len(second.calls) != 0 || len(replacement.calls) != 0 || len(host.handlers) != 2 {
		t.Fatalf("result=%d first=%d second=%d replacement=%d handlers=%d", got, len(first.calls), len(second.calls), len(replacement.calls), len(host.handlers))
	}
	if got := backend.callback(5, 43, 0, 0); got != 77 || len(replacement.calls) != 1 {
		t.Fatalf("next dispatch result=%d replacement=%d", got, len(replacement.calls))
	}
}

func TestWndProcHostContainsIndividualHandlerPanicAndError(t *testing.T) {
	backend := &fakeWndProcBackend{current: 9, chainResult: 77}
	reports := 0
	host := &windowsWndProcHost{backend: backend, hwnd: 5, report: func(error) { reports++ }}
	panicking := &fakeWndProcDecoder{panic: true}
	failing := &fakeWndProcDecoder{err: errors.New("handler")}
	last := &fakeWndProcDecoder{}
	for _, handler := range []wndProcDecoder{panicking, failing, last} {
		if _, err := host.registerHandler(handler); err != nil {
			t.Fatal(err)
		}
	}
	if err := host.install(); err != nil {
		t.Fatal(err)
	}
	if got := backend.callback(5, 42, 0, 0); got != 77 || reports != 2 || len(last.calls) != 1 || len(backend.chainCalls) != 1 {
		t.Fatalf("got=%d reports=%d last=%d chain=%d", got, reports, len(last.calls), len(backend.chainCalls))
	}
}

func TestWndProcHostRejectsNonComparableHandlerIdentity(t *testing.T) {
	host := &windowsWndProcHost{backend: &fakeWndProcBackend{current: 9}, hwnd: 5}
	if _, err := host.registerHandler(nonComparableWndProcDecoder{values: []int{1}}); !errors.Is(err, errWndProcHostInvalid) {
		t.Fatalf("non-comparable handler err=%v", err)
	}
}
