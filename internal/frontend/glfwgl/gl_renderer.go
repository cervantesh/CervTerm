//go:build glfw

package glfwgl

import (
	"image/color"

	"cervterm/internal/frontend/gpu"

	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// glRenderer implements gpu.Renderer with the OpenGL 2.1 immediate-mode calls
// the glfwgl frontend uses today. It is a faithful MOVE of the GL logic in
// app_draw.go (viewport/ortho, clear, fillRect, color helpers) and atlas.go
// (atlas texture create/clear/upload, drawEntry/drawQuad glyph emit) behind the
// backend-neutral interface.
//
// This is the active OpenGL implementation used by App and the shared glyph atlas.
//
// Blend state: BLEND is kept permanently enabled with the resting blend func
// SRC_ALPHA, ONE_MINUS_SRC_ALPHA (set once in the constructor; a GL context is
// current by construction when the renderer is created after gl.Init()). This is
// behavior-preserving — opaque fills use A=255 and render identically. The only
// place that departs from the resting blend is the subpixel two-pass in
// DrawGlyph, which restores it before returning.
//
// TEXTURE_2D state: tracked in texEnabled to avoid redundant Enable/Disable
// toggles. BeginFrame and FillRect ensure it is OFF; DrawGlyph ensures it is ON.
// The GL call is only emitted on a transition, which matches the original
// enable/disable pattern's net effect.
var _ gpu.Renderer = (*glRenderer)(nil)

type glRenderer struct {
	win          *glfw.Window
	pages        []uint32 // atlas page textures
	atlasSizePx  int      // per-side size the pages were configured with (ConfigureAtlas)
	boundTexture uint32   // currently bound page texture (0 = none)
	texEnabled   bool     // tracks gl.Enable/Disable(TEXTURE_2D) to skip redundant toggles
	widthPx      int
	heightPx     int
	clipStack    []gpu.ClipRect
}

// newGLRenderer builds an OpenGL Renderer for win. A GL context must be current
// (gl.Init already called) so the resting blend state can be set immediately.
func newGLRenderer(win *glfw.Window) *glRenderer {
	r := &glRenderer{win: win}
	// Resting blend state: always-on translucent blend (see type doc).
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	return r
}

// setTexEnabled emits gl.Enable/Disable(TEXTURE_2D) only on a state transition.
func (r *glRenderer) setTexEnabled(on bool) {
	if r.texEnabled == on {
		return
	}
	if on {
		gl.Enable(gl.TEXTURE_2D)
	} else {
		gl.Disable(gl.TEXTURE_2D)
	}
	r.texEnabled = on
}

func (r *glRenderer) Resize(widthPx, heightPx int) {
	// No GL here; the GL viewport is established per-frame in BeginFrame.
	r.widthPx, r.heightPx = widthPx, heightPx
}

func (r *glRenderer) BeginFrame(widthPx, heightPx int) {
	// Matches app_draw.go: viewport + ortho projection (top-left origin) + identity
	// modelview.
	gl.Viewport(0, 0, int32(widthPx), int32(heightPx))
	gl.MatrixMode(gl.PROJECTION)
	gl.LoadIdentity()
	gl.Ortho(0, float64(widthPx), float64(heightPx), 0, -1, 1)
	gl.MatrixMode(gl.MODELVIEW)
	gl.LoadIdentity()
	r.widthPx, r.heightPx = widthPx, heightPx
	// Texture off at frame start (matches app_draw.go's gl.Disable(TEXTURE_2D)).
	r.setTexEnabled(false)
	// Ensure the resting blend state (kept enabled for the whole frame).
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	r.clipStack = r.clipStack[:0]
	gl.Disable(gl.SCISSOR_TEST)
}

func (r *glRenderer) PushClip(rect gpu.ClipRect) {
	rect = rect.Clamp(r.widthPx, r.heightPx)
	if n := len(r.clipStack); n > 0 {
		rect = r.clipStack[n-1].Intersect(rect)
	}
	r.clipStack = append(r.clipStack, rect)
	r.applyCurrentClip()
}

func (r *glRenderer) applyCurrentClip() {
	if len(r.clipStack) == 0 {
		gl.Disable(gl.SCISSOR_TEST)
		return
	}
	rect := r.clipStack[len(r.clipStack)-1]
	x, y, width, height := rect.Scissor(r.heightPx)
	gl.Enable(gl.SCISSOR_TEST)
	gl.Scissor(x, y, width, height)
}

func (r *glRenderer) PopClip() {
	if len(r.clipStack) == 0 {
		panic("glfwgl: PopClip without matching PushClip")
	}
	r.clipStack = r.clipStack[:len(r.clipStack)-1]
	r.applyCurrentClip()
}

func (r *glRenderer) Clear(c color.RGBA) {
	// Clear always targets the complete drawable, independent of the clip stack.
	hadClip := len(r.clipStack) > 0
	if hadClip {
		gl.Disable(gl.SCISSOR_TEST)
	}
	gl.ClearColor(float32(c.R)/255, float32(c.G)/255, float32(c.B)/255, float32(c.A)/255)
	gl.Clear(gl.COLOR_BUFFER_BIT)
	if hadClip {
		r.applyCurrentClip()
	}
}

func (r *glRenderer) FillRect(x, y, w, h float32, c color.RGBA) {
	r.setTexEnabled(false)
	// Body of app_draw.go fillRect: color (incl. alpha) then a QUAD.
	gl.Color4f(float32(c.R)/255, float32(c.G)/255, float32(c.B)/255, float32(c.A)/255)
	gl.Begin(gl.QUADS)
	gl.Vertex2f(x, y)
	gl.Vertex2f(x+w, y)
	gl.Vertex2f(x+w, y+h)
	gl.Vertex2f(x, y+h)
	gl.End()
}

func (r *glRenderer) DrawGlyph(page int, mode gpu.GlyphMode, x, y, w, h, skew float32, u0, v0, u1, v1 float32, c color.RGBA) {
	// Bind the page texture (mirrors atlas.drawEntry's enable + lazy bind).
	r.setTexEnabled(true)
	if tex := r.pages[page]; tex != r.boundTexture {
		gl.BindTexture(gl.TEXTURE_2D, tex)
		r.boundTexture = tex
	}
	switch mode {
	case gpu.GlyphColor:
		// Pre-colored glyph (emoji): draw untinted (color forced white).
		gl.Color4ub(255, 255, 255, 255)
		r.drawQuad(x, y, w, h, skew, u0, v0, u1, v1)
	case gpu.GlyphSubpixel:
		// LCD subpixel two-pass (bit-identical to atlas.drawEntry): a coverage pass
		// with ZERO, ONE_MINUS_SRC_COLOR, then a tinted additive pass, then restore
		// the resting blend.
		gl.BlendFunc(gl.ZERO, gl.ONE_MINUS_SRC_COLOR)
		gl.Color4ub(255, 255, 255, 255)
		r.drawQuad(x, y, w, h, skew, u0, v0, u1, v1)
		gl.BlendFunc(gl.ONE, gl.ONE)
		gl.Color4f(float32(c.R)/255, float32(c.G)/255, float32(c.B)/255, float32(c.A)/255)
		r.drawQuad(x, y, w, h, skew, u0, v0, u1, v1)
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	default: // gpu.GlyphMask
		// Alpha-coverage mask tinted by c (the common text path).
		gl.Color4f(float32(c.R)/255, float32(c.G)/255, float32(c.B)/255, float32(c.A)/255)
		r.drawQuad(x, y, w, h, skew, u0, v0, u1, v1)
	}
}

// drawQuad is the body of atlas.drawQuad: a textured QUAD where skew shears only
// the top (y) row of vertices — synthetic italic.
func (r *glRenderer) drawQuad(x, y, w, h, skew float32, u0, v0, u1, v1 float32) {
	gl.Begin(gl.QUADS)
	gl.TexCoord2f(u0, v0)
	gl.Vertex2f(x+skew, y)
	gl.TexCoord2f(u1, v0)
	gl.Vertex2f(x+w+skew, y)
	gl.TexCoord2f(u1, v1)
	gl.Vertex2f(x+w, y+h)
	gl.TexCoord2f(u0, v1)
	gl.Vertex2f(x, y+h)
	gl.End()
}

func (r *glRenderer) ConfigureAtlas(pageCount, sizePx int) {
	// Replace any existing textures.
	if len(r.pages) > 0 {
		gl.DeleteTextures(int32(len(r.pages)), &r.pages[0])
	}
	r.atlasSizePx = sizePx
	r.pages = make([]uint32, pageCount)
	for i := range r.pages {
		// Mirrors atlas.createAtlasTexture.
		var tex uint32
		gl.GenTextures(1, &tex)
		gl.BindTexture(gl.TEXTURE_2D, tex)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
		gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(sizePx), int32(sizePx), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
		r.pages[i] = tex
	}
	r.boundTexture = 0
}

func (r *glRenderer) UploadAtlasRegion(page, x, y, w, h int, rgba []byte) {
	// Mirrors atlas.tryInsert's bind + glTexSubImage2D upload.
	tex := r.pages[page]
	if tex != r.boundTexture {
		gl.BindTexture(gl.TEXTURE_2D, tex)
		r.boundTexture = tex
	}
	gl.TexSubImage2D(gl.TEXTURE_2D, 0, int32(x), int32(y), int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(rgba))
}

func (r *glRenderer) ClearAtlasPage(page int) {
	// Drain: the original atlas.Reset does gl.Finish ONCE before clearing all pages;
	// here it is per-page. Per-page glFinish still fully drains the pipeline before
	// the texture is touched, so pending quads never sample cleared/overwritten
	// texels — behavior-preserving, just a finer-grained stall.
	gl.Finish()
	// Mirrors atlas.clearAtlasTexture: reallocate the page to zeroed contents at the
	// size ConfigureAtlas established (honor the interface's own dimension, not a
	// package const, so a clear never silently changes the texture size).
	sz := int32(r.atlasSizePx)
	gl.BindTexture(gl.TEXTURE_2D, r.pages[page])
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, sz, sz, 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	r.boundTexture = 0
}

func (r *glRenderer) EndFrame() {
	if len(r.clipStack) != 0 {
		panic("glfwgl: unbalanced renderer clip stack")
	}
	r.win.SwapBuffers()
}

func (r *glRenderer) Destroy() {
	if len(r.pages) > 0 {
		gl.DeleteTextures(int32(len(r.pages)), &r.pages[0])
	}
	r.pages = nil
	r.boundTexture = 0
}
