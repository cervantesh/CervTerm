//go:build glfw

package glfwgl

import (
	"image/color"
	"testing"

	"cervterm/internal/frontend/gpu"
)

type replaceRecordingRenderer struct {
	replaced int
	blended  int
}

func (*replaceRecordingRenderer) Resize(int, int)     {}
func (*replaceRecordingRenderer) BeginFrame(int, int) {}
func (*replaceRecordingRenderer) Clear(color.RGBA)    {}
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

func TestBackgroundReplacementAndOverlayBlendUseDistinctRendererOperations(t *testing.T) {
	r := &replaceRecordingRenderer{}
	a := &App{r: r}
	a.replaceRect(0, 0, 10, 10, color.RGBA{A: 0x80})
	a.fillRect(0, 0, 10, 10, color.RGBA{A: 0x80})
	if r.replaced != 1 || r.blended != 1 {
		t.Fatalf("renderer calls: replace=%d blend=%d", r.replaced, r.blended)
	}
}
