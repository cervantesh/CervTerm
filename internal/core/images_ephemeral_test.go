package core

import (
	"errors"
	"testing"

	"cervterm/internal/termimage"
)

func commitEphemeralForTest(t *testing.T, terminal *Terminal, store *termimage.Store, image termimage.ImageID, placement termimage.PlacementSpec) termimage.ResourceRef {
	t.Helper()
	result, err := terminal.CommitImage(ImageCommit{
		Candidate: decodedCandidateForTest(t, store, image, 1, 1),
		Placement: &placement,
		Retention: termimage.ResourceEphemeral,
	})
	if err != nil || result.Placement == nil {
		t.Fatalf("commit result=%#v err=%v", result, err)
	}
	return result.Resource
}

func TestEphemeralCommitRequiresCreateAndPlacement(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 0, store)
	candidate := decodedCandidateForTest(t, store, termimage.MinInternalImageID, 1, 1)
	if _, err := terminal.CommitImage(ImageCommit{Candidate: candidate, Retention: termimage.ResourceEphemeral}); !errors.Is(err, termimage.ErrInvalidRetention) {
		t.Fatalf("missing placement err=%v", err)
	}
	ref := commitEphemeralForTest(t, terminal, store, termimage.MinInternalImageID, termimage.PlacementSpec{ID: termimage.MinInternalPlacementID, Cols: 1, Rows: 1})
	candidate = decodedCandidateForTest(t, store, termimage.MinInternalImageID, 1, 1)
	if _, err := terminal.CommitImage(ImageCommit{Candidate: candidate, Placement: &termimage.PlacementSpec{ID: termimage.MinInternalPlacementID + 1, Cols: 1, Rows: 1}, Retention: termimage.ResourceEphemeral}); !errors.Is(err, termimage.ErrInvalidRetention) {
		t.Fatalf("replacement err=%v", err)
	}
	if _, ok := store.Acquire(ref); !ok {
		t.Fatal("failed replacement retired prior resource")
	}
}

func TestEphemeralFinalPlacementRetiresResourceOnOverwriteAndExplicitDelete(t *testing.T) {
	for _, test := range []struct {
		name   string
		remove func(*Terminal, termimage.PlacementID)
	}{
		{"overwrite", func(terminal *Terminal, _ termimage.PlacementID) { terminal.SetCursor(0, 0); terminal.PutRune('x') }},
		{"explicit placement", func(terminal *Terminal, id termimage.PlacementID) {
			if removed, err := terminal.DeleteImages(termimage.DeleteSelector{Placement: &id}); err != nil || removed != 1 {
				panic("delete failed")
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			process := termimage.NewProcessBudget()
			store := termimage.NewStore(process, termimage.DefaultLimits())
			terminal := newImageTerminalForTest(4, 2, 0, store)
			placementID := termimage.MinInternalPlacementID
			ref := commitEphemeralForTest(t, terminal, store, termimage.MinInternalImageID, termimage.PlacementSpec{ID: placementID, Cols: 1, Rows: 1})
			test.remove(terminal, placementID)
			if _, ok := store.Acquire(ref); ok || store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
				t.Fatalf("resource survived usage=%#v", store.Usage())
			}
		})
	}
}

func TestEphemeralResourceSurvivesUntilFinalPlacementAcrossScreens(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 0, store)
	ref := commitEphemeralForTest(t, terminal, store, termimage.MinInternalImageID, termimage.PlacementSpec{ID: termimage.MinInternalPlacementID, Cols: 1, Rows: 1})
	terminal.SetAlternateScreenMode(true)
	if _, err := terminal.CommitImage(ImageCommit{Existing: &ref, Placement: &termimage.PlacementSpec{ID: termimage.MinInternalPlacementID + 1, Cols: 1, Rows: 1}}); err != nil {
		t.Fatal(err)
	}
	terminal.SetAlternateScreenMode(false)
	if _, ok := store.Acquire(ref); !ok {
		t.Fatal("alternate retirement removed primary-retained resource")
	}
	terminal.SetCursor(0, 0)
	terminal.PutRune('x')
	if _, ok := store.Acquire(ref); ok || store.Usage() != (termimage.Usage{}) {
		t.Fatalf("final primary retirement leaked usage=%#v", store.Usage())
	}
}

