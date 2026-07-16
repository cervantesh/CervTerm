package gpu

import (
	"errors"
	"image/color"
)

// errNotImplemented is returned by the stub backends (Vulkan, Metal, WebGPU)
// until they are built out. Shared here so tagged files reuse one value.
var errNotImplemented = errors.New("gpu: backend not implemented yet")

// GlyphMode selects how DrawGlyph blends/tints an atlas quad.
type GlyphMode uint8

const (
	// GlyphMask: alpha-coverage mask tinted by c (the common text path).
	GlyphMask GlyphMode = iota
	// GlyphColor: pre-colored glyph (emoji); drawn untinted (color forced white).
	GlyphColor
	// GlyphSubpixel: LCD subpixel mask; backend does its two-pass blend using c.
	GlyphSubpixel
)

// ClipRect is a half-open top-left-origin framebuffer rectangle.
type ClipRect struct {
	X, Y, Width, Height int
}

// Intersect returns the overlap of two clip rectangles.
func (r ClipRect) Intersect(other ClipRect) ClipRect {
	x1, y1 := max(r.X, other.X), max(r.Y, other.Y)
	x2, y2 := min(r.X+r.Width, other.X+other.Width), min(r.Y+r.Height, other.Y+other.Height)
	if x2 <= x1 || y2 <= y1 {
		return ClipRect{X: x1, Y: y1}
	}
	return ClipRect{X: x1, Y: y1, Width: x2 - x1, Height: y2 - y1}
}

// Clamp confines a clip rectangle to the framebuffer.
func (r ClipRect) Clamp(width, height int) ClipRect {
	return r.Intersect(ClipRect{Width: max(0, width), Height: max(0, height)})
}

// Scissor converts a top-left clip to OpenGL's bottom-left scissor coordinates.
func (r ClipRect) Scissor(framebufferHeight int) (x, y, width, height int32) {
	return int32(r.X), int32(framebufferHeight - (r.Y + r.Height)), int32(r.Width), int32(r.Height)
}

// Renderer is the backend-neutral GPU contract for one terminal frame. The
// frontend translates its draw-list (solid rects + textured glyph quads) into
// these calls; each backend implements them for its API (OpenGL today, with
// Vulkan, Metal, and WebGPU as stubs). Coordinates are framebuffer pixels with
// origin — the backend sets up that space in BeginFrame. Colors are straight
// (non-premultiplied) RGBA; glyph draws tint an alpha-mask atlas quad by c
// (per GlyphMode).
//
// A frame is: BeginFrame → (Clear? | FillRect | DrawGlyph)* → EndFrame. Atlas
// pages are allocated once with ConfigureAtlas and (re)uploaded out of band with
// UploadAtlasRegion / ClearAtlasPage when the glyph cache changes.
type Renderer interface {
	// Resize notes a new drawable size in framebuffer pixels. Backends that own a
	// swapchain/drawable recreate it here. The GL backend records the size; the
	// per-frame coordinate space is (re)established by BeginFrame.
	Resize(widthPx, heightPx int)

	// BeginFrame starts a frame and establishes a top-left-origin pixel coordinate
	// space sized to (widthPx, heightPx). It does NOT clear — the frontend keeps a
	// partial-redraw damage model and clears explicitly via Clear only on a full
	// redraw. (GL: glViewport + glOrtho + identity modelview.)
	BeginFrame(widthPx, heightPx int)

	// PushClip intersects rect with the current top-left-origin framebuffer clip.
	// PopClip restores the previous clip. BeginFrame resets the stack.
	PushClip(rect ClipRect)
	PopClip()

	// Clear clears the whole drawable to c. The frontend calls this only when it is
	// doing a full-frame redraw; partial-damage frames never call it.
	Clear(c color.RGBA)

	// FillRect draws a solid axis-aligned rectangle, alpha-blended (straight RGBA).
	FillRect(x, y, w, h float32, c color.RGBA)

	// DrawGlyph draws one textured quad from atlas page: destination rect (x,y,w,h)
	// in pixels, top edge sheared by skew (synthetic italic), source UVs
	// (u0,v0)-(u1,v1) in [0,1], per mode. mode selects tint/blend (see GlyphMode).
	DrawGlyph(page int, mode GlyphMode, x, y, w, h, skew float32, u0, v0, u1, v1 float32, c color.RGBA)

	// ConfigureAtlas (re)allocates pageCount square atlas textures of sizePx per side
	// (LINEAR filter, CLAMP_TO_EDGE), replacing any existing ones. Called once when the
	// atlas is built.
	ConfigureAtlas(pageCount, sizePx int)

	// UploadAtlasRegion uploads rgba (w*h*4 bytes, RGBA8) into page at (x,y).
	// (GL: bind page texture + glTexSubImage2D.)
	UploadAtlasRegion(page, x, y, w, h int, rgba []byte)

	// ClearAtlasPage drains any in-flight draws that sample this page, then clears it
	// (fully reallocates/zeroes the texture). Called when the glyph cache is reset
	// mid-frame; the drain (GL: glFinish) prevents pending quads from sampling
	// cleared/overwritten texels.
	ClearAtlasPage(page int)

	// EndFrame presents the frame (GL: SwapBuffers).
	EndFrame()

	// Destroy releases every GPU resource the backend holds.
	Destroy()
}
