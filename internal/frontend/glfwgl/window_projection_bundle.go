//go:build glfw

package glfwgl

import "errors"

func (b *nativeProjectionBundle) unbindProjection() error {
	if b == nil {
		return nil
	}
	var joined error
	if b.beforeUnbind != nil {
		joined = errors.Join(joined, b.beforeUnbind.close())
	}
	if b.unbind != nil {
		joined = errors.Join(joined, b.unbind())
		b.unbind = nil
	}
	return joined
}

func (b *nativeProjectionBundle) close() error {
	if b == nil || b.closed {
		return nil
	}
	b.closed = true
	var joined error
	joined = errors.Join(joined, b.unbindProjection())
	for i := len(b.resources) - 1; i >= 0; i-- {
		if b.resources[i] != nil {
			joined = errors.Join(joined, b.resources[i].Close())
		}
	}
	if b.host != nil {
		b.host.Destroy()
	}
	return joined
}
