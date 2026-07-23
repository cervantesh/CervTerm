//go:build glfw

package glfwgl

import "sync"

type uiaProviderDispatcher struct {
	mu        sync.RWMutex
	next      uintptr
	providers map[uintptr]*uiaRootProvider
}

func newUIAProviderDispatcher() *uiaProviderDispatcher {
	return &uiaProviderDispatcher{providers: make(map[uintptr]*uiaRootProvider, maxUIAProviders)}
}

var processUIAProviderDispatcher = newUIAProviderDispatcher()

func (dispatcher *uiaProviderDispatcher) Register(provider *uiaRootProvider) (uintptr, error) {
	if dispatcher == nil || provider == nil || !provider.available() {
		return 0, errUIAProviderInvalid
	}
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	for _, existing := range dispatcher.providers {
		if existing == provider {
			return 0, errUIAProviderDuplicate
		}
	}
	if len(dispatcher.providers) == maxUIAProviders {
		return 0, errUIAProviderLimit
	}
	if dispatcher.next == ^uintptr(0) {
		return 0, errUIAProviderLimit
	}
	dispatcher.next++
	if dispatcher.next == 0 {
		return 0, errUIAProviderLimit
	}
	dispatcher.providers[dispatcher.next] = provider
	provider.token.Store(dispatcher.next)
	provider.AddRef()
	return dispatcher.next, nil
}

func (dispatcher *uiaProviderDispatcher) Unregister(token uintptr) error {
	if dispatcher == nil || token == 0 {
		return errUIAProviderMissing
	}
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	provider, ok := dispatcher.providers[token]
	if !ok {
		return errUIAProviderMissing
	}
	delete(dispatcher.providers, token)
	provider.token.Store(0)
	provider.Release()
	return nil
}

func (dispatcher *uiaProviderDispatcher) Provider(token uintptr) (*uiaRootProvider, bool) {
	if dispatcher == nil {
		return nil, false
	}
	dispatcher.mu.RLock()
	defer dispatcher.mu.RUnlock()
	provider, ok := dispatcher.providers[token]
	return provider, ok
}

type uiaWndProcHandler struct{ provider *uiaRootProvider }

func (handler *uiaWndProcHandler) handleWndProcMessage(hwnd uintptr, message uint32, wParam, lParam uintptr) (bool, uintptr, error) {
	if handler == nil || handler.provider == nil || message != wmGetObject || int32(lParam) != uiaRootObjectID {
		return false, 0, nil
	}
	provider := handler.provider
	if provider.AddRef() == 0 {
		return false, 0, nil
	}
	defer provider.Release()
	rawProvider := provider.nativePointer.Load()
	if !provider.available() || hwnd != provider.hwnd || provider.token.Load() == 0 || rawProvider == 0 {
		return false, 0, nil
	}
	result := provider.api.ReturnRawElementProvider(hwnd, wParam, lParam, rawProvider)
	return true, result, nil
}
