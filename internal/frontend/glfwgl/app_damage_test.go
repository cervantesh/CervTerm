//go:build glfw

package glfwgl

import (
	"image/color"
	"testing"

	"cervterm/internal/core"
	"cervterm/internal/render"
)

// TestCursorDamageBufferAge verifies the cursor's stale row is repainted for
// TWO frames after it moves, matching the content rows' buffer-age-2 handling.
// With a double-buffered back buffer alternating between the N-1 and N-2 images,
// clearing only the most recent cursor row leaves a ghost on the older buffer
// (the symptom: a phantom cursor after the startup banner).
func TestCursorDamageBufferAge(t *testing.T) {
	const rows, cols = 10, 20
	a := &App{contentScaleX: 1, contentScaleY: 1}
	a.snap = render.Snapshot{
		Cols:          cols,
		Rows:          rows,
		CursorVisible: true,
		Cells:         make([]core.Cell, rows*cols),
	}
	bg := color.RGBA{0, 0, 0, 0xFF}

	// Cell content never changes, so content-row damage stays empty and only the
	// cursor-row marking is exercised. Copy the damage slice before
	// recordDamageFrame lets the next frame reuse the backing buffer.
	run := func(cursorRow int) (damaged []bool, full bool) {
		a.snap.CursorRow = cursorRow
		full, d := a.prepareDamage(800, 600, 0, false, false, bg)
		damaged = make([]bool, len(d))
		copy(damaged, d)
		a.recordDamageFrame(800, 600, 0, false, false, bg, 0)
		return damaged, full
	}

	// Frames 1-3: cursor parked at row 0. The first two frames are full redraws
	// (prev/prevPrev hash buffers not yet populated); by frame 3 the damage path
	// is incremental.
	run(0)
	run(0)
	if _, full := run(0); full {
		t.Fatalf("frame 3 should be incremental, got full redraw")
	}

	// Frame 4: cursor jumps to row 5. Old row 0 must be damaged.
	d4, full4 := run(5)
	if full4 {
		t.Fatalf("cursor move must not force a full redraw")
	}
	if !d4[0] {
		t.Fatalf("frame 4: old cursor row 0 not marked damaged")
	}
	if !d4[5] {
		t.Fatalf("frame 4: new cursor row 5 not marked damaged")
	}

	// Frame 5: cursor stays at row 5. Row 0 must STILL be damaged — the back
	// buffer being drawn now is the N-2 image that still shows the row-0 cursor.
	d5, _ := run(5)
	if !d5[0] {
		t.Fatalf("frame 5: stale cursor row 0 not re-damaged (buffer-age-2 ghost)")
	}

	// Frame 6: both buffers have now repainted row 0, so it is no longer damaged.
	d6, _ := run(5)
	if d6[0] {
		t.Fatalf("frame 6: row 0 should no longer be damaged")
	}
}
