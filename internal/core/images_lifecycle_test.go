package core

import (
	"testing"

	"cervterm/internal/termimage"
)

func placeLifecycleImage(t *testing.T, terminal *Terminal, image termimage.ImageID, spec termimage.PlacementSpec) termimage.ResourceRef {
	t.Helper()
	result, err := terminal.commitImage(imageCommit{candidate: decodedCandidateForTest(t, terminal.imageStore, image, 1, 1), placement: &spec})
	if err != nil {
		t.Fatal(err)
	}
	return result.resource
}

func primaryPlacementByID(t *testing.T, terminal *Terminal, id termimage.PlacementID) termimage.Placement {
	t.Helper()
	for _, entry := range terminal.imageSidecars.primary {
		if entry.placement.ID == id {
			return entry.placement
		}
	}
	t.Fatalf("primary placement %d missing", id)
	return termimage.Placement{}
}

func TestImageLifecycleOverwriteAndEraseDeleteWholePlacement(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(8, 3, 2, store)
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 0, Col: 2}, Cols: 2, Rows: 2})
	placeLifecycleImage(t, terminal, 2, termimage.PlacementSpec{ID: 2, Anchor: termimage.CellAnchor{Row: 2, Col: 6}, Cols: 1, Rows: 1})
	terminal.SetCursor(0, 3)
	terminal.PutRune('x')
	if len(terminal.imageSidecars.primary) != 1 || terminal.imageSidecars.primary[0].placement.ID != 2 {
		t.Fatal("overwrite did not delete whole intersected placement")
	}
	if usage := store.Usage(); usage.Placements != 1 || usage.Images != 2 {
		t.Fatalf("overwrite usage=%#v", usage)
	}
	terminal.SetCursor(2, 6)
	terminal.EraseChars(1)
	if len(terminal.imageSidecars.primary) != 0 || store.Usage().Placements != 0 {
		t.Fatal("ECH retained placement")
	}
}

func TestImageLifecycleEraseDisplaySeparatesLiveAndHistory(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(6, 2, 3, store)
	terminal.scrollUpRegion(0, 1, 1)
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 0}, Cols: 1, Rows: 1})
	placeLifecycleImage(t, terminal, 2, termimage.PlacementSpec{ID: 2, Anchor: termimage.CellAnchor{Row: 1}, Cols: 1, Rows: 1})
	terminal.Clear()
	if len(terminal.imageSidecars.primary) != 1 || terminal.imageSidecars.primary[0].placement.ID != 1 {
		t.Fatal("ED2 did not preserve history-only placement")
	}
	terminal.ClearScrollback()
	if len(terminal.imageSidecars.primary) != 0 || store.Usage().Placements != 0 {
		t.Fatal("ED3 did not clear history placement")
	}
}

func TestImageLifecycleInsertDeleteCharacters(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(8, 3, 0, store)
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 0, Col: 4}, Cols: 1, Rows: 1})
	placeLifecycleImage(t, terminal, 2, termimage.PlacementSpec{ID: 2, Anchor: termimage.CellAnchor{Row: 0, Col: 1}, Cols: 2, Rows: 1})
	placeLifecycleImage(t, terminal, 3, termimage.PlacementSpec{ID: 3, Anchor: termimage.CellAnchor{Row: 0, Col: 5}, Cols: 1, Rows: 2})
	terminal.SetCursor(0, 2)
	terminal.InsertChars(1)
	if got := primaryPlacementByID(t, terminal, 1).Anchor.Col; got != 5 {
		t.Fatalf("insert shifted col=%d", got)
	}
	if len(terminal.imageSidecars.primary) != 1 {
		t.Fatal("insert boundaries did not retire crossing placements")
	}
	terminal.SetCursor(0, 2)
	terminal.DeleteChars(2)
	if got := primaryPlacementByID(t, terminal, 1).Anchor.Col; got != 3 {
		t.Fatalf("delete shifted col=%d", got)
	}
	if len(terminal.imageSidecars.primary) != 1 || store.Usage().Placements != 1 {
		t.Fatal("delete boundary accounting mismatch")
	}
}

