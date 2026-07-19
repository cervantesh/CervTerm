//go:build glfw

package glfwgl

import "errors"

func (b *nativeProjectionBundle) close() error {
	if b == nil || b.closed {
		return nil
	}
	b.closed = true
	var joined error
	if b.unbind != nil {
		joined = errors.Join(joined, b.unbind())
		b.unbind = nil
	}
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
