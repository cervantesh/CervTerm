/* Pinned minimal client binding for org_kde_kwin_blur v1. */
#ifndef CERVTERM_KDE_BLUR_PROTOCOL_H
#define CERVTERM_KDE_BLUR_PROTOCOL_H

#include <stddef.h>
#include <stdint.h>
#include <wayland-client.h>

#ifdef __cplusplus
extern "C" {
#endif

struct org_kde_kwin_blur;
struct org_kde_kwin_blur_manager;

extern const struct wl_interface org_kde_kwin_blur_interface;
extern const struct wl_interface org_kde_kwin_blur_manager_interface;

#define ORG_KDE_KWIN_BLUR_MANAGER_CREATE 0
#define ORG_KDE_KWIN_BLUR_MANAGER_UNSET 1
#define ORG_KDE_KWIN_BLUR_COMMIT 0
#define ORG_KDE_KWIN_BLUR_SET_REGION 1
#define ORG_KDE_KWIN_BLUR_RELEASE 2

static inline struct org_kde_kwin_blur *org_kde_kwin_blur_manager_create(
    struct org_kde_kwin_blur_manager *manager,
    struct wl_surface *surface) {
    struct wl_proxy *id = wl_proxy_marshal_flags(
        (struct wl_proxy *)manager,
        ORG_KDE_KWIN_BLUR_MANAGER_CREATE,
        &org_kde_kwin_blur_interface,
        wl_proxy_get_version((struct wl_proxy *)manager),
        0,
        NULL,
        surface);
    return (struct org_kde_kwin_blur *)id;
}

static inline void org_kde_kwin_blur_manager_unset(
    struct org_kde_kwin_blur_manager *manager,
    struct wl_surface *surface) {
    wl_proxy_marshal_flags(
        (struct wl_proxy *)manager,
        ORG_KDE_KWIN_BLUR_MANAGER_UNSET,
        NULL,
        wl_proxy_get_version((struct wl_proxy *)manager),
        0,
        surface);
}

static inline void org_kde_kwin_blur_commit(struct org_kde_kwin_blur *blur) {
    wl_proxy_marshal_flags(
        (struct wl_proxy *)blur,
        ORG_KDE_KWIN_BLUR_COMMIT,
        NULL,
        wl_proxy_get_version((struct wl_proxy *)blur),
        0);
}

static inline void org_kde_kwin_blur_set_region(
    struct org_kde_kwin_blur *blur,
    struct wl_region *region) {
    wl_proxy_marshal_flags(
        (struct wl_proxy *)blur,
        ORG_KDE_KWIN_BLUR_SET_REGION,
        NULL,
        wl_proxy_get_version((struct wl_proxy *)blur),
        0,
        region);
}

static inline void org_kde_kwin_blur_release(struct org_kde_kwin_blur *blur) {
    wl_proxy_marshal_flags(
        (struct wl_proxy *)blur,
        ORG_KDE_KWIN_BLUR_RELEASE,
        NULL,
        wl_proxy_get_version((struct wl_proxy *)blur),
        WL_MARSHAL_FLAG_DESTROY);
}

#ifdef __cplusplus
}
#endif
#endif
