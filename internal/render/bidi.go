package render

import (
	"cervterm/internal/core"

	"golang.org/x/text/unicode/bidi"
)

type bidiUnit struct {
	rune  rune
	cells []int
}

func VisualOrder(cells []core.Cell) []int {
	order, _ := visualOrder(cells)
	return order
}

func visualOrder(cells []core.Cell) ([]int, bool) {
	identity := make([]int, len(cells))
	for i := range identity {
		identity[i] = i
	}
	if !containsRTL(cells) {
		return identity, true
	}
	units := bidiUnits(cells)
	runes := make([]rune, len(units))
	for i := range units {
		runes[i] = units[i].rune
	}
	var paragraph bidi.Paragraph
	if _, err := paragraph.SetString(string(runes), bidi.DefaultDirection(bidi.LeftToRight)); err != nil {
		return identity, false
	}
	ordering, err := paragraph.Order()
	if err != nil {
		return identity, false
	}
	visual := make([]int, 0, len(cells))
	for i := 0; i < ordering.NumRuns(); i++ {
		run := ordering.Run(i)
		start, end := run.Pos()
		if run.Direction() == bidi.RightToLeft {
			for unit := end; unit >= start; unit-- {
				visual = append(visual, units[unit].cells...)
			}
			continue
		}
		for unit := start; unit <= end; unit++ {
			visual = append(visual, units[unit].cells...)
		}
	}
	if len(visual) != len(cells) {
		return identity, false
	}
	return visual, false
}

func bidiUnits(cells []core.Cell) []bidiUnit {
	units := make([]bidiUnit, 0, len(cells))
	for i := 0; i < len(cells); i++ {
		cell := cells[i]
		r := cell.Rune
		if r == 0 {
			r = ' '
		}
		unit := bidiUnit{rune: r, cells: []int{i}}
		if !cell.WideContinuation && i+1 < len(cells) && cells[i+1].WideContinuation {
			unit.cells = append(unit.cells, i+1)
			i++
		}
		units = append(units, unit)
	}
	return units
}

func containsRTL(cells []core.Cell) bool {
	for _, cell := range cells {
		property, _ := bidi.LookupRune(cell.Rune)
		switch property.Class() {
		case bidi.R, bidi.AL:
			return true
		}
	}
	return false
}

func InversePermutation(order []int) []int {
	inverse := make([]int, len(order))
	for visual, logical := range order {
		if logical >= 0 && logical < len(inverse) {
			inverse[logical] = visual
		}
	}
	return inverse
}
