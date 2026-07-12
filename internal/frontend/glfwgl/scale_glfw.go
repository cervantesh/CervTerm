//go:build glfw

package glfwgl

import (
	"fmt"

	"cervterm/internal/fontglyph"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func DetectContentScale() string {
	if err := glfw.Init(); err != nil {
		return "unavailable (" + err.Error() + ")"
	}
	defer glfw.Terminate()
	monitor := glfw.GetPrimaryMonitor()
	if monitor == nil {
		return "unavailable (no primary monitor)"
	}
	x, y := monitor.GetContentScale()
	return fmt.Sprintf("%.2fx%.2f (effective DPI %.0f)", x, y, effectiveDPI(x, y))
}

func (a *App) applyScale(scaleX, scaleY float32) {
	a.contentScaleX, a.contentScaleY = scaleX, scaleY
	// Derive uiScale from the same clamped factor as the glyph DPI so chrome
	// never grows out of proportion with text past the 4x DPI clamp.
	a.uiScale = float32(effectiveDPI(scaleX, scaleY) / 96)
	a.paddingX = float32(a.cfg.Window.PaddingX) * a.uiScale
	a.paddingY = float32(a.cfg.Window.PaddingY) * a.uiScale
}

func (a *App) rebuildForContentScale(scaleX, scaleY float32) {
	if a.contentScaleX == scaleX && a.contentScaleY == scaleY {
		return
	}
	atlas, err := newGlyphAtlasWithSpec(fontglyph.Spec{Family: a.cfg.Font.Family, Size: a.cfg.Font.Size, DPI: effectiveDPI(scaleX, scaleY)})
	if err != nil {
		return
	}
	old := a.atlas
	a.atlas = atlas
	a.cellW = float32(atlas.cellW)
	a.cellH = float32(atlas.cellH)
	a.applyScale(scaleX, scaleY)
	a.cols, a.rows = 0, 0
	if old != nil {
		old.close()
	}
	a.resizeToWindow()
}
