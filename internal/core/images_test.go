package core

import (
	"errors"
	"math"
	"testing"

	"cervterm/internal/termimage"
)

func newImageTerminalForTest(cols, rows, history int, store *termimage.Store) *Terminal {
	terminal := NewTerminalWithHistory(cols, rows, history)
	terminal.imageStore = store
	terminal.imageSidecars = &imageSidecars{}
	return terminal
}

func decodedCandidateForTest(t *testing.T, store *termimage.Store, image termimage.ImageID, width, height uint32) *termimage.DecodedCandidate {
	t.Helper()
	candidate, err := store.NewDecodedCandidate(image, width, height)
	if err != nil {
		t.Fatal(err)
	}
	pixels := make([]byte, int(width)*int(height)*4)
	for index := range pixels {
		pixels[index] = byte(index)
	}
	if err := candidate.WriteRGBAAt(0, pixels); err != nil {
		t.Fatal(err)
	}
	return candidate
}

func TestDefaultTerminalImageStateIsNil(t *testing.T) {
	terminal := NewTerminal(10, 2)
	if terminal.imageStore != nil || terminal.imageSidecars != nil {
		t.Fatal("default terminal allocated image state")
	}
}

func TestPrivateImageCommitTransmitPlaceReplaceAndDelete(t *testing.T) {
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	terminal := newImageTerminalForTest(10, 3, 0, store)
	first := decodedCandidateForTest(t, store, 7, 2, 2)
	placement := termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 0, Col: 1}, Cols: 2, Rows: 2, Opacity: 255}
	result, err := terminal.commitImage(imageCommit{candidate: first, placement: &placement})
	if err != nil {
		t.Fatal(err)
	}
	if result.placement == nil || len(terminal.imageSidecars.primary) != 1 || len(terminal.imageSidecars.alternate) != 0 {
		t.Fatalf("commit result=%#v sidecars=%#v", result, terminal.imageSidecars)
	}
	acquired, ok := store.Acquire(result.resource)
	if !ok || len(acquired.RGBA) != 16 {
		t.Fatal("committed resource not acquired")
	}
	usage := store.Usage()
	if usage.Images != 1 || usage.DecodedBytes != 16 || usage.Placements != 1 {
		t.Fatalf("usage=%#v", usage)
	}

	replacement := decodedCandidateForTest(t, store, 7, 1, 1)
	replaced, err := terminal.commitImage(imageCommit{candidate: replacement})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.resource.Generation == result.resource.Generation || len(terminal.imageSidecars.primary) != 0 {
		t.Fatal("replacement did not retire old generation placements")
	}
	if _, ok := store.Acquire(result.resource); ok {
		t.Fatal("old generation remained acquirable")
	}
	if usage = store.Usage(); usage.Images != 1 || usage.DecodedBytes != 4 || usage.Placements != 0 {
		t.Fatalf("replacement usage=%#v", usage)
	}

	imageID := termimage.ImageID(7)
	removed, err := terminal.deleteImages(termimage.DeleteSelector{Image: &imageID, DeleteResource: true})
	if err != nil || removed != 0 {
		t.Fatalf("delete removed=%d err=%v", removed, err)
	}
	if _, ok := store.Acquire(replaced.resource); ok || store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
		t.Fatal("resource delete leaked state")
	}
}

