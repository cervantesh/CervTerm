package accessibility

import (
	"strings"
	"unicode/utf8"

	"github.com/clipperhouse/uax29/v2/graphemes"
)

const MaxTerminalProjectionCells = 1 << 18

type CellPoint struct {
	Row int
	Col int
}

type TerminalCell struct {
	Text             string
	Blank            bool
	WideContinuation bool
	TrimBarrier      bool
	Span             int
}

type TerminalSelection struct {
	Start CellPoint
	End   CellPoint
}

type TerminalProjectionInput struct {
	ProviderID uint64
	Generation uint64
	RootID     NodeID
	PaneID     NodeID
	RootName   string
	PaneName   string

	Cols      int
	Rows      int
	Cells     []TerminalCell
	Wrapped   []bool
	Truncated bool

	// VisualToLogical maps visual columns to logical columns. A nil row is identity.
	VisualToLogical [][]int

	CursorVisible bool
	Cursor        CellPoint
	Selection     *TerminalSelection

	OriginX    float64
	OriginY    float64
	CellWidth  float64
	CellHeight float64
	Clip       Rect

	AlternateScreen bool
	DisplayOffset   int
}

type terminalCluster struct {
	text      string
	startCell int
	endCell   int
	bounds    Rect
}

type terminalRow struct {
	text        string
	clusters    []terminalCluster
	start       int
	end         int
	softWrapped bool
}

type terminalChunk struct {
	startByte int
	endByte   int
	startCell int
	endCell   int
}

func ProjectTerminal(input TerminalProjectionInput) (Document, error) {
	pane, truncated, err := projectTerminalNode(input)
	if err != nil {
		return Document{}, err
	}
	return NewDocument(DocumentDraft{
		ProviderID: input.ProviderID,
		Generation: input.Generation,
		Focus:      input.PaneID,
		Truncated:  truncated,
		Nodes: []NodeDraft{
			{ID: input.RootID, Role: RoleWindow, Name: input.RootName},
			pane,
		},
	})
}

func projectTerminalNode(input TerminalProjectionInput) (NodeDraft, bool, error) {
	return projectTerminalNodeWithBudget(input, MaxRows, MaxUTF8Bytes-len(input.RootName)-len(input.PaneName), MaxGraphemes)
}

func projectTerminalNodeWithBudget(input TerminalProjectionInput, rowBudget, byteBudget, graphemeBudget int) (NodeDraft, bool, error) {
	if err := validateTerminalProjection(input); err != nil {
		return NodeDraft{}, false, err
	}
	if rowBudget < 0 || byteBudget < 0 || graphemeBudget < 0 {
		return NodeDraft{}, false, ErrInvalidProjection
	}
	rowLimit := min(input.Rows, rowBudget)
	truncated := input.Truncated || input.Rows > rowLimit
	rows := make([]terminalRow, 0, rowLimit)
	remainingBytes := byteBudget
	remainingGraphemes := graphemeBudget
	for rowIndex := 0; rowIndex < rowLimit; rowIndex++ {
		projected, rowTruncated, err := projectTerminalRow(input, rowIndex, remainingBytes, remainingGraphemes)
		if err != nil {
			return NodeDraft{}, false, err
		}
		rows = append(rows, projected)
		remainingBytes -= len(projected.text)
		remainingGraphemes -= len(projected.clusters)
		if rowTruncated {
			truncated = true
			break
		}
	}
	assignTerminalRowOffsets(rows)

	rowDrafts := make([]RowDraft, len(rows))
	for index, row := range rows {
		bounds := make([]Rect, len(row.clusters))
		for clusterIndex, cluster := range row.clusters {
			bounds[clusterIndex] = cluster.bounds
		}
		rowDrafts[index] = RowDraft{Text: row.text, Bounds: bounds, SoftWrapped: row.softWrapped}
	}

	pane := NodeDraft{ID: input.PaneID, Parent: input.RootID, Role: RoleTerminal, Name: input.PaneName, Rows: rowDrafts}
	if input.CursorVisible && input.DisplayOffset == 0 && input.Cursor.Row >= 0 && input.Cursor.Row < len(rows) {
		caret := terminalCellBoundary(rows[input.Cursor.Row], input.Cursor.Col, false)
		pane.Caret = &caret
	}
	if input.Selection != nil && len(rows) != 0 {
		selection := normalizeTerminalSelection(*input.Selection)
		start := clampTerminalPoint(selection.Start, input.Cols, len(rows))
		end := clampTerminalPoint(selection.End, input.Cols, len(rows))
		span := Span{
			Start: terminalCellBoundary(rows[start.Row], start.Col, false),
			End:   terminalCellBoundary(rows[end.Row], end.Col, true),
		}
		pane.Selection = &span
	}
	return pane, truncated, nil
}

