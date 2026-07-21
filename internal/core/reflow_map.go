package core

// reflowMap maps physical cell anchors through the exact logical stream used by
// terminal reflow. Callers decide whether a mapped anchor remains retained.
type reflowMap struct {
	source, target               [][]Cell
	sourceWrapped, targetWrapped []bool
}

func newReflowMap(source [][]Cell, sourceWrapped []bool, target [][]Cell, targetWrapped []bool) reflowMap {
	return reflowMap{source: source, sourceWrapped: sourceWrapped, target: target, targetWrapped: targetWrapped}
}

func (mapping reflowMap) mapCell(row, col int) (int, int, bool) {
	if row < 0 || row >= len(mapping.source) || col < 0 || len(mapping.target) == 0 {
		return 0, 0, false
	}
	line, rowStart := physicalAnchor(mapping.source, mapping.sourceWrapped, row)
	logicalChar := rowStart + col
	targetRow := physicalForAnchor(mapping.target, mapping.targetWrapped, line, logicalChar)
	if targetRow < 0 || targetRow >= len(mapping.target) {
		return 0, 0, false
	}
	mappedLine, targetStart := physicalAnchor(mapping.target, mapping.targetWrapped, targetRow)
	if mappedLine != line || logicalChar < targetStart {
		return 0, 0, false
	}
	return targetRow, logicalChar - targetStart, true
}
