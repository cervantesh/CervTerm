//go:build webgpu

// WebGPU backend — STUB. Build with `-tags webgpu`. Nothing is implemented yet;
// this file mirrors the Vulkan and Metal phase scaffolds without selecting or
// importing a WebGPU implementation.
//
// A real backend must first choose and pin a native WebGPU implementation and Go
// bridge (for example wgpu-native or Dawn), then connect the GLFW native window
// to that implementation's surface API. Keep that platform plumbing out of the
// backend-neutral Renderer contract.

package gpu

import "image/color"

// webgpuRenderer holds the WebGPU objects a terminal backend needs. The concrete
// types remain comments so this scaffold compiles without cgo or a WebGPU binding.
type webgpuRenderer struct {
	widthPx, heightPx int

	// TODO Phase 1 — instance, surface, adapter, device, and queue:
	//   instance wgpu.Instance
	//   surface  wgpu.Surface
	//   adapter  wgpu.Adapter
	//   device   wgpu.Device
	//   queue    wgpu.Queue
	//
	// TODO Phase 2 — surface and persistent frame target:
	//   surfaceConfig wgpu.SurfaceConfiguration
	//   frameTarget   texture // persistent offscreen color target
	//   frameView     wgpu.TextureView
	//
	// TODO Phase 3 — ordered geometry and pipelines (WGSL shaders):
	//   pipelineSolid    wgpu.RenderPipeline
	//   pipelineGlyph    wgpu.RenderPipeline
	//   pipelineSubpixel [2]wgpu.RenderPipeline
	//   vertexBuffers    []buffer
	//
	// TODO Phase 4 — atlas textures and bindings:
	//   atlasPages map[int]texture
	//   sampler    wgpu.Sampler
	//   bindGroups []wgpu.BindGroup
}

var _ Renderer = (*webgpuRenderer)(nil)

// NewWebGPURenderer will initialize the selected native WebGPU implementation,
// surface, adapter, device, queue, persistent target, and pipelines. It errors
// until those phases are implemented.
func NewWebGPURenderer(widthPx, heightPx int) (Renderer, error) {
	return nil, errNotImplemented
}

// --- Init phases (fill these in after selecting a binding) -------------------

func (r *webgpuRenderer) createInstance() error         { return errNotImplemented }
func (r *webgpuRenderer) createSurface() error          { return errNotImplemented }
func (r *webgpuRenderer) requestAdapter() error         { return errNotImplemented }
func (r *webgpuRenderer) requestDevice() error          { return errNotImplemented }
func (r *webgpuRenderer) configureSurface() error       { return errNotImplemented }
func (r *webgpuRenderer) createPersistentTarget() error { return errNotImplemented }
func (r *webgpuRenderer) createPipelines() error        { return errNotImplemented }
func (r *webgpuRenderer) createVertexBuffers() error    { return errNotImplemented }
func (r *webgpuRenderer) createAtlasResources() error   { return errNotImplemented }

// --- Renderer interface (stubbed) -------------------------------------------

func (r *webgpuRenderer) Resize(widthPx, heightPx int) {
	r.widthPx, r.heightPx = widthPx, heightPx
	// TODO: mark the surface configuration and persistent target for recreation.
}

// PARTIAL-REDRAW HAZARD (must solve before this backend is correct): surface
// textures rotate and do not preserve the image just presented. CervTerm repaints
// only damaged rows, so rendering directly into GetCurrentTexture corrupts partial
// frames. Keep one persistent offscreen color target, update full or partial damage
// there, then copy it or composite it fullscreen into the current surface texture.
// BeginFrame must load that target without clearing; Clear clears only that target.
func (r *webgpuRenderer) BeginFrame(widthPx, heightPx int) {
	// TODO: acquire the current surface texture, create a command encoder, begin a
	// render pass that loads the persistent target, and reset ordered frame geometry.
}

func (r *webgpuRenderer) PushClip(rect ClipRect) {}
func (r *webgpuRenderer) PopClip()               {}

func (r *webgpuRenderer) Clear(c color.RGBA) {
	// TODO: clear the complete persistent target to c only when explicitly called.
}

func (r *webgpuRenderer) FillRect(x, y, w, h float32, c color.RGBA) {
	// TODO: append a colored quad while preserving Renderer call order.
}

func (r *webgpuRenderer) DrawGlyph(page int, mode GlyphMode, x, y, w, h, skew float32, u0, v0, u1, v1 float32, c color.RGBA) {
	// TODO: append a textured quad in call order. Preserve mask/color/subpixel
	// tint and blend semantics, including the two adjacent subpixel passes.
}

func (r *webgpuRenderer) ConfigureAtlas(pageCount, sizePx int) {
	// TODO: (re)create pageCount sizePx×sizePx RGBA textures and bind groups.
}

func (r *webgpuRenderer) UploadAtlasRegion(page, x, y, w, h int, rgba []byte) {
	// TODO: copy owned upload bytes into the page region before any dependent draw.
}

func (r *webgpuRenderer) ClearAtlasPage(page int) {
	// TODO: finish/submit the ordered prefix that samples this page (or version the
	// resource), wait for safe reuse as required, then clear the page. Reset may
	// happen in the middle of a frame, so unsubmitted earlier draws also count.
}

func (r *webgpuRenderer) EndFrame() {
	// TODO: encode draws in Renderer call order into the persistent target, copy or
	// composite the full target to the current surface texture, submit, and present.
}

func (r *webgpuRenderer) Destroy() {
	// TODO: wait for submitted work as required and release every owned resource.
}
