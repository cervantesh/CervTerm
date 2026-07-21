package render

import (
	"cervterm/internal/core"
	"cervterm/internal/termimage"
)

// Snapshot is the renderer-neutral view of a terminal frame.
//
// It intentionally contains only plain values and core cells. Frontends may copy
// this into GPU buffers, text layouts, or remote protocols without depending on
// PTY, parser, GLFW, or OpenGL internals.
type ImagePlacement struct {
	PaneObject uint64
	Placement  termimage.Placement
}

type Snapshot struct {
	Cols, Rows             int
	HistoryRows            int
	DisplayOffset          int
	CursorRow, CursorCol   int
	CursorVisible          bool
	CursorStyle            core.CursorStyle
	Title                  string
	Cwd                    string
	BellCount              int
	PaletteOverrides       core.PaletteOverrides
	Cells                  []core.Cell
	Wrapped                []bool
	Hyperlinks             []core.Hyperlink
	SemanticZones          []core.SemanticZone
	SemanticZonesTruncated bool
	Images                 []ImagePlacement
	ImageGeneration        uint64
	PaneObject             uint64
	imagePlacements        []termimage.Placement
	imageCrops             []termimage.PixelRect
}

type CaptureOptions struct {
	HideCursorWhenScrolled bool
	PaneObject             uint64
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
	if cap(dst.Wrapped) < term.Rows() {
		dst.Wrapped = make([]bool, term.Rows())
	} else {
		dst.Wrapped = dst.Wrapped[:term.Rows()]
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
	dst.PaletteOverrides = term.PaletteOverrides()
	dst.PaneObject = opts.PaneObject
	term.CopyView(dst.Cells)
	for row := range dst.Wrapped {
		dst.Wrapped[row], _ = term.LineWrapped(row)
	}
	dst.Hyperlinks = term.ProjectHyperlinks(dst.Cells, dst.Hyperlinks)
	dst.SemanticZones, dst.SemanticZonesTruncated = core.ProjectSemanticZones(dst.Cells, dst.SemanticZones)
	dst.imagePlacements, dst.imageCrops, dst.ImageGeneration = term.CopyImageProjection(dst.imagePlacements, dst.imageCrops, term.ViewportTopGlobalRow(), term.Rows())
	if cap(dst.Images) < len(dst.imagePlacements) {
		dst.Images = make([]ImagePlacement, len(dst.imagePlacements))
	} else {
		dst.Images = dst.Images[:len(dst.imagePlacements)]
	}
	for index, placement := range dst.imagePlacements {
		dst.Images[index] = ImagePlacement{PaneObject: opts.PaneObject, Placement: placement}
	}
}
