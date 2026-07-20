//go:build glfw && linux && !wayland

package glfwgl

/*
#cgo linux,!wayland LDFLAGS: -lX11
#include <X11/Xatom.h>
#include <X11/Xlib.h>

static Atom cervterm_x11_blur_atom(Display *display) {
    if (display == NULL) {
        return None;
    }
    return XInternAtom(display, "_KDE_NET_WM_BLUR_BEHIND_REGION", True);
}

static int cervterm_x11_blur_supported(void *display_pointer) {
    Display *display = (Display *)display_pointer;
    if (display == NULL) {
        return 0;
    }
    Atom blur = cervterm_x11_blur_atom(display);
    Atom supported = XInternAtom(display, "_NET_SUPPORTED", True);
    if (blur == None || supported == None) {
        return 0;
    }

    long offset = 0;
    for (;;) {
        Atom actual_type = None;
        int actual_format = 0;
        unsigned long item_count = 0;
        unsigned long bytes_after = 0;
        unsigned char *data = NULL;
        int status = XGetWindowProperty(
            display,
            DefaultRootWindow(display),
            supported,
            offset,
            1024,
            False,
            XA_ATOM,
            &actual_type,
            &actual_format,
            &item_count,
            &bytes_after,
            &data);
        if (status != Success || actual_type != XA_ATOM || actual_format != 32 || data == NULL) {
            if (data != NULL) {
                XFree(data);
            }
            return 0;
        }

        Atom *atoms = (Atom *)data;
        for (unsigned long index = 0; index < item_count; index++) {
            if (atoms[index] == blur) {
                XFree(data);
                return 1;
            }
        }
        XFree(data);
        if (bytes_after == 0 || item_count == 0) {
            return 0;
        }
        offset += (long)item_count;
    }
}

static int cervterm_x11_error_code = 0;

static int cervterm_x11_error_handler(Display *display, XErrorEvent *event) {
    (void)display;
    cervterm_x11_error_code = event->error_code;
    return 0;
}

static int cervterm_x11_blur_set(void *display_pointer, unsigned long window, int enabled) {
    Display *display = (Display *)display_pointer;
    Atom blur = cervterm_x11_blur_atom(display);
    if (display == NULL || window == 0) {
        return -1;
    }
    if (blur == None) {
        return enabled ? -2 : 0;
    }

    // GLFW and this provider use Xlib only on the locked main thread. Drain old
    // errors, trap this request, and restore GLFW's handler immediately after.
    XSync(display, False);
    int (*previous_handler)(Display *, XErrorEvent *) = XSetErrorHandler(cervterm_x11_error_handler);
    cervterm_x11_error_code = 0;
    if (enabled) {
        XChangeProperty(display, window, blur, XA_CARDINAL, 32, PropModeReplace, NULL, 0);
    } else {
        XDeleteProperty(display, window, blur);
    }
    XSync(display, False);
    XSetErrorHandler(previous_handler);
    if (cervterm_x11_error_code != 0) {
        return -cervterm_x11_error_code;
    }
    return 0;
}
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func newBlurProvider(w *glfw.Window) BlurProvider {
	display := glfw.GetX11Display()
	window := w.GetX11Window()
	if display == nil || window == 0 {
		return unsupportedBlurProvider{name: "linux-x11"}
	}
	displayPointer := unsafe.Pointer(display)
	return &nativeBlurProvider{
		name:          "linux-x11-kde-blur",
		compatibility: transparentCompositorCompatibility,
		set: func(enabled bool) error {
			if enabled && C.cervterm_x11_blur_supported(displayPointer) == 0 {
				return fmt.Errorf("%w: KWin does not advertise _KDE_NET_WM_BLUR_BEHIND_REGION", errBlurUnsupported)
			}
			value := C.int(0)
			if enabled {
				value = 1
			}
			if status := C.cervterm_x11_blur_set(displayPointer, C.ulong(window), value); status != 0 {
				return fmt.Errorf("set KDE X11 blur: status %d", int(status))
			}
			return nil
		},
	}
}
