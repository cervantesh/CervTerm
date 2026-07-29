package glfwgl

import "errors"

// initialWindowSurface is the minimum native-window capability needed by the
// startup lifecycle. Keeping orchestration independent of GLFW makes reveal
// ordering, focus, and failure behavior deterministic in unit tests.
type initialWindowSurface interface {
	Show()
	Hide()
	MakeContextCurrent()
	Focus()
}

// initialWindowReveal is a fakeable native mask capability. Supported adapters
// must conceal the native window before Show and restore it only after the
// visible frame has reached the compositor. It does not own window opacity.
type initialWindowReveal interface {
	Conceal() (supported bool, err error)
	Restore() error
}

type initialWindowCompositor interface {
	Flush() error
}

type unsupportedInitialWindowReveal struct{}

func (unsupportedInitialWindowReveal) Conceal() (bool, error) { return false, nil }
func (unsupportedInitialWindowReveal) Restore() error         { return nil }

type noopInitialWindowCompositor struct{}

func (noopInitialWindowCompositor) Flush() error { return nil }

// createWindowHidden scopes the process-global native visibility and
// focus-on-show hints to one creation attempt. Restoring both defaults keeps
// child and restored-window creation behavior unchanged.
func createWindowHidden[T any](setInitialHints func(bool), create func() (T, error)) (T, error) {
	setInitialHints(false)
	defer setInitialHints(true)
	return create()
}

// runInitialWindowLifecycle prepares and presents one valid hidden frame, then
// performs a natively concealed visible presentation before reveal and focus.
// The caller retains cleanup ownership of the returned window. Every failure
// after Show synchronously attempts native Hide and never focuses the window.
func runInitialWindowLifecycle[T initialWindowSurface](
	createHidden func() (T, error),
	prepare func(T) error,
	firstPresent func(T) error,
	visiblePresent func(T) error,
	revealFor func(T) initialWindowReveal,
	compositor initialWindowCompositor,
) (T, error) {
	window, err := createHidden()
	if err != nil {
		var zero T
		return zero, err
	}
	if err := prepare(window); err != nil {
		return window, err
	}
	if err := firstPresent(window); err != nil {
		return window, err
	}

	reveal := revealFor(window)
	masked, err := reveal.Conceal()
	if err != nil {
		return window, err
	}
	window.Show()
	window.MakeContextCurrent()
	abortVisible := func(failure error, rollback bool) (T, error) {
		var rollbackErr error
		if rollback && masked {
			_, rollbackErr = reveal.Conceal()
		}
		// Hide is mandatory even when rollback failed; GLFW owns the native hide
		// operation and this is the final guarantee against an observable bad frame.
		window.Hide()
		return window, errors.Join(failure, rollbackErr)
	}
	if err := visiblePresent(window); err != nil {
		return abortVisible(err, false)
	}
	if err := compositor.Flush(); err != nil {
		return abortVisible(err, false)
	}
	if masked {
		if err := reveal.Restore(); err != nil {
			return abortVisible(err, true)
		}
	}
	window.Focus()
	return window, nil
}
