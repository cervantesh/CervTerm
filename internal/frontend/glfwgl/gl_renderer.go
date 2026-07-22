//go:build glfw

package glfwgl

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"strings"

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
// Blend state: BLEND is kept permanently enabled with separate RGB and alpha
// factors so drawing into the authoritative RGBA target preserves destination
// alpha correctly. Opaque fills still render identically. The only place that
// departs from the resting blend is the subpixel two-pass in DrawGlyph, which
// restores it before returning.
//
// TEXTURE_2D state: tracked in texEnabled to avoid redundant Enable/Disable
// toggles. BeginFrame and FillRect ensure it is OFF; DrawGlyph ensures it is ON.
// The GL call is only emitted on a transition, which matches the original
// enable/disable pattern's net effect.
var _ gpu.Renderer = (*glRenderer)(nil)
var _ gpu.BackgroundSurfaceRenderer = (*glRenderer)(nil)
var _ gpu.TerminalImageRenderer = (*glRenderer)(nil)

type glBackgroundSurface struct {
	renderer      *glRenderer
	texture       uint32
	width, height int
}

func (s *glBackgroundSurface) Close() error {
	if s == nil || s.texture == 0 {
		return nil
	}
	if s.renderer != nil && s.renderer.boundTexture == s.texture {
		s.renderer.boundTexture = 0
	}
	gl.DeleteTextures(1, &s.texture)
	s.texture = 0
	return nil
}

type glRenderer struct {
	win          *glfw.Window
	pages        []uint32 // atlas page textures
	atlasSizePx  int      // per-side size the pages were configured with (ConfigureAtlas)
	boundTexture uint32   // currently bound renderer texture (0 = none)
	texEnabled   bool     // tracks gl.Enable/Disable(TEXTURE_2D) to skip redundant toggles
	imageGL      terminalImageGL
	widthPx      int
	heightPx     int
	clipStack    []gpu.ClipRect
	framebuffer  uint32
	frameTexture uint32
	targetReady  bool
}

// newGLRenderer builds an OpenGL Renderer for win. A GL context must be current
// (gl.Init already called) so the resting blend state can be set immediately.
func newGLRenderer(win *glfw.Window) *glRenderer {
	r := &glRenderer{win: win}
	// Resting blend state: always-on translucent blend (see type doc).
	gl.Enable(gl.BLEND)
	gl.BlendFuncSeparate(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA, gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
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

func (r *glRenderer) PrepareBackgroundSurface(surface *image.RGBA) (gpu.BackgroundSurface, error) {
	if surface == nil || surface.Rect.Dx() <= 0 || surface.Rect.Dy() <= 0 {
		return nil, fmt.Errorf("gpu: background surface must be non-empty RGBA")
	}
	width, height := surface.Rect.Dx(), surface.Rect.Dy()
	pixels := make([]byte, width*height*4)
	for y := 0; y < height; y++ {
		source := surface.PixOffset(surface.Rect.Min.X, surface.Rect.Min.Y+y)
		copy(pixels[y*width*4:(y+1)*width*4], surface.Pix[source:source+width*4])
	}
	var texture uint32
	gl.GenTextures(1, &texture)
	gl.BindTexture(gl.TEXTURE_2D, texture)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(width), int32(height), 0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(pixels))
	r.boundTexture = texture
	return &glBackgroundSurface{renderer: r, texture: texture, width: width, height: height}, nil
}

func (r *glRenderer) ReplaceBackgroundRect(surface gpu.BackgroundSurface, rect gpu.ClipRect) error {
	background, ok := surface.(*glBackgroundSurface)
	if !ok || background == nil || background.texture == 0 {
		return fmt.Errorf("gpu: invalid or closed background surface")
	}
	rect = rect.Clamp(r.widthPx, r.heightPx)
	if rect.Width <= 0 || rect.Height <= 0 {
		return nil
	}
	u0, v0, u1, v1 := float32(rect.X)/float32(background.width), float32(rect.Y)/float32(background.height), float32(rect.X+rect.Width)/float32(background.width), float32(rect.Y+rect.Height)/float32(background.height)
	if background.width == 1 {
		u0, u1 = 0, 1
	}
	if background.height == 1 {
		v0, v1 = 0, 1
	}
	r.setTexEnabled(true)
	if r.boundTexture != background.texture {
		gl.BindTexture(gl.TEXTURE_2D, background.texture)
		r.boundTexture = background.texture
	}
	gl.Disable(gl.BLEND)
	gl.Color4ub(255, 255, 255, 255)
	gl.Begin(gl.QUADS)
	gl.TexCoord2f(u0, v0)
	gl.Vertex2f(float32(rect.X), float32(rect.Y))
	gl.TexCoord2f(u1, v0)
	gl.Vertex2f(float32(rect.X+rect.Width), float32(rect.Y))
	gl.TexCoord2f(u1, v1)
	gl.Vertex2f(float32(rect.X+rect.Width), float32(rect.Y+rect.Height))
	gl.TexCoord2f(u0, v1)
	gl.Vertex2f(float32(rect.X), float32(rect.Y+rect.Height))
	gl.End()
	gl.Enable(gl.BLEND)
	gl.BlendFuncSeparate(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA, gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
	return nil
}

func (r *glRenderer) Resize(widthPx, heightPx int) {
	if widthPx == r.widthPx && heightPx == r.heightPx && r.targetReady {
		return
	}
	r.destroyFrameTarget()
	r.widthPx, r.heightPx = widthPx, heightPx
	if widthPx <= 0 || heightPx <= 0 {
		return
	}
	if !supportsFramebufferObject() {
		log.Printf("OpenGL framebuffer objects unavailable; using correctness-first full redraw fallback")
		return
	}
	gl.GenTextures(1, &r.frameTexture)
	gl.BindTexture(gl.TEXTURE_2D, r.frameTexture)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(widthPx), int32(heightPx), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	gl.GenFramebuffers(1, &r.framebuffer)
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.framebuffer)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, r.frameTexture, 0)
	r.targetReady = gl.CheckFramebufferStatus(gl.FRAMEBUFFER) == gl.FRAMEBUFFER_COMPLETE
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	r.boundTexture = 0
	if !r.targetReady {
		log.Printf("OpenGL RGBA frame target is incomplete; falling back to the window framebuffer")
		r.destroyFrameTarget()
	}
}

