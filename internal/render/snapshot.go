package render

import "cervterm/internal/core"

// Snapshot is the renderer-neutral view of a terminal frame.
//
// It intentionally contains only plain values and core cells. Frontends may copy
// this into GPU buffers, text layouts, or remote protocols without depending on
// PTY, parser, GLFW, or OpenGL internals.
type Snapshot struct {
	Cols, Rows           int
	HistoryRows          int
	DisplayOffset        int
	CursorRow, CursorCol int
	CursorVisible        bool
	CursorStyle          core.CursorStyle
	Title                string
	Cwd                  string
	BellCount            int
	Cells                []core.Cell
}

type CaptureOptions struct {
	HideCursorWhenScrolled bool
}

// Capture copies terminal state into dst while reusing dst.Cells when possible.
//
// The copy is deliberate: renderers can consume a stable frame while the parser
// continues mutating the terminal. Reuse keeps steady-state capture allocation
// pressure at zero for unchanged dimensions.
func Capture(dst *Snapshot, term *core.Terminal) {
	CaptureWithOptions(dst, term, CaptureOptions{HideCursorWhenScrolled: true})
}

func CaptureWithOptions(dst *Snapshot, term *core.Terminal, opts CaptureOptions) {
	cellCount := term.Cols() * term.Rows()
	if cap(dst.Cells) < cellCount {
		dst.Cells = make([]core.Cell, cellCount)
	} else {
		dst.Cells = dst.Cells[:cellCount]
	}

	dst.Cols = term.Cols()
	dst.Rows = term.Rows()
	dst.HistoryRows = term.ScrollbackLines()
	dst.DisplayOffset = term.DisplayOffset()
	dst.CursorRow = term.CursorRow()
	dst.CursorCol = term.CursorCol()
	dst.CursorVisible = term.CursorVisible() && (!opts.HideCursorWhenScrolled || dst.DisplayOffset == 0)
	dst.CursorStyle = term.CursorStyle()
	dst.Title = term.Title()
	dst.Cwd = term.Cwd()
	dst.BellCount = term.BellCount()
	term.CopyView(dst.Cells)
}
