package mux

import (
	"testing"

	"cervterm/internal/core"
	"cervterm/internal/termimage"
)

func newImageTestMux(t *testing.T) (*Mux, PaneID) {
	t.Helper()
	limits := termimage.DefaultLimits()
	m := New(&fakeFactory{}, Options{IngressCapacity: 8, ImageLimits: &limits, KittyEnabled: true})
	_, id, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Shutdown() })
	return m, id
}

func commitMuxImage(t *testing.T, m *Mux, paneID PaneID, image termimage.ImageID) termimage.ResourceRef {
	t.Helper()
	p, ok := m.sessions.lookup(paneID)
	if !ok || p.imageStore == nil {
		t.Fatal("missing pane image store")
	}
	candidate, err := p.imageStore.NewDecodedCandidate(image, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = candidate.WriteRGBAAt(0, []byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	result, err := p.terminal.CommitImage(core.ImageCommit{Candidate: candidate})
	if err != nil {
		t.Fatal(err)
	}
	p.capture()
	return result.Resource
}

func TestMuxImageStoresAreOptionalAndPaneLocal(t *testing.T) {
	plain, _, _ := newTestMux(t)
	plainPane, _ := plain.sessions.lookup(1)
	if plain.imageBudget != nil || plainPane.imageStore != nil {
		t.Fatal("legacy mux unexpectedly enabled images")
	}
	m, first := newImageTestMux(t)
	second, _, err := m.SpawnSplit(first, SplitColumns, SpawnSpec{})
	if err != nil {
		t.Fatal(err)
	}
	p1, _ := m.sessions.lookup(first)
	p2, _ := m.sessions.lookup(second)
	if p1.imageStore == nil || p2.imageStore == nil || p1.imageStore == p2.imageStore {
		t.Fatal("stores are not pane-local")
	}
}

func TestMuxAcquireImageResourceChecksPaneAndGenerationAndDetaches(t *testing.T) {
	m, paneID := newImageTestMux(t)
	ref := commitMuxImage(t, m, paneID, 7)
	resource, ok := m.AcquireImageResource(paneID, ref)
	if !ok || len(resource.RGBA) != 4 {
		t.Fatalf("acquire=%#v %v", resource, ok)
	}
	resource.RGBA[0] = 99
	again, ok := m.AcquireImageResource(paneID, ref)
	if !ok || again.RGBA[0] != 1 {
		t.Fatal("resource aliases store memory")
	}
	if _, ok = m.AcquireImageResource(paneID+99, ref); ok {
		t.Fatal("wrong pane accepted")
	}
	if _, ok = m.AcquireImageResource(paneID, termimage.ResourceRef{Image: ref.Image, Generation: ref.Generation + 1}); ok {
		t.Fatal("stale generation accepted")
	}
}

func TestMuxShutdownReleasesSharedImageBudget(t *testing.T) {
	m, paneID := newImageTestMux(t)
	_ = commitMuxImage(t, m, paneID, 9)
	if m.imageBudget.Usage() == (termimage.Usage{}) {
		t.Fatal("usage not charged")
	}
	if err := m.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if got := m.imageBudget.Usage(); got != (termimage.Usage{}) {
		t.Fatalf("leaked usage=%#v", got)
	}
}

func TestMuxInvalidImageLimitsFailClosed(t *testing.T) {
	invalid := termimage.Limits{}
	m := New(&fakeFactory{}, Options{ImageLimits: &invalid, KittyEnabled: true})
	if m.ImageSetupError() == nil || m.imageBudget != nil {
		t.Fatal("invalid limits did not fail closed")
	}
	_, id, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := m.sessions.lookup(id)
	if p.imageStore != nil {
		t.Fatal("invalid limits enabled pane store")
	}
	_ = m.Shutdown()
}

func TestMuxAllDisabledIgnoresLimitsWithoutWakeOrImageAllocation(t *testing.T) {
	invalid := termimage.Limits{}
	wakes := make(chan struct{}, 1)
	m := New(&fakeFactory{}, Options{ImageLimits: &invalid, Wake: func() {
		select {
		case wakes <- struct{}{}:
		default:
		}
	}})
	if m.options.ImageLimits != nil || m.ImageSetupError() != nil || m.imageBudget != nil || m.imageScheduler != nil ||
		m.kittyPending != nil || m.sixelPending != nil || m.itermPending != nil || m.imageLimits != (termimage.Limits{}) {
		t.Fatalf("all-disabled mux retained image state: options=%#v limits=%#v budget=%p scheduler=%p", m.options, m.imageLimits, m.imageBudget, m.imageScheduler)
	}
	_, id, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	pane, _ := m.sessions.lookup(id)
	if pane.imageStore != nil || pane.kittyAdapter != nil || pane.sixelAdapter != nil || pane.itermAdapter != nil {
		t.Fatalf("all-disabled pane retained image state: %#v", pane)
	}
	if deadline, ok := m.NextImageDeadline(); ok || !deadline.IsZero() {
		t.Fatalf("all-disabled deadline=%v ok=%v", deadline, ok)
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if events := m.Drain(1); events != nil {
			t.Fatalf("all-disabled drain events=%#v", events)
		}
	}); allocs != 0 {
		t.Fatalf("all-disabled image idle allocated %.0f times", allocs)
	}
	if len(wakes) != 0 {
		t.Fatalf("all-disabled image idle woke %d times", len(wakes))
	}
	_ = m.Shutdown()
}

func BenchmarkMuxAllDisabledImageIdle(b *testing.B) {
	limits := termimage.DefaultLimits()
	m := New(&fakeFactory{}, Options{ImageLimits: &limits})
	if _, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = m.Shutdown() })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Drain(1)
		m.NextImageDeadline()
	}
	b.StopTimer()
	if m.options.ImageLimits != nil || m.imageBudget != nil || m.imageScheduler != nil {
		b.Fatal("all-disabled benchmark activated images")
	}
}