func TestImageCommitIsOldOrNewAndFaultsRollback(t *testing.T) {
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	terminal := newImageTerminalForTest(8, 2, 0, store)
	base, err := terminal.commitImage(imageCommit{candidate: decodedCandidateForTest(t, store, 1, 1, 1)})
	if err != nil {
		t.Fatal(err)
	}
	baseUsage := store.Usage()
	baseGeneration := terminal.imageSidecars.generation
	injected := errors.New("injected")
	steps := []imagePrepareStep{imagePrepareValidate, imagePreparePrimaryCopy, imagePrepareAlternateCopy, imagePreparePlacement, imagePrepareReservation, imagePrepareStore}
	for _, step := range steps {
		spec := termimage.PlacementSpec{ID: termimage.PlacementID(step + 10), Cols: 1, Rows: 1}
		candidate := decodedCandidateForTest(t, store, 2, 1, 1)
		_, _, prepareErr := terminal.prepareImageCommit(imageCommit{candidate: candidate, placement: &spec}, func(got imagePrepareStep) error {
			if got == step {
				return injected
			}
			return nil
		})
		if !errors.Is(prepareErr, injected) {
			t.Fatalf("step %d error=%v", step, prepareErr)
		}
		if _, ok := store.Acquire(base.resource); !ok || terminal.imageSidecars.generation != baseGeneration || len(terminal.imageSidecars.primary) != 0 || store.Usage() != baseUsage {
			t.Fatalf("step %d changed old state usage=%#v", step, store.Usage())
		}
	}

	candidate := decodedCandidateForTest(t, store, 3, 1, 1)
	prepared, result, err := terminal.prepareImageCommit(imageCommit{candidate: candidate}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Acquire(result.resource); ok || terminal.imageSidecars.generation != baseGeneration {
		t.Fatal("prepared state became visible early")
	}
	terminal.publishPreparedImage(prepared)
	if _, ok := store.Acquire(result.resource); !ok || terminal.imageSidecars.generation != baseGeneration+1 {
		t.Fatal("prepared publication incomplete")
	}
}

func TestPlacementValidationCoordinateScreenAndCapacityRollback(t *testing.T) {
	limits := termimage.DefaultLimits()
	limits.Placements = 1
	store := termimage.NewStore(termimage.NewProcessBudget(), limits)
	terminal := newImageTerminalForTest(4, 2, 0, store)
	bad := termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Col: 4}, Cols: 1, Rows: 1}
	if _, err := terminal.commitImage(imageCommit{candidate: decodedCandidateForTest(t, store, 1, 1, 1), placement: &bad}); err == nil {
		t.Fatal("out-of-grid placement accepted")
	}
	good := termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 1, Col: 3}, Cols: 1, Rows: 1}
	first, err := terminal.commitImage(imageCommit{candidate: decodedCandidateForTest(t, store, 1, 1, 1), placement: &good})
	if err != nil {
		t.Fatal(err)
	}
	before := store.Usage()
	secondSpec := termimage.PlacementSpec{ID: 2, Cols: 1, Rows: 1}
	if _, err := terminal.commitImage(imageCommit{candidate: decodedCandidateForTest(t, store, 2, 1, 1), placement: &secondSpec}); err == nil {
		t.Fatal("placement cap exceeded")
	}
	if store.Usage() != before || len(terminal.imageSidecars.primary) != 1 {
		t.Fatalf("failed placement retained state usage=%#v", store.Usage())
	}
	if _, ok := store.Acquire(first.resource); !ok {
		t.Fatal("failed placement altered prior resource")
	}

	terminal.alternateScreen = true
	alternate := termimage.PlacementSpec{ID: 3, Anchor: termimage.CellAnchor{Row: 1}, Cols: 1, Rows: 1}
	// Capacity is still occupied pane-wide, so deletion first proves active-side isolation.
	placementID := termimage.PlacementID(1)
	if _, err := terminal.deleteImages(termimage.DeleteSelector{Placement: &placementID}); err != nil {
		t.Fatal(err)
	}
	if _, err := terminal.commitImage(imageCommit{candidate: decodedCandidateForTest(t, store, 3, 1, 1), placement: &alternate}); err != nil {
		t.Fatal(err)
	}
	if len(terminal.imageSidecars.primary) != 0 || len(terminal.imageSidecars.alternate) != 1 {
		t.Fatal("alternate placement routed incorrectly")
	}
}

func TestImageDeleteSelectorScopesAndResourceExpansion(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(10, 3, 0, store)
	one := termimage.PlacementSpec{ID: 1, Cols: 2, Rows: 2}
	if _, err := terminal.commitImage(imageCommit{candidate: decodedCandidateForTest(t, store, 1, 1, 1), placement: &one}); err != nil {
		t.Fatal(err)
	}
	two := termimage.PlacementSpec{ID: 2, Anchor: termimage.CellAnchor{Col: 3}, Cols: 1, Rows: 1}
	if _, err := terminal.commitImage(imageCommit{candidate: decodedCandidateForTest(t, store, 2, 1, 1), placement: &two}); err != nil {
		t.Fatal(err)
	}
	terminal.cursorRow, terminal.cursorCol = 0, 0
	removed, err := terminal.deleteImages(termimage.DeleteSelector{UnderCursor: true})
	if err != nil || removed != 1 || len(terminal.imageSidecars.primary) != 1 {
		t.Fatalf("under-cursor removed=%d err=%v", removed, err)
	}
	removed, err = terminal.deleteImages(termimage.DeleteSelector{All: true, DeleteResource: true})
	if err != nil || removed != 1 || store.Usage() != (termimage.Usage{}) {
		t.Fatalf("all delete removed=%d usage=%#v err=%v", removed, store.Usage(), err)
	}
}

func TestPlacementReplacementTransfersFullCapacityLease(t *testing.T) {
	limits := termimage.DefaultLimits()
	limits.Placements = 1
	store := termimage.NewStore(termimage.NewProcessBudget(), limits)
	terminal := newImageTerminalForTest(4, 2, 0, store)
	first := termimage.PlacementSpec{ID: 1, Cols: 1, Rows: 1}
	if _, err := terminal.commitImage(imageCommit{candidate: decodedCandidateForTest(t, store, 1, 1, 1), placement: &first}); err != nil {
		t.Fatal(err)
	}
	replacement := termimage.PlacementSpec{ID: 2, Anchor: termimage.CellAnchor{Col: 1}, Cols: 1, Rows: 1}
	result, err := terminal.commitImage(imageCommit{candidate: decodedCandidateForTest(t, store, 1, 1, 1), placement: &replacement})
	if err != nil {
		t.Fatal(err)
	}
	if len(terminal.imageSidecars.primary) != 1 || terminal.imageSidecars.primary[0].placement.ID != 2 || store.Usage().Placements != 1 {
		t.Fatal("replacement did not transfer placement lease")
	}
	if _, ok := store.Acquire(result.resource); !ok {
		t.Fatal("replacement resource unavailable")
	}
}

