//go:build glfw && darwin

package glfwgl

/*
#cgo darwin LDFLAGS: -framework Cocoa
#include <stdint.h>

typedef struct cervterm_visual_effect cervterm_visual_effect;

cervterm_visual_effect *cervterm_visual_effect_create(void *ns_window, int *status_out);
int cervterm_visual_effect_set(cervterm_visual_effect *effect, int enabled);
void cervterm_visual_effect_destroy(cervterm_visual_effect *effect);
*/
import "C"

import (
	"fmt"

	"github.com/go-gl/glfw/v3.3/glfw"
)

type darwinBlurProvider struct {
	effect *C.cervterm_visual_effect
	native *nativeBlurProvider
}

func newBlurProvider(w *glfw.Window) BlurProvider {
	nsWindow := w.GetCocoaWindow()
	if nsWindow == nil {
		return unsupportedBlurProvider{name: "macos-appkit"}
	}
	var initStatus C.int
	effect := C.cervterm_visual_effect_create(nsWindow, &initStatus)
	if effect == nil {
		return failedBlurProvider{
			name: "macos-appkit",
			err:  fmt.Errorf("initialize macOS visual effect: status %d", int(initStatus)),
		}
	}
	provider := &darwinBlurProvider{effect: effect}
	provider.native = &nativeBlurProvider{
		name:          "macos-appkit-visual-effect",
		compatibility: transparentCompositorCompatibility,
		set:           provider.setEnabled,
	}
	return provider
}

func (p *darwinBlurProvider) Name() string { return p.native.Name() }

func (p *darwinBlurProvider) Apply(request BlurRequest) BlurResult {
	return p.native.Apply(request)
}

func (p *darwinBlurProvider) setEnabled(enabled bool) error {
	if p.effect == nil {
		return nil
	}
	value := C.int(0)
	if enabled {
		value = 1
	}
	if result := C.cervterm_visual_effect_set(p.effect, value); result != 0 {
		return fmt.Errorf("set macOS visual effect: status %d", int(result))
	}
	return nil
}

func (p *darwinBlurProvider) Close() error {
	err := p.native.Close()
	if p.effect != nil {
		C.cervterm_visual_effect_destroy(p.effect)
		p.effect = nil
	}
	return err
}
