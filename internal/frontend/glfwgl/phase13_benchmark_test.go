//go:build glfw

package glfwgl

import (
	"image/color"
	"testing"

	"cervterm/internal/config"
	"cervterm/internal/core"
	"cervterm/internal/render"
)

// BenchmarkPhase13DisabledDraw pins the context-free text-grid walk used when
// terminal images are disabled. It deliberately uses blank text cells so the
// benchmark needs no GLFW window, OpenGL context, glyph backend, or atlas.
func BenchmarkPhase13DisabledDraw(b *testing.B) {
	const cols, rows = 120, 40
	cells := make([]core.Cell, cols*rows)
	for i := range cells {
		cells[i].Rune = ' '
	}
	app := App{
		cfg:   config.Defaults(),
		snap:  render.Snapshot{Cols: cols, Rows: rows, Cells: cells},
		cellW: 8,
		cellH: 16,
	}
	resolver := core.NewColorResolver(core.DefaultFG, core.DefaultBG, core.ANSIColors())
	background := color.RGBA{A: 0xff}
	selection := color.RGBA{A: 0xff}
	foreground := color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}

	for row := 0; row < rows; row++ {
		app.drawRow(row, background, selection, foreground, &resolver)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for row := 0; row < rows; row++ {
			app.drawRow(row, background, selection, foreground, &resolver)
		}
	}
}