func TestMuxCrossWindowTransferPreservesImageStoreAndResource(t *testing.T) {
	m, first := newImageTestMux(t)
	destination, _ := addRuntimeTestWindow(t, m, tabMetrics())
	if err := m.model.ActivateWindow(1); err != nil {
		t.Fatal(err)
	}
	paneID, _, err := m.SpawnSplit(first, SplitColumns, SpawnSpec{})
	if err != nil {
		t.Fatal(err)
	}
	ref := commitMuxImage(t, m, paneID, 11)
	before, _ := m.sessions.lookup(paneID)
	_, err = m.TransferPaneBetweenWindows(PaneTransferRequest{SourceWindow: 1, DestinationWindow: destination.ID, Pane: paneID, DestinationTab: destination.Tabs[0].ID, DestinationPane: destination.Tabs[0].Focused, Axis: SplitRows, Ratio: DefaultSplitRatio, SourceBounds: PixelRect{Width: 800, Height: 480}, DestinationBounds: PixelRect{Width: 480, Height: 320}, Resolve: m.resolveMetrics})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := m.sessions.lookup(paneID)
	if after != before || after.imageStore != before.imageStore {
		t.Fatal("transfer changed image ownership")
	}
	if _, ok := m.AcquireImageResource(paneID, ref); !ok {
		t.Fatal("resource lost across transfer")
	}
}

func TestMuxCrossWindowTabTransferPreservesEveryImageStoreAndResource(t *testing.T) {
	m, firstPane := newImageTestMux(t)
	destination, _ := addRuntimeTestWindow(t, m, tabMetrics())
	if err := m.model.ActivateWindow(1); err != nil {
		t.Fatal(err)
	}
	secondPane, _, err := m.SpawnSplit(firstPane, SplitColumns, SpawnSpec{})
	if err != nil {
		t.Fatal(err)
	}
	refs := map[PaneID]termimage.ResourceRef{firstPane: commitMuxImage(t, m, firstPane, 21), secondPane: commitMuxImage(t, m, secondPane, 22)}
	stores := make(map[PaneID]*termimage.Store, 2)
	for id := range refs {
		pane, _ := m.sessions.lookup(id)
		stores[id] = pane.imageStore
	}
	if _, err = m.TransferTabBetweenWindows(TabTransferRequest{SourceWindow: 1, DestinationWindow: destination.ID, Tab: 1, Position: 1, SourceBounds: PixelRect{Width: 800, Height: 480}, DestinationBounds: PixelRect{Width: 480, Height: 320}, Resolve: m.resolveMetrics}); err != nil {
		t.Fatal(err)
	}
	for id, ref := range refs {
		pane, ok := m.sessions.lookup(id)
		if !ok || pane.imageStore != stores[id] {
			t.Fatalf("pane %d changed image store", id)
		}
		if _, ok = m.AcquireImageResource(id, ref); !ok {
			t.Fatalf("pane %d lost resource", id)
		}
	}
}

func TestMuxRestoreAbortClosesEveryDetachedPaneStore(t *testing.T) {
	limits := termimage.DefaultLimits()
	m := New(&restoreTestFactory{}, Options{IngressCapacity: 64, ImageLimits: &limits, KittyEnabled: true})
	candidate, err := m.PrepareRestore(blueprintFromSnapshot(t, restoreSnapshot()), restoreGeometries())
	if err != nil {
		t.Fatal(err)
	}
	stores := make([]*termimage.Store, len(candidate.panes))
	for i, pane := range candidate.panes {
		stores[i] = pane.imageStore
		if stores[i] == nil {
			t.Fatalf("pane %d missing store", pane.id)
		}
		for j := 0; j < i; j++ {
			if stores[j] == stores[i] {
				t.Fatal("restore panes share store")
			}
		}
		image, imageErr := stores[i].AllocateInternalImageID()
		placement, placementErr := stores[i].AllocateInternalPlacementID()
		if imageErr != nil || placementErr != nil || image != termimage.MinInternalImageID || placement != termimage.MinInternalPlacementID {
			t.Fatalf("pane %d fresh IDs image=%#x placement=%#x imageErr=%v placementErr=%v", pane.id, image, placement, imageErr, placementErr)
		}
	}
	if err = m.AbortRestore(candidate); err != nil {
		t.Fatal(err)
	}
	for i, store := range stores {
		if !store.Closed() {
			t.Fatalf("store %d remained open", i)
		}
	}
	if got := m.imageBudget.Usage(); got != (termimage.Usage{}) {
		t.Fatalf("abort leaked usage=%#v", got)
	}
}
