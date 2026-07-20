//go:build glfw

package glfwgl

import (
	"strings"

	"cervterm/internal/accessibility"
	"cervterm/internal/core"
	termmux "cervterm/internal/mux"
	"cervterm/internal/render"
	selectmodel "cervterm/internal/selection"
)

type terminalAccessibilityCapture struct {
	ProviderID uint64
	Generation uint64
	RootID     accessibility.NodeID
	PaneID     accessibility.NodeID
	RootName   string
	PaneName   string
	Snapshot   render.Snapshot
	PanePixels termmux.PixelRect
	CellWidth  float64
	CellHeight float64
	Selection  *selectmodel.Range
	Bidi       bool
	Alternate  bool
}

func buildTerminalAccessibilityInput(capture terminalAccessibilityCapture) (accessibility.TerminalProjectionInput, error) {
	snapshot := capture.Snapshot
	if snapshot.Cols <= 0 || snapshot.Cols > accessibility.MaxGraphemes || snapshot.Rows <= 0 || snapshot.Rows > accessibility.MaxTerminalProjectionCells/snapshot.Cols || len(snapshot.Cells) != snapshot.Cols*snapshot.Rows || len(snapshot.Wrapped) != snapshot.Rows {
		return accessibility.TerminalProjectionInput{}, accessibility.ErrInvalidProjection
	}
	projectedRows := min(snapshot.Rows, accessibility.MaxRows)
	projectedCells := projectedRows * snapshot.Cols
	cells := make([]accessibility.TerminalCell, projectedCells)
	for index, cell := range snapshot.Cells[:projectedCells] {
		text := ""
		if cell.Rune != 0 && !cell.WideContinuation {
			var builder strings.Builder
			builder.WriteRune(cell.Rune)
			for _, combining := range cell.Combining() {
				builder.WriteRune(combining)
			}
			text = builder.String()
		}
		span := 0
		if text != "" {
			span = 1
			column := index % snapshot.Cols
			for column+span < snapshot.Cols && snapshot.Cells[index+span].WideContinuation {
				span++
			}
		}
		metadata := cell.HyperlinkID != 0 || cell.SemanticKind != core.SemanticNone
		cells[index] = accessibility.TerminalCell{
			Text: text, Blank: !metadata && (cell.Rune == 0 || cell.Rune == ' '), TrimBarrier: metadata && (cell.Rune == 0 || cell.WideContinuation),
			WideContinuation: cell.WideContinuation, Span: span,
		}
	}
	var visual [][]int
	if capture.Bidi {
		visual = make([][]int, projectedRows)
		for row := 0; row < projectedRows; row++ {
			start := row * snapshot.Cols
			visual[row] = render.VisualOrder(snapshot.Cells[start : start+snapshot.Cols])
		}
	}
	var selection *accessibility.TerminalSelection
	if capture.Selection != nil {
		selection = &accessibility.TerminalSelection{
			Start: accessibility.CellPoint{Row: capture.Selection.Start.Row, Col: capture.Selection.Start.Col},
			End:   accessibility.CellPoint{Row: capture.Selection.End.Row, Col: capture.Selection.End.Col},
		}
	}
	return accessibility.TerminalProjectionInput{
		ProviderID: capture.ProviderID, Generation: capture.Generation,
		RootID: capture.RootID, PaneID: capture.PaneID, RootName: capture.RootName, PaneName: capture.PaneName,
		Cols: snapshot.Cols, Rows: projectedRows, Cells: cells, Wrapped: append([]bool(nil), snapshot.Wrapped[:projectedRows]...), VisualToLogical: visual, Truncated: snapshot.Rows > projectedRows,
		CursorVisible: snapshot.CursorVisible, Cursor: accessibility.CellPoint{Row: snapshot.CursorRow, Col: snapshot.CursorCol}, Selection: selection,
		OriginX: float64(capture.PanePixels.X), OriginY: float64(capture.PanePixels.Y), CellWidth: capture.CellWidth, CellHeight: capture.CellHeight,
		Clip:            accessibility.Rect{X: float64(capture.PanePixels.X), Y: float64(capture.PanePixels.Y), Width: float64(capture.PanePixels.Width), Height: float64(capture.PanePixels.Height)},
		AlternateScreen: capture.Alternate, DisplayOffset: snapshot.DisplayOffset,
	}, nil
}
