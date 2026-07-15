package gpu

import (
	"errors"
	"image/color"
)

// errNotImplemented is returned by the stub backends (Vulkan, Metal) until they
// are built out. Shared here so the tagged files reuse one value.
var errNotImplemented = errors.New("gpu: backend not implemented yet")

// Renderer is the backend-neutral GPU contract for one terminal frame. The
// frontend translates its draw-list (solid rects + textured glyph quads) into
// these calls; each backend implements them for its API (OpenGL today,
// Vulkan/Metal as stubs). Coordinates are framebuffer pixels with a top-left
// origin — the backend sets up that space in BeginFrame. Colors are straight
// (non-premultiplied) RGBA; glyph draws tint an alpha-mask atlas quad by c.
//
// A frame is: BeginFrame → (FillRect | DrawGlyph)* → EndFrame. Atlas pages are
// (re)uploaded out of band with UploadAtlasPage when the glyph cache changes.
//
// NOTE: this shape is a first draft meant to be refined while implementing the
// OpenGL adapter (Phase 0). Expect the glyph/vertex signature and blend handling
// to evolve once a real backend drives it.
type Renderer interface {
	// Resize sets the drawable size in framebuffer pixels (window or DPI change).
	// Backends recreate their swapchain/drawable here.
	Resize(widthPx, heightPx int)

	// BeginFrame starts a frame and clears the drawable to clear, establishing the
	// top-left-origin pixel coordinate space the frontend draws in.
	BeginFrame(clear color.RGBA)

	// FillRect draws a solid (alpha-respecting) axis-aligned rectangle.
	FillRect(x, y, w, h float32, c color.RGBA)

	// DrawGlyph draws one textured quad sampled from atlas page: the destination
	// rect (x,y,w,h) in pixels, source UVs (u0,v0)-(u1,v1) in [0,1], tinted by c.
	DrawGlyph(page int, x, y, w, h, u0, v0, u1, v1 float32, c color.RGBA)

	// UploadAtlasPage (re)uploads an atlas page's RGBA pixels as a GPU texture.
	UploadAtlasPage(page int, rgba []byte, widthPx, heightPx int)

	// EndFrame finishes the frame and presents it (buffer swap / queue submit +
	// present).
	EndFrame()

	// Destroy releases every GPU resource the backend holds.
	Destroy()
}
