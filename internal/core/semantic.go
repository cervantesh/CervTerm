package core

const (
	MaxSemanticZones         = 4096
	MaxSemanticHistoryCells  = 1 << 20
	MaxSemanticHistoryRanges = 4096
)

type SemanticKind uint8

const (
	SemanticNone SemanticKind = iota
	SemanticPrompt
	SemanticInput
	SemanticOutput
	semanticBoundaryMask SemanticKind = 0x80
)

type SemanticZone struct {
	Kind       SemanticKind
	Start, End int // visible-cell offsets; End is exclusive
}

func (t *Terminal) SetSemanticKind(kind SemanticKind) bool {
	if kind > SemanticOutput {
		return false
	}
	t.semanticKind = kind
	t.semanticBoundaryPending = kind != SemanticNone
	return true
}

func (t *Terminal) SemanticKind() SemanticKind { return t.semanticKind }

func SemanticCellKind(cell Cell) SemanticKind { return cell.SemanticKind &^ semanticBoundaryMask }
func SemanticCellStartsRange(cell Cell) bool  { return cell.SemanticKind&semanticBoundaryMask != 0 }

func (t *Terminal) consumeSemanticCell() SemanticKind {
	kind := t.semanticKind
	if t.semanticBoundaryPending && kind != SemanticNone {
		kind |= semanticBoundaryMask
		t.semanticBoundaryPending = false
	}
	return kind
}

func ProjectSemanticZones(cells []Cell, dst []SemanticZone) ([]SemanticZone, bool) {
	dst = dst[:0]
	for index := 0; index < len(cells); {
		kind := SemanticCellKind(cells[index])
		if kind == SemanticNone {
			index++
			continue
		}
		end := index + 1
		for end < len(cells) && SemanticCellKind(cells[end]) == kind && !SemanticCellStartsRange(cells[end]) {
			end++
		}
		if len(dst) == MaxSemanticZones {
			return dst, true
		}
		dst = append(dst, SemanticZone{Kind: kind, Start: index, End: end})
		index = end
	}
	return dst, false
}

type SemanticPoint struct{ GlobalRow, Col int }

type SemanticRange struct {
	Kind           SemanticKind
	Start, End     SemanticPoint // End is exclusive on its physical row
	StartsAtMarker bool
}

func (t *Terminal) semanticPhysicalRow(global int) []Cell {
	if global < 0 || global >= t.scrollbackRows+t.rows {
		return nil
	}
	if global < t.scrollbackRows {
		source := (t.scrollbackStart + global) % t.scrollbackCapacity
		return t.scrollback[source*t.cols : (source+1)*t.cols]
	}
	row := global - t.scrollbackRows
	return t.cells[row*t.cols : (row+1)*t.cols]
}

func (t *Terminal) semanticPhysicalRowWrapped(global int) bool {
	if global < 0 || global >= t.scrollbackRows+t.rows {
		return false
	}
	if global < t.scrollbackRows {
		source := (t.scrollbackStart + global) % t.scrollbackCapacity
		return len(t.scrollbackWrapped) == t.scrollbackCapacity && t.scrollbackWrapped[source]
	}
	row := global - t.scrollbackRows
	return row >= 0 && row < len(t.rowWrapped) && t.rowWrapped[row]
}

func (t *Terminal) SemanticHistory() (ranges []SemanticRange, truncated bool) {
	totalRows := t.scrollbackRows + t.rows
	if totalRows == 0 || t.cols <= 0 {
		return nil, false
	}
	if t.cols > MaxSemanticHistoryCells {
		return nil, true
	}
	oldest := 0
	maxRows := max(1, MaxSemanticHistoryCells/t.cols)
	if totalRows > maxRows {
		oldest = totalRows - maxRows
		truncated = true
	}
	// Scan newest-to-oldest directly from the ring/live rows; no pre-budget clone.
scan:
	for global := totalRows - 1; global >= oldest; global-- {
		row := t.semanticPhysicalRow(global)
		for col := len(row) - 1; col >= 0; {
			kind := SemanticCellKind(row[col])
			if kind == SemanticNone {
				col--
				continue
			}
			end := col + 1
			startMarker := false
			for col >= 0 && SemanticCellKind(row[col]) == kind {
				startMarker = SemanticCellStartsRange(row[col])
				col--
				if startMarker {
					break
				}
			}
			start := col + 1
			if len(ranges) > 0 && ranges[len(ranges)-1].Kind == kind && ranges[len(ranges)-1].Start.GlobalRow == global+1 && !ranges[len(ranges)-1].StartsAtMarker {
				ranges[len(ranges)-1].Start = SemanticPoint{global, start}
				ranges[len(ranges)-1].StartsAtMarker = startMarker
			} else {
				if len(ranges) == MaxSemanticHistoryRanges {
					truncated = true
					break scan
				}
				ranges = append(ranges, SemanticRange{Kind: kind, Start: SemanticPoint{global, start}, End: SemanticPoint{global, end}, StartsAtMarker: startMarker})
			}
		}
	}
	for left, right := 0, len(ranges)-1; left < right; left, right = left+1, right-1 {
		ranges[left], ranges[right] = ranges[right], ranges[left]
	}
	return ranges, truncated
}

func (t *Terminal) stampBlankSemanticRow() {
	if t.semanticKind == SemanticNone || t.cursorRow < 0 || t.cursorRow >= t.rows || t.cols == 0 {
		return
	}
	start := t.cursorRow * t.cols
	row := t.cells[start : start+t.cols]
	for _, cell := range row {
		if (cell.Rune != 0 && cell.Rune != ' ') || cell.HasCombining() || SemanticCellKind(cell) != SemanticNone {
			return
		}
	}
	row[0].Rune = 0 // metadata-only blank-line sentinel; never rendered or copied as text
	row[0].SemanticKind = t.consumeSemanticCell()
}
