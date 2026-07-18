package modal

import "testing"

func TestClipCellsPreservesUnicodeClusters(t *testing.T) {
	cases := []struct {
		text string
		cols int
		want string
	}{
		{"a界b", 3, "a界"},
		{"e\u0301x", 1, "e\u0301"},
		{"👩\u200d💻x", 2, "👩\u200d💻"},
		{"🇺🇸x", 1, "🇺🇸"},
	}
	for _, tc := range cases {
		if got := ClipCells(tc.text, tc.cols); got != tc.want {
			t.Errorf("ClipCells(%q, %d) = %q, want %q", tc.text, tc.cols, got, tc.want)
		}
	}
}

func TestListLayoutBoundedAndDamageRevisionBased(t *testing.T) {
	var c Coordinator
	c.Open(ModeCommandPalette, 1, 1, entries("zero", "one", "two", "three"))
	c.Move(3)
	s := c.Snapshot()
	geometry := LayoutGeometry{Columns: 4, Rows: 4, VisibleRows: 2}
	layout := ListLayout(s, geometry)
	if len(layout.Commands) > MaxDrawCommands || layout.Scroll != 2 {
		t.Fatalf("layout commands=%d scroll=%d", len(layout.Commands), layout.Scroll)
	}
	before := SnapshotDamage(s, geometry)
	if before.Changed(SnapshotDamage(c.Snapshot(), geometry)) {
		t.Fatal("unchanged retained state reports damage")
	}
	c.Backspace() // empty query is unchanged
	if before.Changed(SnapshotDamage(c.Snapshot(), geometry)) {
		t.Fatal("no-op edit reports damage")
	}
	c.AppendRune('o')
	if !before.Changed(SnapshotDamage(c.Snapshot(), geometry)) {
		t.Fatal("mutation did not report damage")
	}
}
