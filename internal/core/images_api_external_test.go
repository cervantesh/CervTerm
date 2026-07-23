package core_test

import (
	"errors"
	"testing"

	"cervterm/internal/core"
	"cervterm/internal/termimage"
)

func apiCandidate(t *testing.T, store *termimage.Store, image termimage.ImageID) *termimage.DecodedCandidate {
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

func TestPublicImageAPICommitProjectionDeleteAndReset(t *testing.T) {
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	terminal := core.NewTerminalWithHistory(6, 3, 2)
	if err := terminal.AttachImageStore(store); err != nil {
		t.Fatal(err)
	}
	crop := termimage.PixelRect{Width: 1, Height: 1}
	result, err := terminal.CommitImage(core.ImageCommit{
		Candidate: apiCandidate(t, store, 1),
		Placement: &termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 1, Col: 2}, Cols: 2, Rows: 1, Crop: &crop, Opacity: 255},
	})
	if err != nil || result.Placement == nil {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	projection := terminal.ImageProjection(0, 3)
	if projection.Generation == 0 || len(projection.Placements) != 1 || projection.Placements[0].Anchor.Row != 1 {
		t.Fatalf("projection=%#v", projection)
	}
	projection.Placements[0].Anchor.Row = 99
	projection.Placements[0].Crop.Width = 99
	again := terminal.ImageProjection(0, 3)
	if again.Placements[0].Anchor.Row != 1 || again.Placements[0].Crop.Width != 1 {
		t.Fatal("projection aliases terminal sidecars")
	}
	maxInt := int(^uint(0) >> 1)
	if got := terminal.ImageProjection(maxInt, maxInt); len(got.Placements) != 0 {
		t.Fatal("overflow viewport accepted")
	}
	if got := terminal.ImageProjection(-1, 1); len(got.Placements) != 0 {
		t.Fatal("negative primary viewport accepted")
	}
	if _, err = terminal.DeleteImages(termimage.DeleteSelector{}); !errors.Is(err, termimage.ErrInvalidSelector) {
		t.Fatalf("selector error=%v", err)
	}
	id := termimage.PlacementID(1)
	removed, err := terminal.DeleteImages(termimage.DeleteSelector{Placement: &id})
	if err != nil || removed != 1 || store.Usage().Placements != 0 {
		t.Fatalf("removed=%d usage=%#v err=%v", removed, store.Usage(), err)
	}
	candidate := apiCandidate(t, store, 2)
	transfer, err := store.BeginTransfer(termimage.Header{Transfer: 1, Image: 3})
	if err != nil {
		t.Fatal(err)
	}
	if err = transfer.Append([]byte("data")); err != nil {
		t.Fatal(err)
	}
	oldEpoch := store.Epoch()
	terminal.ResetImages()
	if store.Epoch() == oldEpoch || candidate.ValidFor(store) || store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
		t.Fatalf("reset epoch=%d usage=%#v", store.Epoch(), store.Usage())
	}
	candidate.Close()
	if _, err = terminal.CommitImage(core.ImageCommit{Candidate: apiCandidate(t, store, 4)}); err != nil {
		t.Fatal(err)
	}
}

func TestAttachImageStoreRejectsDuplicateOwnership(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	first, second := core.NewTerminal(3, 2), core.NewTerminal(3, 2)
	if err := first.AttachImageStore(store); err != nil {
		t.Fatal(err)
	}
	result, err := first.CommitImage(core.ImageCommit{Candidate: apiCandidate(t, store, 1), Placement: &termimage.PlacementSpec{ID: 1, Cols: 1, Rows: 1}})
	if err != nil {
		t.Fatal(err)
	}
	store.Reset()
	store.Close()
	if store.Closed() || len(first.ImageProjection(0, 2).Placements) != 1 {
		t.Fatal("external store lifecycle broke attached terminal")
	}
	if _, ok := store.Acquire(result.Resource); !ok {
		t.Fatal("external lifecycle retired attached resource")
	}
	blocked := apiCandidate(t, store, 9)
	if _, _, err := store.PrepareCandidate(blocked); !errors.Is(err, termimage.ErrPreparedState) {
		t.Fatalf("direct prepare error=%v", err)
	}
	blocked.Close()
	if _, err := store.PrepareResourceRemoval([]termimage.ResourceRef{result.Resource}); !errors.Is(err, termimage.ErrPreparedState) {
		t.Fatalf("direct removal error=%v", err)
	}
	if err := first.AttachImageStore(store); !errors.Is(err, core.ErrImageStoreAttached) {
		t.Fatalf("duplicate error=%v", err)
	}
	if err := second.AttachImageStore(store); !errors.Is(err, core.ErrImageStoreAttached) {
		t.Fatalf("shared error=%v", err)
	}
	closed := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	closed.Close()
	if err := second.AttachImageStore(closed); !errors.Is(err, core.ErrImageStoreUnavailable) {
		t.Fatalf("closed error=%v", err)
	}
	preparedStore := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	preparedCandidate := apiCandidate(t, preparedStore, 10)
	prepared, _, err := preparedStore.PrepareCandidate(preparedCandidate)
	if err != nil {
		t.Fatal(err)
	}
	third := core.NewTerminal(3, 2)
	if err = third.AttachImageStore(preparedStore); !errors.Is(err, core.ErrImageStoreAttached) {
		t.Fatalf("prepared attach error=%v", err)
	}
	prepared.Abort()
	if err = third.AttachImageStore(preparedStore); err != nil {
		t.Fatal(err)
	}
}

