//go:build glfw && windows

package glfwgl

import (
	"errors"
	"sync"

	"cervterm/internal/accessibility"
)

type projectionAccessibilityPreparation struct {
	Document   accessibility.Document
	ScreenX    float64
	ScreenY    float64
	Bounds     accessibility.Rect
	HWND       uintptr
	API        uiaNativeProviderAPI
	Dispatcher *uiaProviderDispatcher
	Host       *windowsWndProcHost
	Report     func(error)
	EventSink  func(accessibility.Document) error
}

type projectionAccessibility struct {
	publication *uiaPublication
	root        *uiaRootProvider
	native      *nativeUIAProvider
	dispatcher  *uiaProviderDispatcher
	token       uintptr
	host        *windowsWndProcHost
	handlerID   wndProcHandlerID
	closeOnce   sync.Once
	closeErr    error
	eventSink   func(accessibility.Document) error
}

func prepareDormantProjectionAccessibility(spec projectionAccessibilityPreparation, before *compositionBeforeUnbind) (*projectionAccessibility, error) {
	if before == nil || spec.HWND == 0 || spec.API == nil {
		return nil, errProjectionAccessibilityInvalid
	}
	publication := &uiaPublication{}
	if err := publication.PublishScreen(spec.Document, spec.ScreenX, spec.ScreenY, spec.Bounds); err != nil {
		return nil, err
	}
	root, err := newDormantUIARootProvider(publication, spec.API, spec.HWND)
	if err != nil {
		return nil, err
	}
	lifecycle := &projectionAccessibility{publication: publication, root: root, eventSink: spec.EventSink}
	fail := func(cause error) (*projectionAccessibility, error) {
		return lifecycle, cause
	}
	native, err := newNativeUIAProvider(root)
	if err != nil {
		return fail(err)
	}
	lifecycle.native = native
	dispatcher := spec.Dispatcher
	if dispatcher == nil {
		dispatcher = processUIAProviderDispatcher
	}
	token, err := dispatcher.Register(root)
	if err != nil {
		return fail(err)
	}
	lifecycle.dispatcher, lifecycle.token = dispatcher, token
	host := spec.Host
	reusedHost := before.wndProcHost != nil
	if before.wndProcHost != nil {
		if host != nil && host != before.wndProcHost {
			return fail(errWndProcInstallConflict)
		}
		host = before.wndProcHost
		if !host.installed || !host.active {
			return fail(errWndProcHostInvalid)
		}
	} else {
		if host == nil {
			host = &windowsWndProcHost{backend: nativeWndProcBackend{}, hwnd: spec.HWND, report: spec.Report}
		}
		if err := before.attachWndProcHost(host); err != nil {
			return fail(err)
		}
	}
	lifecycle.host = host
	handlerID, err := host.registerHandler(&uiaWndProcHandler{provider: root})
	if err != nil {
		return fail(err)
	}
	lifecycle.handlerID = handlerID
	if !reusedHost {
		if err := host.install(); err != nil {
			return fail(err)
		}
	}
	return lifecycle, nil
}

func (lifecycle *projectionAccessibility) Publish(document accessibility.Document, screenX, screenY float64, bounds accessibility.Rect) error {
	if lifecycle == nil || lifecycle.publication == nil || lifecycle.root == nil || !lifecycle.root.available() {
		return errProjectionAccessibilityInvalid
	}
	if err := lifecycle.publication.PublishScreen(document, screenX, screenY, bounds); err != nil {
		return err
	}
	if lifecycle.eventSink != nil {
		return lifecycle.eventSink(document)
	}
	return nil
}

type projectionAccessibilitySnapshot struct {
	Document accessibility.Document
	ScreenX  float64
	ScreenY  float64
	Bounds   accessibility.Rect
}

func (lifecycle *projectionAccessibility) CaptureAndPublish(capture func() (projectionAccessibilitySnapshot, error)) error {
	if capture == nil {
		return errProjectionAccessibilityInvalid
	}
	snapshot, err := capture()
	if err != nil {
		return err
	}
	return lifecycle.Publish(snapshot.Document, snapshot.ScreenX, snapshot.ScreenY, snapshot.Bounds)
}

func (lifecycle *projectionAccessibility) Close() error {
	if lifecycle == nil {
		return nil
	}
	lifecycle.closeOnce.Do(func() {
		if lifecycle.native != nil {
			lifecycle.native.Close()
			lifecycle.native = nil
		} else if lifecycle.root != nil {
			lifecycle.root.Disconnect()
		}
		if lifecycle.host != nil && lifecycle.handlerID != 0 {
			lifecycle.closeErr = errors.Join(lifecycle.closeErr, lifecycle.host.unregisterHandler(lifecycle.handlerID))
			lifecycle.handlerID = 0
		}
		if lifecycle.dispatcher != nil && lifecycle.token != 0 {
			lifecycle.closeErr = errors.Join(lifecycle.closeErr, lifecycle.dispatcher.Unregister(lifecycle.token))
			lifecycle.token = 0
		}
		if lifecycle.root != nil {
			lifecycle.root.Release()
			lifecycle.root = nil
		}
	})
	return lifecycle.closeErr
}
