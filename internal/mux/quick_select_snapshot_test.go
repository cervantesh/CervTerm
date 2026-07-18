package mux

import (
	"strings"
	"testing"
)

func TestQuickSelectSnapshotIsBoundedDetachedAndGenerationChecked(t *testing.T) {
	m, session, wakes := newTestMux(t)
	initial, ok := m.QuickSelectSnapshot(1, 1, 10)
	if !ok {
		t.Fatal("snapshot")
	}
	if initial.Rows > 1 || len(initial.Cells) > 10 {
		t.Fatalf("bounds=%#v", initial)
	}
	if err := session.feed([]byte("hello\r\nworld")); err != nil {
		t.Fatal(err)
	}
	awaitWake(t, wakes)
	m.Drain(16)
	if m.QuickSelectSnapshotCurrent(initial) {
		t.Fatal("output did not stale snapshot")
	}
	after, ok := m.QuickSelectSnapshot(1, 0, 0)
	if !ok || after.ContentGen <= initial.ContentGen {
		t.Fatalf("after=%#v", after)
	}
	if len(after.Cells) > 0 {
		original := after.Cells[0].Rune
		after.Cells[0].Rune = 'Z'
		again, _ := m.QuickSelectSnapshot(1, 0, 0)
		if again.Cells[0].Rune != original {
			t.Fatal("snapshot aliases terminal cells")
		}
	}
}

func TestQuickSelectGenerationsTrackResizeAndViewport(t *testing.T) {
	m, session, wakes := newTestMux(t)
	if err := session.feed([]byte(strings.Repeat("line\r\n", 80))); err != nil {
		t.Fatal(err)
	}
	awaitWake(t, wakes)
	m.Drain(256)
	before, _ := m.QuickSelectSnapshot(1, 0, 0)
	moved, err := m.ScrollViewport(1, 5)
	if err != nil || !moved {
		t.Fatalf("scroll moved=%v err=%v", moved, err)
	}
	scrolled, _ := m.QuickSelectSnapshot(1, 0, 0)
	if scrolled.ViewportGen <= before.ViewportGen || m.QuickSelectSnapshotCurrent(before) {
		t.Fatalf("viewport generations before=%#v after=%#v", before, scrolled)
	}
	if _, err := m.Resize(PixelRect{Width: 640, Height: 400}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		t.Fatal(err)
	}
	resized, _ := m.QuickSelectSnapshot(1, 0, 0)
	if resized.ReflowGen <= scrolled.ReflowGen || m.QuickSelectSnapshotCurrent(scrolled) {
		t.Fatalf("reflow generations before=%#v after=%#v", scrolled, resized)
	}
}

func TestQuickSelectSnapshotRejectsFocusChange(t *testing.T) {
	m, _, _ := newTestMux(t)
	second, _, err := m.Split(1, SplitColumns, SpawnSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.FocusPane(1); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := m.QuickSelectSnapshot(1, 0, 0)
	if _, err := m.FocusPane(second); err != nil {
		t.Fatal(err)
	}
	if m.QuickSelectSnapshotCurrent(snapshot) {
		t.Fatal("focus change did not stale snapshot")
	}
}