func TestImageLifecyclePartialScrollMovesContainedAndDeletesCrossing(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(5, 4, 0, store)
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 2}, Cols: 1, Rows: 1})
	placeLifecycleImage(t, terminal, 2, termimage.PlacementSpec{ID: 2, Anchor: termimage.CellAnchor{Row: 1, Col: 2}, Cols: 1, Rows: 2})
	placeLifecycleImage(t, terminal, 3, termimage.PlacementSpec{ID: 3, Anchor: termimage.CellAnchor{Row: 0, Col: 3}, Cols: 1, Rows: 1})
	terminal.scrollUpRegion(1, 3, 1)
	if got := primaryPlacementByID(t, terminal, 1).Anchor.Row; got != 1 {
		t.Fatalf("partial up row=%d", got)
	}
	if len(terminal.imageSidecars.primary) != 2 {
		t.Fatal("partial up did not delete boundary crossing")
	}
	terminal.scrollDownRegion(1, 3, 1)
	if got := primaryPlacementByID(t, terminal, 1).Anchor.Row; got != 2 {
		t.Fatalf("partial down row=%d", got)
	}
	if primaryPlacementByID(t, terminal, 3).Anchor.Row != 0 {
		t.Fatal("outside placement moved")
	}
}

func TestImageLifecycleFullScrollHistoryAndRingEviction(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(5, 3, 2, store)
	removedResource := placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 0}, Cols: 1, Rows: 1})
	placeLifecycleImage(t, terminal, 2, termimage.PlacementSpec{ID: 2, Anchor: termimage.CellAnchor{Row: 2, Col: 1}, Cols: 1, Rows: 1})
	terminal.scrollUpRegion(0, 2, 1)
	terminal.scrollUpRegion(0, 2, 1)
	if primaryPlacementByID(t, terminal, 1).Anchor.Row != 0 || primaryPlacementByID(t, terminal, 2).Anchor.Row != 2 {
		t.Fatal("history growth moved physical anchors")
	}
	terminal.scrollUpRegion(0, 2, 1)
	if len(terminal.imageSidecars.primary) != 1 || primaryPlacementByID(t, terminal, 2).Anchor.Row != 1 {
		t.Fatal("ring eviction did not retire/rebase placements")
	}
	if terminal.scrollbackStart == 0 {
		t.Fatal("test did not wrap history ring")
	}
	if usage := store.Usage(); usage.Placements != 1 {
		t.Fatalf("ring usage=%#v", usage)
	}
	if _, ok := store.Acquire(removedResource); !ok {
		t.Fatal("placement eviction removed reusable resource")
	}
}

func TestImageLifecycleZeroHistoryFullScrollMatchesRegionalDrop(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 3, 0, store)
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 0}, Cols: 1, Rows: 1})
	placeLifecycleImage(t, terminal, 2, termimage.PlacementSpec{ID: 2, Anchor: termimage.CellAnchor{Row: 2}, Cols: 1, Rows: 1})
	terminal.scrollUpRegion(0, 2, 1)
	if len(terminal.imageSidecars.primary) != 1 || primaryPlacementByID(t, terminal, 2).Anchor.Row != 1 {
		t.Fatal("zero-history scroll did not drop/rebase")
	}
}

func TestImageLifecycleCapacityReductionAndED3Rebase(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(5, 3, 4, store)
	for i := 0; i < 3; i++ {
		terminal.scrollUpRegion(0, 2, 1)
	}
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 0}, Cols: 1, Rows: 1})
	placeLifecycleImage(t, terminal, 2, termimage.PlacementSpec{ID: 2, Anchor: termimage.CellAnchor{Row: 2}, Cols: 1, Rows: 2})
	placeLifecycleImage(t, terminal, 3, termimage.PlacementSpec{ID: 3, Anchor: termimage.CellAnchor{Row: 4}, Cols: 1, Rows: 1})
	terminal.SetScrollbackCapacity(1)
	if len(terminal.imageSidecars.primary) != 2 || primaryPlacementByID(t, terminal, 2).Anchor.Row != 0 || primaryPlacementByID(t, terminal, 3).Anchor.Row != 2 {
		t.Fatal("capacity reduction did not retain/rebase newest physical rows")
	}
	terminal.ClearScrollback()
	if len(terminal.imageSidecars.primary) != 1 || primaryPlacementByID(t, terminal, 3).Anchor.Row != 1 {
		t.Fatal("ED3 did not retire crossing placement and rebase live")
	}
}