func TestEphemeralLifecyclePreparationFaultIsOldOrNew(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 0, store)
	ref := commitEphemeralForTest(t, terminal, store, termimage.MinInternalImageID, termimage.PlacementSpec{ID: termimage.MinInternalPlacementID, Cols: 1, Rows: 1})
	generation := terminal.imageSidecars.generation
	fault := errors.New("fault")
	changed, err := terminal.mutateImagePlacementsWithFault(false, func(placement termimage.Placement) (termimage.Placement, bool) {
		return placement, false
	}, func(step imagePrepareStep) error {
		if step == imagePrepareStore {
			return fault
		}
		return nil
	})
	if changed || !errors.Is(err, fault) || terminal.imageSidecars.generation != generation {
		t.Fatalf("changed=%v err=%v generation=%d", changed, err, terminal.imageSidecars.generation)
	}
	if _, ok := store.Acquire(ref); !ok || store.Usage().Placements != 1 {
		t.Fatalf("fault changed old state usage=%#v", store.Usage())
	}
}

func TestDurableKittyStyleResourceSurvivesFinalPlacementRetirement(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 0, store)
	result, err := terminal.CommitImage(ImageCommit{Candidate: decodedCandidateForTest(t, store, 1, 1, 1), Placement: &termimage.PlacementSpec{ID: 1, Cols: 1, Rows: 1}})
	if err != nil {
		t.Fatal(err)
	}
	terminal.SetCursor(0, 0)
	terminal.PutRune('x')
	if _, ok := store.Acquire(result.Resource); !ok || store.Usage().Images != 1 || store.Usage().Placements != 0 {
		t.Fatalf("durable resource retired usage=%#v", store.Usage())
	}
}

func TestEphemeralLifecycleHistoryReflowAlternateResetAndClose(t *testing.T) {
	t.Run("history eviction", func(t *testing.T) {
		store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
		terminal := newImageTerminalForTest(4, 2, 1, store)
		ref := commitEphemeralForTest(t, terminal, store, termimage.MinInternalImageID, termimage.PlacementSpec{ID: termimage.MinInternalPlacementID, Cols: 1, Rows: 1})
		terminal.scrollUpRegion(0, 1, 1)
		if _, ok := store.Acquire(ref); !ok {
			t.Fatal("resource retired before final placement eviction")
		}
		terminal.scrollUpRegion(0, 1, 1)
		if _, ok := store.Acquire(ref); ok || store.Usage() != (termimage.Usage{}) {
			t.Fatalf("history eviction usage=%#v", store.Usage())
		}
	})
	t.Run("reflow eviction", func(t *testing.T) {
		store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
		terminal := newImageTerminalForTest(4, 3, 0, store)
		for _, r := range "abcdefghijkl" {
			terminal.PutRune(r)
		}
		ref := commitEphemeralForTest(t, terminal, store, termimage.MinInternalImageID, termimage.PlacementSpec{ID: termimage.MinInternalPlacementID, Anchor: termimage.CellAnchor{Row: 0}, Cols: 1, Rows: 1})
		terminal.Resize(2, 1)
		if _, ok := store.Acquire(ref); ok || store.Usage() != (termimage.Usage{}) {
			t.Fatalf("reflow usage=%#v", store.Usage())
		}
	})
	t.Run("alternate exit", func(t *testing.T) {
		store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
		terminal := newImageTerminalForTest(4, 2, 0, store)
		terminal.SetAlternateScreenMode(true)
		ref := commitEphemeralForTest(t, terminal, store, termimage.MinInternalImageID, termimage.PlacementSpec{ID: termimage.MinInternalPlacementID, Cols: 1, Rows: 1})
		terminal.SetAlternateScreenMode(false)
		if _, ok := store.Acquire(ref); ok || store.Usage() != (termimage.Usage{}) {
			t.Fatalf("alternate usage=%#v", store.Usage())
		}
	})
	for _, closeStore := range []bool{false, true} {
		name := "reset"
		if closeStore {
			name = "close"
		}
		t.Run(name, func(t *testing.T) {
			store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
			terminal := newImageTerminalForTest(4, 2, 0, store)
			commitEphemeralForTest(t, terminal, store, termimage.MinInternalImageID, termimage.PlacementSpec{ID: termimage.MinInternalPlacementID, Cols: 1, Rows: 1})
			if closeStore {
				terminal.CloseImageStore()
			} else {
				terminal.ResetImages()
			}
			if store.Usage() != (termimage.Usage{}) {
				t.Fatalf("usage=%#v", store.Usage())
			}
		})
	}
}

