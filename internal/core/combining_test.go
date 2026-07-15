package core

import (
	"testing"
	"unsafe"
)

// TestCellSizeBudget pins the shrink win from docs/cell-memory-plan.md: Cell is
// held by the live grid, the scrollback ring, and every render snapshot, and is
// scanned in full each frame — a field reorder (or a stray large field) that
// re-bloats it must fail here.
func TestCellSizeBudget(t *testing.T) {
	if got := unsafe.Sizeof(Cell{}); got > 32 {
		t.Fatalf("Cell is %d bytes; keep it <= 32 (see docs/cell-memory-plan.md)", got)
	}
}

// TestAppendCombiningCopyOnWrite is the distinguishing COW test that is only
// observable now that combining is a *[]rune: a value copy shares the pointer,
// so without copy-on-write a live append would mutate the copy. (Reverting
// AppendCombining to `*c.combining = append(*c.combining, r)` fails this.)
func TestAppendCombiningCopyOnWrite(t *testing.T) {
	live := NewCellWithCombining('e', Attr{}, 'a')
	snap := live // value copy — shares the *[]rune pointer

	live.AppendCombining('b')

	if got := snap.Combining(); len(got) != 1 || got[0] != 'a' {
		t.Fatalf("copy-on-write violated: snapshot observed the live append: %q", got)
	}
	if got := live.Combining(); len(got) != 2 || got[1] != 'b' {
		t.Fatalf("live append lost: %q", got)
	}
}

// TestCombiningSnapshotFrozenAfterAppend drives the real capture path: CopyView
// copies cells shallowly (sharing the combining backing), so a later combining
// mark added to the same live cell must not reach an already-taken snapshot.
//
// In Phase A (combining is a value []rune) this holds via slice value semantics.
// It is the guard Phase B must keep: once combining becomes *[]rune, a value
// copy shares the pointer, and AppendCombining must copy-on-write or this test
// fails. See docs/cell-memory-plan.md, traps 1 & 2.
func TestCombiningSnapshotFrozenAfterAppend(t *testing.T) {
	term := NewTerminal(10, 2)
	term.PutRune('e')
	term.PutRune('́') // combining acute -> cell 0 carries one mark

	snap := make([]Cell, term.Cols()*term.Rows())
	term.CopyView(snap) // shallow copy, exactly as render.Capture does
	if len(snap[0].Combining()) != 1 {
		t.Fatalf("setup: snapshot should capture 1 mark, got %d", len(snap[0].Combining()))
	}

	term.PutRune('̂') // stack a second mark on the SAME live cell

	if got := snap[0].Combining(); len(got) != 1 || got[0] != '́' {
		t.Fatalf("snapshot combining mutated by a later live append: %q", got)
	}
	live := make([]Cell, term.Cols()*term.Rows())
	term.CopyView(live)
	if len(live[0].Combining()) != 2 {
		t.Fatalf("live cell should carry both marks, got %d", len(live[0].Combining()))
	}
}

func TestCellCombiningAccessors(t *testing.T) {
	var blank Cell
	if blank.HasCombining() || blank.Combining() != nil || blank.CloneCombining() != nil {
		t.Fatalf("zero cell should have no combining marks")
	}
	c := NewCellWithCombining('a', Attr{Bold: true}, 'x', 'y')
	if !c.HasCombining() || len(c.Combining()) != 2 {
		t.Fatalf("constructor did not attach marks: %#v", c.Combining())
	}
	// CloneCombining must be independent of the source.
	clone := c.CloneCombining()
	clone[0] = 'z'
	if c.Combining()[0] != 'x' {
		t.Fatalf("CloneCombining aliased the source slice")
	}
}