func TestImageLifecycleCapacityWhileAlternateMutatesPrimaryOnly(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(5, 2, 3, store)
	terminal.scrollUpRegion(0, 1, 2)
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 2}, Cols: 1, Rows: 1})
	terminal.SetAlternateScreenMode(true)
	placeLifecycleImage(t, terminal, 2, termimage.PlacementSpec{ID: 2, Anchor: termimage.CellAnchor{Row: 1}, Cols: 1, Rows: 1})
	terminal.SetScrollbackCapacity(0)
	if primaryPlacementByID(t, terminal, 1).Anchor.Row != 0 {
		t.Fatal("saved primary placement was not rebased")
	}
	if len(terminal.imageSidecars.alternate) != 1 || terminal.imageSidecars.alternate[0].placement.ID != 2 {
		t.Fatal("capacity change altered alternate placement")
	}
}

func TestImageViewportProjectionPreservesScrolledBackCoordinates(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(5, 2, 3, store)
	terminal.scrollUpRegion(0, 1, 2)
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 0}, Cols: 1, Rows: 1})
	placeLifecycleImage(t, terminal, 2, termimage.PlacementSpec{ID: 2, Anchor: termimage.CellAnchor{Row: 2}, Cols: 1, Rows: 1})
	live := terminal.imageViewportPlacements(nil)
	if len(live) != 1 || live[0].ID != 2 || live[0].Anchor.Row != 0 {
		t.Fatalf("live projection=%#v", live)
	}
	if !terminal.ScrollViewport(2) {
		t.Fatal("viewport did not scroll")
	}
	history := terminal.imageViewportPlacements(nil)
	if len(history) != 1 || history[0].ID != 1 || history[0].Anchor.Row != 0 {
		t.Fatalf("history projection=%#v", history)
	}
	generation := terminal.imageSidecars.generation
	terminal.ScrollViewport(-2)
	if terminal.imageSidecars.generation != generation {
		t.Fatal("viewport movement mutated placement state")
	}
}

func TestNilImageLifecycleLeavesTextPathAllocationFree(t *testing.T) {
	terminal := NewTerminalWithHistory(8, 3, 2)
	allocs := testing.AllocsPerRun(1000, func() {
		terminal.SetCursor(0, 0)
		terminal.PutRune('x')
		terminal.EraseChars(1)
		terminal.InsertChars(1)
		terminal.DeleteChars(1)
	})
	if allocs != 0 {
		t.Fatalf("nil image lifecycle allocations=%f", allocs)
	}
}

func TestImageLifecycleVerticalMovementIgnoresHorizontalClip(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(5, 4, 0, store)
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 2, Col: 4}, Cols: 2, Rows: 1})
	terminal.scrollUpRegion(1, 3, 1)
	if got := primaryPlacementByID(t, terminal, 1).Anchor.Row; got != 1 {
		t.Fatalf("clipped placement row=%d", got)
	}
}

func TestImageLifecycleCombiningRuneRetiresFullBaseCell(t *testing.T) {
	tests := []struct {
		name      string
		cols, col int
		base      rune
		imageCol  uint32
		autoWrap  bool
	}{
		{"narrow", 5, 0, 'a', 0, true},
		{"right margin", 3, 2, 'a', 2, true},
		{"right margin no wrap", 3, 2, 'a', 2, false},
		{"wide continuation", 5, 0, '界', 1, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
			terminal := newImageTerminalForTest(test.cols, 2, 0, store)
			terminal.SetAutoWrapMode(test.autoWrap)
			terminal.SetCursor(0, test.col)
			terminal.PutRune(test.base)
			placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 0, Col: test.imageCol}, Cols: 1, Rows: 1})
			terminal.PutRune('\u0301')
			if len(terminal.imageSidecars.primary) != 0 || store.Usage().Placements != 0 {
				t.Fatal("combining-cell mutation retained overlap")
			}
		})
	}
}

