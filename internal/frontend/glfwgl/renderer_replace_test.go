//go:build glfw

package glfwgl

import (
	"image"
	"image/color"
	"testing"

	"cervterm/internal/config"
	"cervterm/internal/frontend/gpu"
)

type replaceRecordingRenderer struct {
	replaced           int
	blended            int
	cleared            int
	surfaceReplaced    int
	preparedBackground color.RGBA
	preparedSurface    *recordingBackgroundSurface
}

type recordingBackgroundSurface struct{ closed int }

func (s *recordingBackgroundSurface) Close() error { s.closed++; return nil }

func (*replaceRecordingRenderer) Resize(int, int)     {}
func (*replaceRecordingRenderer) BeginFrame(int, int) {}
func (r *replaceRecordingRenderer) Clear(color.RGBA)  { r.cleared++ }
func (r *replaceRecordingRenderer) ReplaceRect(float32, float32, float32, float32, color.RGBA) {
	r.replaced++
}
func (r *replaceRecordingRenderer) FillRect(float32, float32, float32, float32, color.RGBA) {
	r.blended++
}
func (*replaceRecordingRenderer) PushClip(gpu.ClipRect) {}
func (*replaceRecordingRenderer) PopClip()              {}
func (*replaceRecordingRenderer) DrawGlyph(int, gpu.GlyphMode, float32, float32, float32, float32, float32, float32, float32, float32, float32, color.RGBA) {
}
func (*replaceRecordingRenderer) ConfigureAtlas(int, int)                           {}
func (*replaceRecordingRenderer) UploadAtlasRegion(int, int, int, int, int, []byte) {}
func (*replaceRecordingRenderer) ClearAtlasPage(int)                                {}
func (*replaceRecordingRenderer) EndFrame()                                         {}
func (*replaceRecordingRenderer) Destroy()                                          {}
func (r *replaceRecordingRenderer) PrepareBackgroundSurface(surface *image.RGBA) (gpu.BackgroundSurface, error) {
	r.preparedBackground = surface.RGBAAt(surface.Rect.Min.X, surface.Rect.Min.Y)
	r.preparedSurface = &recordingBackgroundSurface{}
	return r.preparedSurface, nil
}
func (r *replaceRecordingRenderer) ReplaceBackgroundRect(gpu.BackgroundSurface, gpu.ClipRect) error {
	r.surfaceReplaced++
	return nil
}

func TestBackgroundReplacementAndOverlayBlendUseDistinctRendererOperations(t *testing.T) {
	r := &replaceRecordingRenderer{}
	a := &App{r: r}
	a.replaceRect(0, 0, 10, 10, color.RGBA{A: 0x80})
	a.fillRect(0, 0, 10, 10, color.RGBA{A: 0x80})
	if r.replaced != 1 || r.blended != 1 {
		t.Fatalf("renderer calls: replace=%d blend=%d", r.replaced, r.blended)
	}
}

func TestSolidBackgroundSurfaceUsesEffectiveAlphaAndReplacement(t *testing.T) {
	r := &replaceRecordingRenderer{}
	cfg := config.Defaults()
	cfg.Colors.Background = "#10203080"
	cfg.Window.BackgroundOpacity = 0.5
	surface, err := prepareSolidBackgroundSurface(r, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r.preparedBackground != (color.RGBA{R: 0x10, G: 0x20, B: 0x30, A: 0x40}) {
		t.Fatalf("prepared background = %#v", r.preparedBackground)
	}
	a := &App{r: r, backgroundSurface: surface}
	a.restoreBackgroundSurface(color.RGBA{R: 1, A: 255}, 100, 80)
	if r.surfaceReplaced != 1 || r.cleared != 0 {
		t.Fatalf("surface replace=%d clear=%d", r.surfaceReplaced, r.cleared)
	}
	if err := surface.Close(); err != nil || r.preparedSurface.closed != 1 {
		t.Fatalf("close err=%v count=%d", err, r.preparedSurface.closed)
	}
}

func TestApplyOpacityMultipliesAlphaOnce(t *testing.T) {
	if got := applyOpacity(color.RGBA{R: 1, A: 128}, 0.5); got.A != 64 || got.R != 1 {
		t.Fatalf("opacity result = %#v", got)
	}
	if got := applyOpacity(color.RGBA{A: 128}, 1); got.A != 128 {
		t.Fatalf("identity opacity changed alpha: %#v", got)
	}
}

func TestComposeSolidPanePreservesLegacyPixelsThenAppliesMultiplier(t *testing.T) {
	configured := color.RGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xE6}
	if got, want := composeSolidPane(configured, configured, 1), (color.RGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xFD}); got != want {
		t.Fatalf("default pane = %#v, want %#v", got, want)
	}
	if got := composeSolidPane(configured, configured, 0.5); got.A != 0x7F {
		t.Fatalf("multiplied pane alpha = %#x, want 0x7f", got.A)
	}
	osc := color.RGBA{R: 0xAA, G: 0xBB, B: 0xCC, A: 0xE6}
	if got, want := effectivePaneBackground(configured, osc, true, 0.5), (color.RGBA{R: 0xAA, G: 0xBB, B: 0xCC, A: 0x73}); got != want {
		t.Fatalf("OSC pane = %#v, want pure override %#v", got, want)
	}
}
