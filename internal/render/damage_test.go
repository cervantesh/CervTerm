package render

import (
	"testing"

	"cervterm/internal/core"
)

func TestHashRowsOneCellChangeOnlyChangesItsRow(t *testing.T) {
	cells := []core.Cell{{Rune: 'a'}, {Rune: 'b'}, {Rune: 'c'}, {Rune: 'd'}}
	before := make([]uint64, 2)
	HashRows(before, cells, 2)

	cells[2].Rune = 'x'
	after := make([]uint64, 2)
	HashRows(after, cells, 2)

	if before[0] != after[0] {
		t.Fatal("unchanged row hash changed")
	}
	if before[1] == after[1] {
		t.Fatal("changed row hash did not change")
	}
}

func TestHashRowsAttributeOnlyChangeCounts(t *testing.T) {
	cells := []core.Cell{{Rune: 'a'}}
	before := make([]uint64, 1)
	HashRows(before, cells, 1)
	cells[0].Attr.Bold = true
	after := make([]uint64, 1)
	HashRows(after, cells, 1)
	if before[0] == after[0] {
		t.Fatal("attribute-only change did not change row hash")
	}
}

func TestHashRowsEqualRowsHaveEqualHashes(t *testing.T) {
	row := []core.Cell{{Rune: 'a', Combining: []rune{'\u0301'}, Attr: core.Attr{Italic: true}}, {Rune: 'b'}}
	cells := append(append([]core.Cell{}, row...), row...)
	hashes := make([]uint64, 2)
	HashRows(hashes, cells, 2)
	if hashes[0] != hashes[1] {
		t.Fatalf("equal rows hashed differently: %x != %x", hashes[0], hashes[1])
	}
}

func TestHashRowsWritesIntoDestination(t *testing.T) {
	dst := make([]uint64, 2, 8)
	backing := &dst[0]
	cells := []core.Cell{{Rune: 'a'}, {Rune: 'b'}}
	HashRows(dst, cells, 1)
	if &dst[0] != backing {
		t.Fatal("HashRows replaced destination backing store")
	}
	if dst[0] == 0 || dst[1] == 0 {
		t.Fatal("HashRows did not populate destination")
	}
	if allocs := testing.AllocsPerRun(100, func() { HashRows(dst, cells, 1) }); allocs != 0 {
		t.Fatalf("HashRows allocated %.0f times, want zero", allocs)
	}
}