func TestImageLifecycleEraseBoundaryTruthTable(t *testing.T) {
	tests := []struct {
		name    string
		col     uint32
		cols    uint16
		removed bool
	}{
		{"left touch", 0, 2, false}, {"left overlap", 1, 2, true}, {"inside", 2, 1, true},
		{"right overlap", 3, 2, true}, {"right touch", 4, 1, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
			terminal := newImageTerminalForTest(6, 2, 0, store)
			placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Col: test.col}, Cols: test.cols, Rows: 1})
			terminal.eraseImageLiveRect(0, 1, 2, 4)
			if (len(terminal.imageSidecars.primary) == 0) != test.removed {
				t.Fatalf("removed=%v", test.removed)
			}
		})
	}
}

func TestImageLifecycleLineEntryPointsAndAlternateScroll(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(5, 4, 0, store)
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 2}, Cols: 1, Rows: 1})
	terminal.SetScrollRegion(1, 3)
	terminal.SetCursor(1, 0)
	terminal.InsertLines(1)
	if primaryPlacementByID(t, terminal, 1).Anchor.Row != 3 {
		t.Fatal("IL did not move placement")
	}
	terminal.DeleteLines(1)
	if primaryPlacementByID(t, terminal, 1).Anchor.Row != 2 {
		t.Fatal("DL did not move placement")
	}
	terminal.SetAlternateScreenMode(true)
	placeLifecycleImage(t, terminal, 2, termimage.PlacementSpec{ID: 2, Anchor: termimage.CellAnchor{Row: 2, Col: 4}, Cols: 2, Rows: 1})
	terminal.ResetScrollRegion()
	terminal.ScrollUp(1)
	if len(terminal.imageSidecars.alternate) != 1 || terminal.imageSidecars.alternate[0].placement.Anchor.Row != 1 {
		t.Fatal("alternate full scroll lost clipped placement")
	}
	terminal.ScrollDown(1)
	if terminal.imageSidecars.alternate[0].placement.Anchor.Row != 2 {
		t.Fatal("alternate scroll down did not move placement")
	}
}

func TestImageLifecycleStalePreparedMutationAbortsOwnership(t *testing.T) {
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	terminal := newImageTerminalForTest(5, 2, 0, store)
	baseSpec := termimage.PlacementSpec{ID: 1, Cols: 1, Rows: 1}
	placeLifecycleImage(t, terminal, 1, baseSpec)
	preparedSpec := termimage.PlacementSpec{ID: 2, Anchor: termimage.CellAnchor{Col: 1}, Cols: 1, Rows: 1}
	prepared, _, err := terminal.prepareImageCommit(imageCommit{candidate: decodedCandidateForTest(t, store, 2, 1, 1), placement: &preparedSpec}, nil)
	if err != nil {
		t.Fatal(err)
	}
	terminal.eraseImageLiveRect(0, 1, 0, 1)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("stale publish did not panic")
			}
		}()
		terminal.publishPreparedImage(prepared)
	}()
	if usage := store.Usage(); usage.Images != 1 || usage.Placements != 0 {
		t.Fatalf("stale abort usage=%#v", usage)
	}
	if process.Usage() != store.Usage() {
		t.Fatalf("process usage=%#v pane=%#v", process.Usage(), store.Usage())
	}
	if candidate, err := store.NewDecodedCandidate(3, 1, 1); err != nil {
		t.Fatal(err)
	} else {
		candidate.Close()
	}
}

