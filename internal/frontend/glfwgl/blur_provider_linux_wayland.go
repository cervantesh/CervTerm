//go:build glfw && linux && wayland

package glfwgl

/*
#cgo linux,wayland pkg-config: wayland-client
#include <errno.h>
#include <poll.h>
#include <stdlib.h>
#include <string.h>
#include <wayland-client.h>
#include "kde_blur_protocol_linux_wayland.h"

#define CERVTERM_WAYLAND_FLUSH_TIMEOUT_MS 100

typedef struct cervterm_wayland_blur {
    struct wl_display *display;
    struct wl_surface *surface;
    struct wl_registry *registry;
    struct org_kde_kwin_blur_manager *manager;
    struct org_kde_kwin_blur *blur;
    uint32_t manager_name;
    int async_error;
} cervterm_wayland_blur;

static void destroy_blur_manager(struct org_kde_kwin_blur_manager *manager) {
    wl_proxy_destroy((struct wl_proxy *)manager);
}

static int display_error(cervterm_wayland_blur *context) {
    int status = wl_display_get_error(context->display);
    return status == 0 ? 0 : -status;
}

static int flush_blur(cervterm_wayland_blur *context) {
    int status = display_error(context);
    if (status != 0) {
        return status;
    }

    errno = 0;
    if (wl_display_flush(context->display) >= 0) {
        return display_error(context);
    }
    int flush_errno = errno;
    if (flush_errno != EAGAIN) {
        return flush_errno == 0 ? -1 : -flush_errno;
    }

    int display_fd = wl_display_get_fd(context->display);
    if (display_fd < 0) {
        return -1;
    }
    struct pollfd writable = {
        .fd = display_fd,
        .events = POLLOUT,
    };
    errno = 0;
    int poll_status = poll(&writable, 1, CERVTERM_WAYLAND_FLUSH_TIMEOUT_MS);
    if (poll_status < 0) {
        return errno == 0 ? -1 : -errno;
    }
    if (poll_status == 0) {
        return -ETIMEDOUT;
    }
    if ((writable.revents & (POLLERR | POLLHUP | POLLNVAL)) != 0 ||
        (writable.revents & POLLOUT) == 0) {
        return -EIO;
    }

    errno = 0;
    if (wl_display_flush(context->display) < 0) {
        return errno == 0 ? -1 : -errno;
    }
    return display_error(context);
}

static void registry_global(
    void *data,
    struct wl_registry *registry,
    uint32_t name,
    const char *interface,
    uint32_t version) {
    cervterm_wayland_blur *context = data;
    if (context->manager != NULL || strcmp(interface, "org_kde_kwin_blur_manager") != 0) {
        return;
    }
    uint32_t bind_version = version < 1 ? version : 1;
    if (bind_version == 0) {
        return;
    }
    context->manager = wl_registry_bind(
        registry,
        name,
        &org_kde_kwin_blur_manager_interface,
        bind_version);
    if (context->manager != NULL) {
        context->manager_name = name;
    }
}

static void registry_remove(
    void *data,
    struct wl_registry *registry,
    uint32_t name) {
    (void)registry;
    cervterm_wayland_blur *context = data;
    if (context->manager_name != name) {
        return;
    }
    if (context->blur != NULL) {
        if (context->manager != NULL) {
            org_kde_kwin_blur_manager_unset(context->manager, context->surface);
            org_kde_kwin_blur_release(context->blur);
        } else {
            wl_proxy_destroy((struct wl_proxy *)context->blur);
        }
        context->blur = NULL;
    }
    if (context->manager != NULL) {
        int status = flush_blur(context);
        if (status != 0) {
            context->async_error = status;
        }
        destroy_blur_manager(context->manager);
        context->manager = NULL;
    }
    context->manager_name = 0;
}

static const struct wl_registry_listener registry_listener = {
    .global = registry_global,
    .global_remove = registry_remove,
};

static cervterm_wayland_blur *create_blur(
    void *display_pointer,
    void *surface_pointer,
    int *status_out) {
    if (status_out == NULL) {
        return NULL;
    }
    *status_out = 0;
    struct wl_display *display = display_pointer;
    struct wl_surface *surface = surface_pointer;
    if (display == NULL || surface == NULL) {
        *status_out = -1;
        return NULL;
    }
    cervterm_wayland_blur *context = calloc(1, sizeof(*context));
    if (context == NULL) {
        *status_out = -ENOMEM;
        return NULL;
    }
    context->display = display;
    context->surface = surface;
    context->registry = wl_display_get_registry(display);
    if (context->registry == NULL ||
        wl_registry_add_listener(context->registry, &registry_listener, context) != 0) {
        *status_out = -1;
        goto fail;
    }
    if (wl_display_roundtrip(display) < 0) {
        *status_out = display_error(context);
        if (*status_out == 0) {
            *status_out = -1;
        }
        goto fail;
    }
    if (context->manager == NULL) {
        *status_out = 1;
        goto fail;
    }
    return context;

fail:
    if (context->manager != NULL) {
        destroy_blur_manager(context->manager);
    }
    if (context->registry != NULL) {
        wl_registry_destroy(context->registry);
    }
    free(context);
    return NULL;
}

static int set_blur(cervterm_wayland_blur *context, int enabled) {
    if (context == NULL) {
        return -1;
    }
    if (context->async_error != 0) {
        int status = context->async_error;
        context->async_error = 0;
        return status;
    }
    int status = display_error(context);
    if (status != 0) {
        return status;
    }
    if (!enabled) {
        if (context->blur == NULL) {
            return 0;
        }
        if (context->manager != NULL) {
            org_kde_kwin_blur_manager_unset(context->manager, context->surface);
            org_kde_kwin_blur_release(context->blur);
        } else {
            wl_proxy_destroy((struct wl_proxy *)context->blur);
        }
        context->blur = NULL;
        return flush_blur(context);
    }
    if (context->manager == NULL) {
        return -2;
    }
    if (context->blur == NULL) {
        context->blur = org_kde_kwin_blur_manager_create(
            context->manager,
            context->surface);
        if (context->blur == NULL) {
            return -3;
        }
    }
    org_kde_kwin_blur_set_region(context->blur, NULL);
    org_kde_kwin_blur_commit(context->blur);
    return flush_blur(context);
}

static int destroy_blur(cervterm_wayland_blur *context) {
    if (context == NULL) {
        return 0;
    }
    int status = set_blur(context, 0);
    if (context->blur != NULL) {
        wl_proxy_destroy((struct wl_proxy *)context->blur);
        context->blur = NULL;
    }
    if (context->manager != NULL) {
        destroy_blur_manager(context->manager);
    }
    if (context->registry != NULL) {
        wl_registry_destroy(context->registry);
    }
    free(context);
    return status;
}
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/go-gl/glfw/v3.3/glfw"
)

type waylandBlurProvider struct {
	context *C.cervterm_wayland_blur
	native  *nativeBlurProvider
}

func newBlurProvider(w *glfw.Window) BlurProvider {
	display := glfw.GetWaylandDisplay()
	surface := w.GetWaylandWindow()
	if display == nil || surface == nil {
		return unsupportedBlurProvider{name: "linux-wayland"}
	}
	var initStatus C.int
	context := C.create_blur(unsafe.Pointer(display), unsafe.Pointer(surface), &initStatus)
	if context == nil {
		if initStatus == 1 {
			return unsupportedBlurProvider{name: "linux-wayland-kde-blur"}
		}
		return failedBlurProvider{
			name: "linux-wayland-kde-blur",
			err:  fmt.Errorf("initialize KDE Wayland blur: status %d", int(initStatus)),
		}
	}
	provider := &waylandBlurProvider{context: context}
	provider.native = &nativeBlurProvider{
		name:          "linux-wayland-kde-blur",
		compatibility: transparentCompositorCompatibility,
		set:           provider.setEnabled,
	}
	return provider
}

func (p *waylandBlurProvider) Name() string { return p.native.Name() }

func (p *waylandBlurProvider) Apply(request BlurRequest) BlurResult {
	return p.native.Apply(request)
}

func (p *waylandBlurProvider) setEnabled(enabled bool) error {
	if p.context == nil {
		return nil
	}
	value := C.int(0)
	if enabled {
		value = 1
	}
	status := int(C.set_blur(p.context, value))
	if status == -2 {
		return fmt.Errorf("%w: org_kde_kwin_blur manager unavailable", errBlurUnsupported)
	}
	if status != 0 {
		return fmt.Errorf("set KDE Wayland blur: status %d", status)
	}
	return nil
}

func (p *waylandBlurProvider) Close() error {
	err := p.native.Close()
	if p.context != nil {
		if status := int(C.destroy_blur(p.context)); status != 0 && err == nil {
			err = fmt.Errorf("destroy KDE Wayland blur: status %d", status)
		}
		p.context = nil
	}
	return err
}
