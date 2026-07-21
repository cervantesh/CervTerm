package core

import (
	"testing"

	"cervterm/internal/termimage"
)

func TestImagePrimaryReflowMapsTopLeftAndPreservesSpan(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(6, 3, 4, store)
	for _, r := range "abcdefghijkl" {
		terminal.PutRune(r)
	}
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 1}, Cols: 2, Rows: 2})
	terminal.Resize(3, 5)
	placement := primaryPlacementByID(t, terminal, 1)
	if placement.Anchor.Row != 2 || placement.Anchor.Col != 0 || placement.Cols != 2 || placement.Rows != 2 {
		t.Fatalf("narrow placement=%#v", placement)
	}
	terminal.Resize(6, 3)
	placement = primaryPlacementByID(t, terminal, 1)
	if placement.Anchor.Row != 1 || placement.Anchor.Col != 0 {
		t.Fatalf("wide placement=%#v", placement)
	}
}

func TestImagePrimaryReflowEvictionReleasesPlacementNotResource(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 3, 0, store)
	for _, r := range "abcdefghijkl" {
		terminal.PutRune(r)
	}
	ref := placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 0}, Cols: 1, Rows: 1})
	terminal.Resize(2, 1)
	if len(terminal.imageSidecars.primary) != 0 || store.Usage().Placements != 0 {
		t.Fatal("evicted reflow anchor retained")
	}
	if _, ok := store.Acquire(ref); !ok {
		t.Fatal("reflow eviction removed reusable resource")
	}
}

func TestImageAlternateResizeCropsAnchorOnlyAndExitDiscards(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(6, 4, 2, store)
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 0}, Cols: 1, Rows: 1})
	terminal.SetAlternateScreenMode(true)
	placeLifecycleImage(t, terminal, 2, termimage.PlacementSpec{ID: 2, Anchor: termimage.CellAnchor{Row: 1, Col: 2}, Cols: 4, Rows: 4})
	placeLifecycleImage(t, terminal, 3, termimage.PlacementSpec{ID: 3, Anchor: termimage.CellAnchor{Row: 3, Col: 5}, Cols: 1, Rows: 1})
	terminal.Resize(3, 2)
	if len(terminal.imageSidecars.alternate) != 1 || terminal.imageSidecars.alternate[0].placement.ID != 2 {
		t.Fatal("alternate crop did not use top-left policy")
	}
	kept := terminal.imageSidecars.alternate[0].placement
	if kept.Cols != 4 || kept.Rows != 4 {
		t.Fatal("alternate crop changed span")
	}
	terminal.SetAlternateScreenMode(false)
	if len(terminal.imageSidecars.alternate) != 0 || len(terminal.imageSidecars.primary) != 1 {
		t.Fatal("alternate exit did not discard/preserve isolated placements")
	}
	if usage := store.Usage(); usage.Placements != 1 || usage.Images != 3 {
		t.Fatalf("exit usage=%#v", usage)
	}
}

func TestReflowMapRejectsMissingAndMapsPhysicalCells(t *testing.T) {
	source := [][]Cell{{{Rune: 'a'}, {Rune: 'b'}, {Rune: 'c'}}, {{Rune: 'd'}}}
	mapping := newReflowMap(source, []bool{true, false}, [][]Cell{{{Rune: 'a'}, {Rune: 'b'}}, {{Rune: 'c'}, {Rune: 'd'}}}, []bool{true, false})
	row, col, ok := mapping.mapCell(1, 0)
	if !ok || row != 1 || col != 1 {
		t.Fatalf("mapped=%d,%d,%v", row, col, ok)
	}
	if _, _, ok := mapping.mapCell(9, 0); ok {
		t.Fatal("missing source mapped")
	}
}

func TestImagePrimaryReflowRetainsPaddingAnchorByClamping(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(6, 2, 2, store)
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 0, Col: 5}, Cols: 1, Rows: 1})
	terminal.Resize(3, 2)
	placement := primaryPlacementByID(t, terminal, 1)
	if placement.Anchor.Row != 0 || placement.Anchor.Col != 2 {
		t.Fatalf("padding placement=%#v", placement)
	}
}

func TestImagePrimaryReflowCursorAndPlacementShareMapping(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(6, 3, 3, store)
	for _, r := range "abcdefgh" {
		terminal.PutRune(r)
	}
	globalRow := terminal.scrollbackRows + terminal.cursorRow
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: int64(globalRow), Col: uint32(terminal.cursorCol)}, Cols: 1, Rows: 1})
	terminal.Resize(3, 5)
	placement := primaryPlacementByID(t, terminal, 1)
	if placement.Anchor.Row != int64(terminal.scrollbackRows+terminal.cursorRow) || placement.Anchor.Col != uint32(terminal.cursorCol) {
		t.Fatalf("cursor=%d,%d placement=%#v", terminal.cursorRow, terminal.cursorCol, placement)
	}
}

func TestImagePrimaryRepeatedNarrowWideReflowDeterministic(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(8, 4, 8, store)
	for _, r := range "abcdefghijklmnop" {
		terminal.PutRune(r)
	}
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 1, Col: 2}, Cols: 2, Rows: 1})
	for cycle := 0; cycle < 10; cycle++ {
		terminal.Resize(4, 6)
		terminal.Resize(8, 4)
		placement := primaryPlacementByID(t, terminal, 1)
		if placement.Anchor.Row != 1 || placement.Anchor.Col != 2 {
			t.Fatalf("cycle %d placement=%#v", cycle, placement)
		}
	}
}

func TestImageHistoryLiveStraddleReflowPreservesBothSides(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 4, store)
	for _, r := range "abcdefgh" {
		terminal.PutRune(r)
	}
	terminal.scrollUpRegion(0, 1, 1)
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 0, Col: 1}, Cols: 1, Rows: 1})
	placeLifecycleImage(t, terminal, 2, termimage.PlacementSpec{ID: 2, Anchor: termimage.CellAnchor{Row: int64(terminal.scrollbackRows), Col: 1}, Cols: 1, Rows: 1})
	terminal.Resize(3, 3)
	if len(terminal.imageSidecars.primary) != 2 {
		t.Fatalf("straddle placements=%#v", terminal.imageSidecars.primary)
	}
	first, second := primaryPlacementByID(t, terminal, 1), primaryPlacementByID(t, terminal, 2)
	if first.Anchor.Row > second.Anchor.Row {
		t.Fatalf("history/live order reversed: %#v %#v", first, second)
	}
}