func validateTerminalProjection(input TerminalProjectionInput) error {
	if input.ProviderID == 0 || input.Generation == 0 || !input.RootID.Valid() || !input.PaneID.Valid() ||
		input.RootID.Kind != NodeKindWindow || input.PaneID.Kind != NodeKindPane || input.RootID.Projection != input.PaneID.Projection ||
		input.Cols <= 0 || input.Cols > MaxGraphemes || input.Rows <= 0 || input.Rows > MaxTerminalProjectionCells/input.Cols ||
		len(input.Cells) != input.Cols*input.Rows || len(input.Wrapped) != input.Rows ||
		(len(input.VisualToLogical) != 0 && len(input.VisualToLogical) != input.Rows) ||
		!finite(input.OriginX) || !finite(input.OriginY) || !finite(input.CellWidth) || !finite(input.CellHeight) ||
		input.CellWidth <= 0 || input.CellHeight <= 0 || !input.Clip.Valid() || input.DisplayOffset < 0 ||
		!utf8.ValidString(input.RootName) || !utf8.ValidString(input.PaneName) || len(input.RootName) > MaxNodeNameBytes || len(input.PaneName) > MaxNodeNameBytes ||
		len(input.RootName)+len(input.PaneName) > MaxUTF8Bytes {
		return ErrInvalidProjection
	}
	return nil
}

func projectTerminalRow(input TerminalProjectionInput, rowIndex, byteBudget, graphemeBudget int) (terminalRow, bool, error) {
	cells := input.Cells[rowIndex*input.Cols : (rowIndex+1)*input.Cols]
	if err := validateTerminalCells(cells); err != nil {
		return terminalRow{}, false, err
	}
	last := len(cells) - 1
	for last >= 0 && terminalBlank(cells[last]) {
		last--
	}
	inverse, err := terminalLogicalToVisual(input, rowIndex)
	if err != nil {
		return terminalRow{}, false, err
	}
	var text strings.Builder
	chunks := make([]terminalChunk, 0, last+1)
	truncated := false
	for column := 0; column <= last; column++ {
		cell := cells[column]
		if !utf8.ValidString(cell.Text) || strings.ContainsAny(cell.Text, "\r\n") || cell.Span < 0 || cell.Span > input.Cols-column {
			return terminalRow{}, false, ErrInvalidProjection
		}
		if cell.WideContinuation || cell.Text == "" {
			continue
		}
		span := cell.Span
		if span == 0 {
			span = 1
		}
		if len(cell.Text) > byteBudget-text.Len() {
			truncated = true
			break
		}
		start := text.Len()
		text.WriteString(cell.Text)
		chunks = append(chunks, terminalChunk{startByte: start, endByte: text.Len(), startCell: column, endCell: column + span})
	}
	rowText := text.String()
	row := terminalRow{text: rowText, softWrapped: input.Wrapped[rowIndex]}
	iterator := graphemes.FromString(rowText)
	byteOffset := 0
	chunkIndex := 0
	for iterator.Next() {
		clusterText := iterator.Value()
		if len(row.clusters) == graphemeBudget {
			row.text = rowText[:byteOffset]
			truncated = true
			break
		}
		clusterStart, clusterEnd := byteOffset, byteOffset+len(clusterText)
		byteOffset = clusterEnd
		for chunkIndex < len(chunks) && chunks[chunkIndex].endByte <= clusterStart {
			chunkIndex++
		}
		startCell, endCell := input.Cols, 0
		for index := chunkIndex; index < len(chunks) && chunks[index].startByte < clusterEnd; index++ {
			startCell = min(startCell, chunks[index].startCell)
			endCell = max(endCell, chunks[index].endCell)
		}
		if startCell >= endCell {
			return terminalRow{}, false, ErrInvalidProjection
		}
		row.clusters = append(row.clusters, terminalCluster{
			text: clusterText, startCell: startCell, endCell: endCell,
			bounds: terminalClusterBounds(input, rowIndex, startCell, endCell, inverse),
		})
	}
	return row, truncated, nil
}

