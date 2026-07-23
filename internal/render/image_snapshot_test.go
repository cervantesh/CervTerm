package render

import (
	"testing"

	"cervterm/internal/core"
	"cervterm/internal/termimage"
)

func imageCandidate(t *testing.T, store *termimage.Store, image termimage.ImageID) *termimage.DecodedCandidate {
	t.Helper()
	candidate, err := store.NewDecodedCandidate(image, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = candidate.WriteRGBAAt(0, []byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	return candidate
}

func TestCaptureProjectsDetachedImagesAndReusesCapacity(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := core.NewTerminal(6, 3)
	if err := terminal.AttachImageStore(store); err != nil {
		t.Fatal(err)
	}
	crop := termimage.PixelRect{Width: 1, Height: 1}
	if _, err := terminal.CommitImage(core.ImageCommit{Candidate: imageCandidate(t, store, 1), Placement: &termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 1, Col: 2}, Cols: 2, Rows: 1, Crop: &crop, Z: -1, Opacity: 200}}); err != nil {
		t.Fatal(err)
	}
	var snapshot Snapshot
	CaptureWithOptions(&snapshot, terminal, CaptureOptions{PaneObject: 42})
	if snapshot.PaneObject != 42 || snapshot.ImageGeneration == 0 || len(snapshot.Images) != 1 {
		t.Fatalf("snapshot=%#v", snapshot.Images)
	}
	image := snapshot.Images[0]
	if image.PaneObject != 42 || image.Placement.Resource.Image != 1 || image.Placement.Anchor.Row != 1 || image.Placement.Crop == nil || image.Placement.Z != -1 || image.Placement.Opacity != 200 {
		t.Fatalf("image=%#v", image)
	}
	image.Placement.Crop.Width = 99
	if got := terminal.ImageProjection(0, 3).Placements[0].Crop.Width; got != 1 {
		t.Fatal("snapshot crop aliases terminal")
	}
	backing := &snapshot.Images[0]
	CaptureWithOptions(&snapshot, terminal, CaptureOptions{PaneObject: 42})
	if &snapshot.Images[0] != backing {
		t.Fatal("image capture did not reuse capacity")
	}
	id := termimage.PlacementID(1)
	if _, err := terminal.DeleteImages(termimage.DeleteSelector{Placement: &id}); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Images) != 1 {
		t.Fatal("terminal mutation changed prior snapshot")
	}
	CaptureWithOptions(&snapshot, terminal, CaptureOptions{PaneObject: 42})
	if len(snapshot.Images) != 0 {
		t.Fatal("capture retained deleted placement")
	}
}

func TestImageGenerationIsIndependentFromTextRowHashes(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := core.NewTerminal(4, 2)
	if err := terminal.AttachImageStore(store); err != nil {
		t.Fatal(err)
	}
	var before, after Snapshot
	Capture(&before, terminal)
	if _, err := terminal.CommitImage(core.ImageCommit{Candidate: imageCandidate(t, store, 1), Placement: &termimage.PlacementSpec{ID: 1, Cols: 1, Rows: 1}}); err != nil {
		t.Fatal(err)
	}
	Capture(&after, terminal)
	left, right := make([]uint64, before.Rows), make([]uint64, after.Rows)
	HashRows(left, before.Cells, before.Cols)
	HashRows(right, after.Cells, after.Cols)
	for row := range left {
		if left[row] != right[row] {
			t.Fatal("image changed text row hash")
		}
	}
	if before.ImageGeneration == after.ImageGeneration {
		t.Fatal("image generation did not change")
	}
}

func TestTextOnlyCaptureKeepsNilImagesAllocationFree(t *testing.T) {
	terminal := core.NewTerminal(20, 4)
	var snapshot Snapshot
	Capture(&snapshot, terminal)
	allocs := testing.AllocsPerRun(1000, func() { Capture(&snapshot, terminal) })
	if allocs != 0 || snapshot.Images != nil || snapshot.ImageGeneration != 0 {
		t.Fatalf("allocs=%f images=%#v generation=%d", allocs, snapshot.Images, snapshot.ImageGeneration)
	}
}

func TestDetachedSnapshotPreservesNilImageFastPath(t *testing.T) {
	detached := DetachedSnapshot(Snapshot{})
	if detached.Images != nil {
		t.Fatal("nil images became non-nil")
	}
}