func TestCurrentScreenScopesResourceSelection(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(5, 2, 0, store)
	primarySpec := termimage.PlacementSpec{ID: 1, Cols: 1, Rows: 1}
	primary, err := terminal.commitImage(imageCommit{candidate: decodedCandidateForTest(t, store, 1, 1, 1), placement: &primarySpec})
	if err != nil {
		t.Fatal(err)
	}
	terminal.alternateScreen = true
	alternateSpec := termimage.PlacementSpec{ID: 2, Cols: 1, Rows: 1}
	if _, err = terminal.commitImage(imageCommit{candidate: decodedCandidateForTest(t, store, 2, 1, 1), placement: &alternateSpec}); err != nil {
		t.Fatal(err)
	}
	imageOne := termimage.ImageID(1)
	removed, err := terminal.deleteImages(termimage.DeleteSelector{Image: &imageOne, CurrentScreen: true, DeleteResource: true})
	if err != nil || removed != 0 {
		t.Fatalf("inactive image selected removed=%d err=%v", removed, err)
	}
	if _, ok := store.Acquire(primary.resource); !ok {
		t.Fatal("inactive resource removed")
	}
	removed, err = terminal.deleteImages(termimage.DeleteSelector{All: true, CurrentScreen: true, DeleteResource: true})
	if err != nil || removed != 1 || len(terminal.imageSidecars.primary) != 1 || len(terminal.imageSidecars.alternate) != 0 {
		t.Fatalf("screen delete removed=%d err=%v", removed, err)
	}
	if _, ok := store.Acquire(primary.resource); !ok {
		t.Fatal("primary resource removed by alternate scope")
	}
}

func TestImageDeletePreparationFaultsRollback(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(5, 2, 0, store)
	spec := termimage.PlacementSpec{ID: 1, Cols: 1, Rows: 1}
	resource, err := terminal.commitImage(imageCommit{candidate: decodedCandidateForTest(t, store, 1, 1, 1), placement: &spec})
	if err != nil {
		t.Fatal(err)
	}
	before := store.Usage()
	injected := errors.New("delete injected")
	for _, step := range []imagePrepareStep{imagePrepareValidate, imagePreparePrimaryCopy, imagePrepareAlternateCopy, imagePrepareStore} {
		_, _, prepareErr := terminal.prepareImageDelete(termimage.DeleteSelector{All: true, DeleteResource: true}, func(got imagePrepareStep) error {
			if got == step {
				return injected
			}
			return nil
		})
		if !errors.Is(prepareErr, injected) {
			t.Fatalf("step %d error=%v", step, prepareErr)
		}
		if _, ok := store.Acquire(resource.resource); !ok || store.Usage() != before || len(terminal.imageSidecars.primary) != 1 {
			t.Fatalf("step %d changed state", step)
		}
	}
}

func TestImageResetAndCloseReleasePreparedAndCommittedPlacements(t *testing.T) {
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	terminal := newImageTerminalForTest(5, 2, 0, store)
	spec := termimage.PlacementSpec{ID: 1, Cols: 1, Rows: 1}
	prepared, _, err := terminal.prepareImageCommit(imageCommit{candidate: decodedCandidateForTest(t, store, 1, 1, 1), placement: &spec}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if usage := store.Usage(); usage.Images != 1 || usage.Placements != 1 {
		t.Fatalf("prepared usage=%#v", usage)
	}
	terminal.resetImages()
	if store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) || len(terminal.imageSidecars.primary) != 0 {
		t.Fatal("reset leaked prepared image ownership")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("stale core publication did not fail before mutation")
			}
		}()
		terminal.publishPreparedImage(prepared)
	}()
	spec.ID = 2
	if _, err = terminal.commitImage(imageCommit{candidate: decodedCandidateForTest(t, store, 2, 1, 1), placement: &spec}); err != nil {
		t.Fatal(err)
	}
	terminal.closeImages()
	if terminal.imageStore != nil || terminal.imageSidecars != nil || store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
		t.Fatal("close leaked committed image ownership")
	}
}

func TestImageResetGenerationExhaustionClosesState(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(2, 1, 0, store)
	terminal.imageSidecars.generation = math.MaxUint64
	terminal.resetImages()
	if terminal.imageStore != nil || terminal.imageSidecars != nil || store.Usage() != (termimage.Usage{}) {
		t.Fatal("generation exhaustion wrapped or retained state")
	}
}