func (r *glRenderer) destroyFrameTarget() {
	if r.framebuffer != 0 {
		gl.DeleteFramebuffers(1, &r.framebuffer)
	}
	if r.frameTexture != 0 {
		gl.DeleteTextures(1, &r.frameTexture)
	}
	r.framebuffer, r.frameTexture = 0, 0
	r.targetReady = false
}

func (r *glRenderer) PersistentTargetReady() bool { return r.targetReady }

func supportsFramebufferObject() bool {
	extensions := gl.GoStr(gl.GetString(gl.EXTENSIONS))
	return strings.Contains(extensions, "GL_ARB_framebuffer_object")
}

func (r *glRenderer) BeginFrame(widthPx, heightPx int) {
	if r.targetReady {
		gl.BindFramebuffer(gl.FRAMEBUFFER, r.framebuffer)
	}
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
	gl.BlendFuncSeparate(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA, gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
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

func (r *glRenderer) ReplaceRect(x, y, w, h float32, c color.RGBA) {
	r.setTexEnabled(false)
	gl.Disable(gl.BLEND)
	r.solidQuad(x, y, w, h, c)
	gl.Enable(gl.BLEND)
	gl.BlendFuncSeparate(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA, gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
}

func (r *glRenderer) FillRect(x, y, w, h float32, c color.RGBA) {
	r.setTexEnabled(false)
	r.solidQuad(x, y, w, h, c)
}

func (r *glRenderer) solidQuad(x, y, w, h float32, c color.RGBA) {
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
		// Pre-colored glyphs keep their RGB and consume the caller's text alpha.
		gl.Color4ub(255, 255, 255, c.A)
		r.drawQuad(x, y, w, h, skew, u0, v0, u1, v1)
	case gpu.GlyphSubpixel:
		// Scale both the coverage removal and additive tint by caller alpha so
		// subpixel text opacity is applied exactly once.
		alpha := float32(c.A) / 255
		gl.BlendFuncSeparate(gl.ZERO, gl.ONE_MINUS_SRC_COLOR, gl.ZERO, gl.ONE)
		gl.Color4f(alpha, alpha, alpha, alpha)
		r.drawQuad(x, y, w, h, skew, u0, v0, u1, v1)
		gl.BlendFuncSeparate(gl.ONE, gl.ONE, gl.ZERO, gl.ONE)
		gl.Color4f(float32(c.R)/255*alpha, float32(c.G)/255*alpha, float32(c.B)/255*alpha, alpha)
		r.drawQuad(x, y, w, h, skew, u0, v0, u1, v1)
		gl.BlendFuncSeparate(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA, gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
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
	if r.targetReady {
		// Present the authoritative RGBA target as one full-frame image.
		gl.Disable(gl.SCISSOR_TEST)
		gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
		gl.Viewport(0, 0, int32(r.widthPx), int32(r.heightPx))
		gl.MatrixMode(gl.PROJECTION)
		gl.LoadIdentity()
		gl.Ortho(0, float64(r.widthPx), float64(r.heightPx), 0, -1, 1)
		gl.MatrixMode(gl.MODELVIEW)
		gl.LoadIdentity()
		gl.Disable(gl.BLEND)
		r.setTexEnabled(true)
		gl.BindTexture(gl.TEXTURE_2D, r.frameTexture)
		gl.Color4ub(255, 255, 255, 255)
		gl.Begin(gl.QUADS)
		gl.TexCoord2f(0, 1)
		gl.Vertex2f(0, 0)
		gl.TexCoord2f(1, 1)
		gl.Vertex2f(float32(r.widthPx), 0)
		gl.TexCoord2f(1, 0)
		gl.Vertex2f(float32(r.widthPx), float32(r.heightPx))
		gl.TexCoord2f(0, 0)
		gl.Vertex2f(0, float32(r.heightPx))
		gl.End()
		r.boundTexture = 0
		r.setTexEnabled(false)
		gl.Enable(gl.BLEND)
		gl.BlendFuncSeparate(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA, gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
	}
	r.win.SwapBuffers()
}

func (r *glRenderer) Destroy() {
	r.destroyFrameTarget()
	if len(r.pages) > 0 {
		gl.DeleteTextures(int32(len(r.pages)), &r.pages[0])
	}
	r.pages = nil
	r.boundTexture = 0
}
