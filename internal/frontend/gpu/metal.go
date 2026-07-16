//go:build metal && darwin

// Metal backend — STUB (macOS only). Build with `-tags metal` on darwin. Nothing
// is implemented yet. Do Phase 0 (route the frontend through gpu.Renderer, see
// doc.go) before wiring this in.
//
// Metal is an Objective-C API, so a real backend needs a CGo/Obj-C bridge (or a
// Go Metal binding such as dmitri.shuralyov.com/gpu/mtl, which is incomplete).
// The drawable is a CAMetalLayer backing the GLFW Cocoa NSWindow's contentView.
// This stub is pure Go (no cgo) so it cross-compiles for review; the real one
// will pull in cgo and can only be built/run on a Mac.

package gpu

import "image/color"

// metalRenderer holds the Metal objects a terminal backend needs. All TODO; the
// Metal types are comments so this compiles without the binding/cgo yet.
type metalRenderer struct {
	widthPx, heightPx int

	// TODO Phase 1 — device & layer:
	//   device       mtl.Device
	//   layer        ca.MetalLayer          // attached to the NSView from GLFW
	//   queue        mtl.CommandQueue
	//
	// TODO Phase 2 — pipelines & shaders (Metal Shading Language, .metal):
	//   pipelineSolid mtl.RenderPipelineState   // colored quad
	//   pipelineGlyph mtl.RenderPipelineState   // textured, tinted quad
	//   library       mtl.Library
	//
	// TODO Phase 3 — geometry (retained: per-frame vertex buffers):
	//   vertexBuffers []mtl.Buffer              // triple-buffered
	//
	// TODO Phase 4 — atlas as textures:
	//   atlasPages    map[int]mtl.Texture
	//   sampler       mtl.SamplerState
}

var _ Renderer = (*metalRenderer)(nil)

// NewMetalRenderer will create the device, CAMetalLayer, queue, and pipelines.
// It errors until built out.
func NewMetalRenderer(widthPx, heightPx int) (Renderer, error) {
	return nil, errNotImplemented
}

// --- Init phases (fill these in) ----------------------------------------------

func (r *metalRenderer) createDevice() error    { return errNotImplemented } // MTLCreateSystemDefaultDevice
func (r *metalRenderer) createLayer() error     { return errNotImplemented } // CAMetalLayer on the GLFW NSView
func (r *metalRenderer) createQueue() error     { return errNotImplemented }
func (r *metalRenderer) createPipelines() error { return errNotImplemented } // compile MSL, solid + glyph pipelines
func (r *metalRenderer) createBuffers() error   { return errNotImplemented } // dynamic vertex buffers

// --- Renderer interface (stubbed) ---------------------------------------------

func (r *metalRenderer) Resize(widthPx, heightPx int) {
	r.widthPx, r.heightPx = widthPx, heightPx
	// TODO: update layer.drawableSize.
}

// PARTIAL-REDRAW HAZARD (must solve before this backend is correct): the frontend
// repaints only damaged rows and relies on the previous frame's pixels surviving in
// the drawable. CAMetalLayer drawables ROTATE — nextDrawable returns an OLDER or
// undefined frame, not the one just presented. Rendering straight into it will corrupt
// partial frames. Fix: keep a persistent offscreen MTLTexture, draw every frame into
// it, and blit it to the acquired drawable before present. Clear() clears THAT target.
func (r *metalRenderer) BeginFrame(widthPx, heightPx int) {
	// TODO: nextDrawable, command buffer, render command encoder with a load (not
	// clear) load action; set viewport to (widthPx, heightPx); reset the per-frame
	// vertex writer.
}

func (r *metalRenderer) PushClip(rect ClipRect) {}
func (r *metalRenderer) PopClip()               {}

func (r *metalRenderer) Clear(c color.RGBA) {
	// TODO: clear the current drawable to c (clear load action / clear encoder).
}

func (r *metalRenderer) ReplaceRect(x, y, w, h float32, c color.RGBA) {
	// TODO: append a quad using the unblended replacement pipeline.
}

func (r *metalRenderer) FillRect(x, y, w, h float32, c color.RGBA) {
	// TODO: append a colored quad to the solid batch.
}

func (r *metalRenderer) DrawGlyph(page int, mode GlyphMode, x, y, w, h, skew float32, u0, v0, u1, v1 float32, c color.RGBA) {
	// TODO: append a textured quad (page's texture) to the glyph batch, selecting
	// the blend/tint per mode (mask/color/subpixel).
}

func (r *metalRenderer) ConfigureAtlas(pageCount, sizePx int) {
	// TODO: (re)create pageCount sizePx×sizePx textures.
}

func (r *metalRenderer) UploadAtlasRegion(page, x, y, w, h int, rgba []byte) {
	// TODO: texture.ReplaceRegion for page's sub-rect at (x,y).
}

func (r *metalRenderer) ClearAtlasPage(page int) {
	// TODO: drain in-flight work (waitUntilCompleted), then clear page's texture.
}

func (r *metalRenderer) EndFrame() {
	// TODO: encode draws (bind pipeline+buffers+texture), endEncoding,
	// presentDrawable, commit.
}

func (r *metalRenderer) Destroy() {
	// TODO: release device/layer/queue/pipelines/textures.
}
