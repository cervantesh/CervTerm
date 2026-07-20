//go:build glfw && linux && wayland

/* Pinned minimal interface metadata for org_kde_kwin_blur v1. */
#include <stddef.h>
#include <wayland-util.h>

#ifndef __has_attribute
#define __has_attribute(x) 0
#endif
#if (__has_attribute(visibility) || defined(__GNUC__) && __GNUC__ >= 4)
#define CERVTERM_WL_PRIVATE __attribute__ ((visibility("hidden")))
#else
#define CERVTERM_WL_PRIVATE
#endif

extern const struct wl_interface wl_region_interface;
extern const struct wl_interface wl_surface_interface;
extern const struct wl_interface org_kde_kwin_blur_interface;

static const struct wl_interface *blur_types[] = {
	&org_kde_kwin_blur_interface,
	&wl_surface_interface,
	&wl_region_interface,
};

static const struct wl_message manager_requests[] = {
	{ "create", "no", blur_types + 0 },
	{ "unset", "o", blur_types + 1 },
};

CERVTERM_WL_PRIVATE const struct wl_interface org_kde_kwin_blur_manager_interface = {
	"org_kde_kwin_blur_manager", 1,
	2, manager_requests,
	0, NULL,
};

static const struct wl_message blur_requests[] = {
	{ "commit", "", blur_types + 0 },
	{ "set_region", "?o", blur_types + 2 },
	{ "release", "", blur_types + 0 },
};

CERVTERM_WL_PRIVATE const struct wl_interface org_kde_kwin_blur_interface = {
	"org_kde_kwin_blur", 1,
	3, blur_requests,
	0, NULL,
};