func validateTerminalCells(cells []TerminalCell) error {
	coveredUntil := 0
	for column, cell := range cells {
		if !utf8.ValidString(cell.Text) || strings.ContainsAny(cell.Text, "\r\n") || cell.Span < 0 || cell.Span > len(cells)-column || (cell.TrimBarrier && (cell.Text != "" || cell.Span != 0 || cell.Blank)) {
			return ErrInvalidProjection
		}
		if column < coveredUntil {
			if !cell.WideContinuation || cell.Text != "" || cell.Span != 0 {
				return ErrInvalidProjection
			}
			continue
		}
		if cell.WideContinuation || (cell.Blank && cell.Text != "" && cell.Text != " ") {
			return ErrInvalidProjection
		}
		span := cell.Span
		if span == 0 {
			span = 1
		}
		coveredUntil = column + span
		for continuation := column + 1; continuation < coveredUntil; continuation++ {
			if !cells[continuation].WideContinuation {
				return ErrInvalidProjection
			}
		}
	}
	return nil
}

func terminalBlank(cell TerminalCell) bool {
	if cell.TrimBarrier {
		return false
	}
	return cell.WideContinuation || cell.Text == "" || cell.Blank
}

func terminalLogicalToVisual(input TerminalProjectionInput, row int) ([]int, error) {
	inverse := make([]int, input.Cols)
	order := []int(nil)
	if len(input.VisualToLogical) != 0 {
		order = input.VisualToLogical[row]
	}
	if order == nil {
		for column := range inverse {
			inverse[column] = column
		}
		return inverse, nil
	}
	if len(order) != input.Cols {
		return nil, ErrInvalidProjection
	}
	seen := make([]bool, input.Cols)
	for visual, logical := range order {
		if logical < 0 || logical >= input.Cols || seen[logical] {
			return nil, ErrInvalidProjection
		}
		seen[logical] = true
		inverse[logical] = visual
	}
	return inverse, nil
}

func terminalClusterBounds(input TerminalProjectionInput, row, startCell, endCell int, inverse []int) Rect {
	var union Rect
	have := false
	for logical := startCell; logical < endCell && logical < len(inverse); logical++ {
		visual := inverse[logical]
		rect := Rect{
			X:     input.OriginX + float64(visual)*input.CellWidth,
			Y:     input.OriginY + float64(row)*input.CellHeight,
			Width: input.CellWidth, Height: input.CellHeight,
		}
		rect, visible := intersectTerminalRect(rect, input.Clip)
		if !visible {
			continue
		}
		if !have {
			union, have = rect, true
		} else {
			union = unionTerminalRect(union, rect)
		}
	}
	if !have {
		return Rect{X: input.Clip.X, Y: input.Clip.Y}
	}
	return union
}

func intersectTerminalRect(rect, clip Rect) (Rect, bool) {
	left := maxFloat(rect.X, clip.X)
	top := maxFloat(rect.Y, clip.Y)
	right := minFloat(rect.X+rect.Width, clip.X+clip.Width)
	bottom := minFloat(rect.Y+rect.Height, clip.Y+clip.Height)
	if right <= left || bottom <= top {
		return Rect{}, false
	}
	return Rect{X: left, Y: top, Width: right - left, Height: bottom - top}, true
}

func unionTerminalRect(left, right Rect) Rect {
	x := minFloat(left.X, right.X)
	y := minFloat(left.Y, right.Y)
	farX := maxFloat(left.X+left.Width, right.X+right.Width)
	farY := maxFloat(left.Y+left.Height, right.Y+right.Height)
	return Rect{X: x, Y: y, Width: farX - x, Height: farY - y}
}

func assignTerminalRowOffsets(rows []terminalRow) {
	offset := 0
	for index := range rows {
		rows[index].start = offset
		offset += len(rows[index].clusters)
		rows[index].end = offset
		if index+1 < len(rows) && !rows[index].softWrapped {
			offset++
		}
	}
}

func terminalCellBoundary(row terminalRow, column int, after bool) int {
	if column < 0 {
		column = 0
	}
	for index, cluster := range row.clusters {
		if column < cluster.startCell {
			return row.start + index
		}
		if column >= cluster.startCell && column < cluster.endCell {
			if after {
				return row.start + index + 1
			}
			return row.start + index
		}
	}
	return row.end
}

func normalizeTerminalSelection(selection TerminalSelection) TerminalSelection {
	if selection.End.Row < selection.Start.Row || (selection.End.Row == selection.Start.Row && selection.End.Col < selection.Start.Col) {
		selection.Start, selection.End = selection.End, selection.Start
	}
	return selection
}

func clampTerminalPoint(point CellPoint, columns, rows int) CellPoint {
	if point.Row < 0 {
		point.Row = 0
	} else if point.Row >= rows {
		point.Row = rows - 1
	}
	if point.Col < 0 {
		point.Col = 0
	} else if point.Col >= columns {
		point.Col = columns - 1
	}
	return point
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}
func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}