func TestEphemeralRetirementPreservesResourceGenerationMonotonicity(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 0, store)
	first := commitEphemeralForTest(t, terminal, store, termimage.MinInternalImageID, termimage.PlacementSpec{ID: termimage.MinInternalPlacementID, Cols: 1, Rows: 1})
	terminal.SetCursor(0, 0)
	terminal.PutRune('x')
	second := commitEphemeralForTest(t, terminal, store, termimage.MinInternalImageID, termimage.PlacementSpec{ID: termimage.MinInternalPlacementID + 1, Anchor: termimage.CellAnchor{Col: 1}, Cols: 1, Rows: 1})
	if second.Generation <= first.Generation {
		t.Fatalf("generation %d -> %d", first.Generation, second.Generation)
	}
}

func TestEphemeralPreparedRetirementAbortAndPublishAreOldOrNew(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 0, store)
	ref := commitEphemeralForTest(t, terminal, store, termimage.MinInternalImageID, termimage.PlacementSpec{ID: termimage.MinInternalPlacementID, Cols: 1, Rows: 1})
	entry := terminal.imageSidecars.primary[0]
	prepare := func() *preparedImageMutation {
		storePrepared, err := terminal.imageOwner.PrepareResourceRemoval([]termimage.ResourceRef{ref})
		if err != nil {
			t.Fatal(err)
		}
		return &preparedImageMutation{terminal: terminal, store: storePrepared, baseSidecars: terminal.imageSidecars, sidecars: &imageSidecars{generation: terminal.imageSidecars.generation + 1}, retired: []*termimage.PlacementReservation{entry.lease}}
	}
	prepared := prepare()
	terminal.abortPreparedImage(prepared)
	if _, ok := store.Acquire(ref); !ok || len(terminal.imageSidecars.primary) != 1 {
		t.Fatal("abort changed old state")
	}
	prepared = prepare()
	terminal.publishPreparedImage(prepared)
	if _, ok := store.Acquire(ref); ok || store.Usage() != (termimage.Usage{}) {
		t.Fatalf("publish usage=%#v", store.Usage())
	}
}

func TestEphemeralLifecycleStorePreparationFailureClosesImageCapability(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 0, store)
	commitEphemeralForTest(t, terminal, store, termimage.MinInternalImageID, termimage.PlacementSpec{ID: termimage.MinInternalPlacementID, Cols: 1, Rows: 1})
	loose := decodedCandidateForTest(t, store, 2, 1, 1)
	prepared, _, err := terminal.imageOwner.PrepareCandidate(loose)
	if err != nil {
		t.Fatal(err)
	}
	terminal.SetCursor(0, 0)
	terminal.PutRune('x')
	if terminal.imageStore != nil || !store.Closed() || store.Usage() != (termimage.Usage{}) {
		t.Fatalf("fail-closed store=%p closed=%v usage=%#v", terminal.imageStore, store.Closed(), store.Usage())
	}
	prepared.Abort()
}

func TestEphemeralExplicitDeletePreparationFaultRollsBack(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 0, store)
	ref := commitEphemeralForTest(t, terminal, store, termimage.MinInternalImageID, termimage.PlacementSpec{ID: termimage.MinInternalPlacementID, Cols: 1, Rows: 1})
	id := termimage.MinInternalPlacementID
	fault := errors.New("fault")
	if _, _, err := terminal.prepareImageDelete(termimage.DeleteSelector{Placement: &id}, func(step imagePrepareStep) error {
		if step == imagePrepareStore {
			return fault
		}
		return nil
	}); !errors.Is(err, fault) {
		t.Fatalf("err=%v", err)
	}
	if _, ok := store.Acquire(ref); !ok || store.Usage().Placements != 1 {
		t.Fatalf("usage=%#v", store.Usage())
	}
}