func TestImageLifecycleBoundedMaximumPlacementRetirement(t *testing.T) {
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	terminal := newImageTerminalForTest(3, 1, 0, store)
	resource, err := terminal.commitImage(imageCommit{candidate: decodedCandidateForTest(t, store, 1, 1, 1)})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < int(termimage.DefaultLimits().Placements); index++ {
		lease, reserveErr := store.ReservePlacements(1)
		if reserveErr != nil {
			t.Fatal(reserveErr)
		}
		placement, placementErr := termimage.NewPlacement(termimage.PlacementSpec{ID: termimage.PlacementID(index + 1), Cols: 1, Rows: 1}, resource.resource, 1, 1)
		if placementErr != nil {
			t.Fatal(placementErr)
		}
		terminal.imageSidecars.primary = append(terminal.imageSidecars.primary, imagePlacement{placement: placement, lease: lease})
	}
	if store.Usage().Placements != termimage.DefaultLimits().Placements {
		t.Fatal("maximum was not reserved")
	}
	terminal.ClearLine(0)
	if len(terminal.imageSidecars.primary) != 0 || store.Usage().Placements != 0 || store.Usage().Images != 1 {
		t.Fatalf("maximum retirement usage=%#v", store.Usage())
	}
}

func TestImageLifecycleNoOpMutationDoesNotAllocateOrPublish(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(5, 2, 0, store)
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 1, Col: 4}, Cols: 1, Rows: 1})
	generation := terminal.imageSidecars.generation
	allocs := testing.AllocsPerRun(1000, func() { terminal.SetCursor(0, 0); terminal.PutRune('x') })
	if allocs != 0 || terminal.imageSidecars.generation != generation {
		t.Fatalf("no-op allocs=%f generation=%d", allocs, terminal.imageSidecars.generation)
	}
}

func TestImageViewportPinnedOutputAndEviction(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 3, store)
	terminal.scrollUpRegion(0, 1, 2)
	placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 0}, Cols: 1, Rows: 1})
	terminal.ScrollViewport(2)
	terminal.scrollUpRegion(0, 1, 1)
	if projection := terminal.imageViewportPlacements(nil); len(projection) != 1 || projection[0].ID != 1 || projection[0].Anchor.Row != 0 {
		t.Fatalf("pinned projection=%#v", projection)
	}
	terminal.scrollUpRegion(0, 1, 1)
	if projection := terminal.imageViewportPlacements(nil); len(projection) != 0 {
		t.Fatalf("evicted projection=%#v", projection)
	}
}

func TestImageLifecycleEraseModeMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Terminal)
	}{
		{"clear line", func(term *Terminal) { term.ClearLine(1) }},
		{"EL0", func(term *Terminal) { term.SetCursor(1, 2); term.ClearToEndOfLine() }},
		{"EL1", func(term *Terminal) { term.SetCursor(1, 2); term.ClearToBeginningOfLine() }},
		{"ED0", func(term *Terminal) { term.SetCursor(1, 2); term.ClearToEndOfScreen() }},
		{"ED1", func(term *Terminal) { term.SetCursor(1, 2); term.ClearToBeginningOfScreen() }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
			terminal := newImageTerminalForTest(6, 3, 0, store)
			placements := []termimage.PlacementSpec{
				{ID: 1, Anchor: termimage.CellAnchor{Row: 0}, Cols: 1, Rows: 1},
				{ID: 2, Anchor: termimage.CellAnchor{Row: 1}, Cols: 1, Rows: 1},
				{ID: 3, Anchor: termimage.CellAnchor{Row: 1, Col: 3}, Cols: 1, Rows: 1},
				{ID: 4, Anchor: termimage.CellAnchor{Row: 2}, Cols: 1, Rows: 1},
			}
			for index, spec := range placements {
				placeLifecycleImage(t, terminal, termimage.ImageID(index+1), spec)
			}
			test.mutate(terminal)
			remaining := map[termimage.PlacementID]bool{}
			for _, entry := range terminal.imageSidecars.primary {
				remaining[entry.placement.ID] = true
			}
			switch test.name {
			case "clear line":
				if !remaining[1] || remaining[2] || remaining[3] || !remaining[4] {
					t.Fatalf("remaining=%v", remaining)
				}
			case "EL0":
				if !remaining[1] || !remaining[2] || remaining[3] || !remaining[4] {
					t.Fatalf("remaining=%v", remaining)
				}
			case "EL1":
				if !remaining[1] || remaining[2] || !remaining[3] || !remaining[4] {
					t.Fatalf("remaining=%v", remaining)
				}
			case "ED0":
				if !remaining[1] || !remaining[2] || remaining[3] || remaining[4] {
					t.Fatalf("remaining=%v", remaining)
				}
			case "ED1":
				if remaining[1] || remaining[2] || !remaining[3] || !remaining[4] {
					t.Fatalf("remaining=%v", remaining)
				}
			}
		})
	}
}

