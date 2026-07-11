package render

import (
	"reflect"
	"testing"

	"cervterm/internal/core"
)

func TestVisualOrder(t *testing.T) {
	tests := []struct {
		name  string
		cells []core.Cell
		want  []int
	}{
		{"pure LTR", cellsFromString("abc"), []int{0, 1, 2}},
		{"pure RTL Hebrew", cellsFromString("שלום"), []int{3, 2, 1, 0}},
		{"mixed", cellsFromString("abc שלום xyz"), []int{0, 1, 2, 3, 7, 6, 5, 4, 8, 9, 10, 11}},
		{"numbers in RTL", cellsFromString("שלום 123"), []int{4, 3, 2, 1, 0, 5, 6, 7}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VisualOrder(tt.cells); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("VisualOrder() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVisualOrderKeepsWidePairAdjacent(t *testing.T) {
	cells := []core.Cell{{Rune: 'א'}, {Rune: '界'}, {WideContinuation: true}, {Rune: 'ב'}}
	order := VisualOrder(cells)
	for i := 0; i+1 < len(order); i++ {
		if order[i] == 1 && order[i+1] == 2 {
			return
		}
	}
	t.Fatalf("wide pair is not adjacent and ordered: %v", order)
}

func TestVisualOrderFastPath(t *testing.T) {
	order, fast := visualOrder(cellsFromString("plain ASCII"))
	if !fast {
		t.Fatal("LTR row did not take identity fast path")
	}
	for i, logical := range order {
		if i != logical {
			t.Fatalf("identity[%d] = %d", i, logical)
		}
	}
}

func TestVisualOrderKeepsCellData(t *testing.T) {
	cells := cellsFromString("אב")
	cells[0].Attr.Bold = true
	cells[0].Combining = []rune{'\u05b0'}
	order := VisualOrder(cells)
	visual := []core.Cell{cells[order[0]], cells[order[1]]}
	if visual[1].Rune != 'א' || !visual[1].Attr.Bold || len(visual[1].Combining) != 1 {
		t.Fatalf("cell attributes or combining marks did not travel: %#v", visual)
	}
}

func TestInversePermutationRoundTrip(t *testing.T) {
	order := VisualOrder(cellsFromString("abc שלום"))
	inverse := InversePermutation(order)
	for visual, logical := range order {
		if inverse[logical] != visual {
			t.Fatalf("inverse[%d] = %d, want %d", logical, inverse[logical], visual)
		}
	}
}

func cellsFromString(value string) []core.Cell {
	cells := make([]core.Cell, 0, len([]rune(value)))
	for _, r := range value {
		cells = append(cells, core.Cell{Rune: r})
	}
	return cells
}
