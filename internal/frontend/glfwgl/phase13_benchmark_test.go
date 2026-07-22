//go:build glfw

package glfwgl

import (
	"image/color"
	"reflect"
	"testing"
	"time"

	"cervterm/internal/config"
	"cervterm/internal/core"
	termmux "cervterm/internal/mux"
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

func phase13DisabledFrameFixture() (*App, termmux.Layout, time.Time) {
	app := &App{
		terminalImages: terminalImageFrame{
			panes:      []terminalImagePaneFrame{{pane: 17, paneObject: 23, generation: 29}},
			draws:      []terminalImageDrawDescriptor{{z: -1, renderOrder: 3}},
			candidates: []terminalImageKeyCandidate{{z: 1, renderOrder: 5}},
		},
		terminalImageDamage: terminalImageDamageState{
			panes: map[uint64]terminalImagePaneDamage{
				23: {generation: 29, remaining: 1, seen: 7},
			},
			sequence: 7,
		},
	}
	layout := termmux.Layout{Panes: []termmux.PaneGeometry{{
		Pane: 17, Pixels: termmux.PixelRect{X: 3, Y: 5, Width: 80, Height: 32}, Cols: 10, Rows: 2,
	}}}
	return app, layout, time.Unix(123, 456)
}

func runPhase13DisabledFrame(app *App, layout termmux.Layout, now time.Time) {
	app.beginTerminalImageFrame(now, layout)
	app.drawTerminalImages(17, true)
	app.drawTerminalImages(17, false)
	app.finishTerminalImageFrame()
}

func assertPhase13DisabledFrameUnchanged(tb testing.TB, app *App, before terminalImageFrame, damage terminalImagePaneDamage, sequence uint64) {
	tb.Helper()
	if !reflect.DeepEqual(app.terminalImages, before) {
		tb.Fatalf("nil-cache frame mutated: got %#v want %#v", app.terminalImages, before)
	}
	if app.terminalImageDamage.sequence != sequence || len(app.terminalImageDamage.panes) != 1 || app.terminalImageDamage.panes[23] != damage {
		tb.Fatalf("nil-cache damage mutated: %#v", app.terminalImageDamage)
	}
}

func TestPhase13DisabledFrameIsAllocationAndMutationFree(t *testing.T) {
	app, layout, now := phase13DisabledFrameFixture()
	before := app.terminalImages
	damage := app.terminalImageDamage.panes[23]
	sequence := app.terminalImageDamage.sequence
	if allocs := testing.AllocsPerRun(1000, func() {
		runPhase13DisabledFrame(app, layout, now)
	}); allocs != 0 {
		t.Fatalf("disabled frame allocated %.0f times, want zero", allocs)
	}
	assertPhase13DisabledFrameUnchanged(t, app, before, damage, sequence)
}

func TestPhase13DisabledFrameAddsNoRedrawOrIdleCadence(t *testing.T) {
	app := &App{cfg: config.Defaults()}
	now := time.Unix(123, 456)
	if app.redrawWanted(now) {
		t.Fatal("disabled image dispatch requested redraw")
	}
	withoutImages := app.nextWakeTimeout(now)
	app.beginTerminalImageFrame(now, termmux.Layout{})
	app.finishTerminalImageFrame()
	if app.redrawWanted(now) || app.nextWakeTimeout(now) != withoutImages {
		t.Fatal("disabled image dispatch changed idle scheduling")
	}
}

// BenchmarkPhase13DisabledFrame measures the actual begin/draw/finish image
// integration seams with the default nil cache. The paired test above makes the
// zero-allocation and no-mutation requirements hard assertions.
func BenchmarkPhase13DisabledFrame(b *testing.B) {
	app, layout, now := phase13DisabledFrameFixture()
	before := app.terminalImages
	damage := app.terminalImageDamage.panes[23]
	sequence := app.terminalImageDamage.sequence
	runPhase13DisabledFrame(app, layout, now)
	assertPhase13DisabledFrameUnchanged(b, app, before, damage, sequence)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runPhase13DisabledFrame(app, layout, now)
	}
	b.StopTimer()
	assertPhase13DisabledFrameUnchanged(b, app, before, damage, sequence)
}