func TestImageLifecycleCharacterEditBoundaryMatrix(t *testing.T) {
	tests := []struct {
		name   string
		insert bool
		col    uint32
		cols   uint16
		keep   bool
		want   uint32
	}{
		{"insert left touch", true, 0, 2, true, 0}, {"insert cursor crossing", true, 1, 2, false, 0},
		{"insert source start", true, 2, 1, true, 3}, {"insert source end", true, 4, 1, true, 5},
		{"insert discard", true, 5, 1, false, 0}, {"delete left touch", false, 0, 2, true, 0},
		{"delete crossing", false, 1, 2, false, 0}, {"delete band", false, 2, 1, false, 0},
		{"delete source start", false, 3, 1, true, 2}, {"delete source end", false, 5, 1, true, 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
			terminal := newImageTerminalForTest(6, 2, 0, store)
			placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Col: test.col}, Cols: test.cols, Rows: 1})
			terminal.SetCursor(0, 2)
			if test.insert {
				terminal.InsertChars(1)
			} else {
				terminal.DeleteChars(1)
			}
			if (len(terminal.imageSidecars.primary) == 1) != test.keep {
				t.Fatalf("keep=%v placements=%d", test.keep, len(terminal.imageSidecars.primary))
			}
			if test.keep && terminal.imageSidecars.primary[0].placement.Anchor.Col != test.want {
				t.Fatalf("col=%d want=%d", terminal.imageSidecars.primary[0].placement.Anchor.Col, test.want)
			}
		})
	}
}

func TestImageLifecycleVerticalBoundaryMatrix(t *testing.T) {
	tests := []struct {
		name string
		row  int64
		rows uint16
		keep bool
		want int64
	}{
		{"outside", 0, 1, true, 0}, {"cross top", 0, 2, false, 0}, {"dropped top", 1, 1, false, 0},
		{"source start", 2, 1, true, 1}, {"source end", 3, 1, true, 2}, {"cross bottom", 3, 2, false, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
			terminal := newImageTerminalForTest(5, 5, 0, store)
			placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: test.row, Col: 4}, Cols: 2, Rows: test.rows})
			terminal.scrollUpRegion(1, 3, 1)
			if (len(terminal.imageSidecars.primary) == 1) != test.keep {
				t.Fatalf("keep=%v", test.keep)
			}
			if test.keep && terminal.imageSidecars.primary[0].placement.Anchor.Row != test.want {
				t.Fatalf("row=%d want=%d", terminal.imageSidecars.primary[0].placement.Anchor.Row, test.want)
			}
		})
	}
}

func TestImageLifecycleIndexNextLineAndReverseIndex(t *testing.T) {
	for _, entry := range []struct {
		name        string
		invoke      func(*Terminal)
		start, want int64
	}{
		{"index", func(term *Terminal) { term.Index() }, 2, 1},
		{"next line", func(term *Terminal) { term.NextLine() }, 2, 1},
		{"reverse index", func(term *Terminal) { term.ReverseIndex() }, 2, 3},
	} {
		t.Run(entry.name, func(t *testing.T) {
			store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
			terminal := newImageTerminalForTest(5, 4, 0, store)
			terminal.SetScrollRegion(1, 3)
			cursor := 3
			if entry.name == "reverse index" {
				cursor = 1
			}
			terminal.SetCursor(cursor, 0)
			placeLifecycleImage(t, terminal, 1, termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: entry.start}, Cols: 1, Rows: 1})
			entry.invoke(terminal)
			if got := primaryPlacementByID(t, terminal, 1).Anchor.Row; got != entry.want {
				t.Fatalf("row=%d want=%d", got, entry.want)
			}
		})
	}
}