func TestPublicImageAPIAlternateProjectionAndExit(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := core.NewTerminal(5, 3)
	if err := terminal.AttachImageStore(store); err != nil {
		t.Fatal(err)
	}
	if _, err := terminal.CommitImage(core.ImageCommit{Candidate: apiCandidate(t, store, 1), Placement: &termimage.PlacementSpec{ID: 1, Cols: 1, Rows: 1}}); err != nil {
		t.Fatal(err)
	}
	terminal.SetAlternateScreenMode(true)
	if len(terminal.ImageProjection(0, 3).Placements) != 0 {
		t.Fatal("primary leaked into alternate")
	}
	if _, err := terminal.CommitImage(core.ImageCommit{Candidate: apiCandidate(t, store, 2), Placement: &termimage.PlacementSpec{ID: 2, Anchor: termimage.CellAnchor{Row: 1}, Cols: 1, Rows: 1}}); err != nil {
		t.Fatal(err)
	}
	if got := terminal.ImageProjection(99, 3); len(got.Placements) != 1 || got.Placements[0].Anchor.Row != 1 {
		t.Fatalf("alternate projection=%#v", got)
	}
	terminal.SetAlternateScreenMode(false)
	if got := terminal.ImageProjection(0, 3); len(got.Placements) != 1 || got.Placements[0].ID != 1 {
		t.Fatalf("primary projection=%#v", got)
	}
	if store.Usage().Placements != 1 {
		t.Fatalf("exit usage=%#v", store.Usage())
	}
}

func TestImageCursorAnchorTracksPrimaryHistoryAndAlternateCoordinates(t *testing.T) {
	terminal := core.NewTerminalWithHistory(4, 2, 4)
	for _, r := range []rune{'a', 'b', 'c'} {
		terminal.PutRune(r)
		terminal.CarriageReturn()
		terminal.NewLine()
	}
	if terminal.ScrollbackLines() == 0 {
		t.Fatal("setup did not create scrollback")
	}
	terminal.SetCursor(1, 2)
	beforeRow, beforeCol := terminal.CursorRow(), terminal.CursorCol()
	primary := terminal.ImageCursorAnchor()
	if want := int64(terminal.ScrollbackLines() + beforeRow); primary.Row != want || primary.Col != uint32(beforeCol) {
		t.Fatalf("primary anchor=%#v want row=%d col=%d", primary, want, beforeCol)
	}
	if terminal.CursorRow() != beforeRow || terminal.CursorCol() != beforeCol {
		t.Fatal("anchor query mutated the cursor")
	}

	terminal.Resize(3, 3)
	afterResize := terminal.ImageCursorAnchor()
	if want := int64(terminal.ScrollbackLines() + terminal.CursorRow()); afterResize.Row != want || afterResize.Col != uint32(terminal.CursorCol()) {
		t.Fatalf("resized anchor=%#v want row=%d col=%d", afterResize, want, terminal.CursorCol())
	}

	terminal.SetAlternateScreenMode(true)
	terminal.SetCursor(2, 1)
	alternate := terminal.ImageCursorAnchor()
	if alternate.Row != 2 || alternate.Col != 1 {
		t.Fatalf("alternate anchor=%#v", alternate)
	}
	terminal.SetAlternateScreenMode(false)
	restored := terminal.ImageCursorAnchor()
	if want := int64(terminal.ScrollbackLines() + terminal.CursorRow()); restored.Row != want || restored.Col != uint32(terminal.CursorCol()) {
		t.Fatalf("restored anchor=%#v want row=%d col=%d", restored, want, terminal.CursorCol())
	}
	var nilTerminal *core.Terminal
	if got := nilTerminal.ImageCursorAnchor(); got != (termimage.CellAnchor{}) {
		t.Fatalf("nil anchor=%#v", got)
	}
}

func TestImageAnchorGenerationTracksCoordinateMutationsNotCursorMovement(t *testing.T) {
	terminal := core.NewTerminalWithHistory(4, 2, 2)
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	if err := terminal.AttachImageStore(store); err != nil {
		t.Fatal(err)
	}
	start := terminal.ImageAnchorGeneration()
	terminal.SetCursor(1, 2)
	if got := terminal.ImageAnchorGeneration(); got != start {
		t.Fatalf("cursor-only generation=%d want=%d", got, start)
	}
	terminal.PutRune('x')
	afterText := terminal.ImageAnchorGeneration()
	if afterText == start {
		t.Fatal("text mutation did not invalidate captured anchors")
	}
	terminal.Resize(3, 3)
	afterResize := terminal.ImageAnchorGeneration()
	if afterResize == afterText {
		t.Fatal("resize/reflow did not invalidate captured anchors")
	}
	terminal.SetAlternateScreenMode(true)
	if got := terminal.ImageAnchorGeneration(); got == afterResize {
		t.Fatal("screen transition did not invalidate captured anchors")
	}
}

func TestImageGenerationReportsFreshOwnerState(t *testing.T) {
	terminal := core.NewTerminal(4, 2)
	var nilTerminal *core.Terminal
	if nilTerminal.ImageGeneration() != 0 || terminal.ImageGeneration() != 0 {
		t.Fatal("unattached image generation must be zero")
	}
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	if err := terminal.AttachImageStore(store); err != nil {
		t.Fatal(err)
	}
	if terminal.ImageGeneration() != 0 {
		t.Fatal("fresh sidecar generation must be zero")
	}
	if _, err := terminal.CommitImage(core.ImageCommit{Candidate: apiCandidate(t, store, 1)}); err != nil {
		t.Fatal(err)
	}
	committed := terminal.ImageGeneration()
	if committed == 0 {
		t.Fatal("commit did not advance the fresh generation")
	}
	terminal.ResetImages()
	if terminal.ImageGeneration() <= committed {
		t.Fatal("reset did not advance the fresh generation")
	}
	terminal.CloseImageStore()
	if terminal.ImageGeneration() != 0 {
		t.Fatal("closed image state must report zero")
	}
}
