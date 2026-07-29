package mux

import (
	"errors"
	"testing"

	"cervterm/internal/termimage"
)

var l302BenchmarkEvents []Event
var l302BenchmarkLayout Layout
var l302BenchmarkFrameChecksum uint64
var l302BenchmarkSpawnErr = errors.New("benchmark spawn unavailable")

func l302ConsumePaneView(view PaneView) uint64 {
	sum := uint64(view.Snapshot.Cols + view.Snapshot.Rows + view.ScrollbackLines)
	for _, cell := range view.Snapshot.Cells {
		sum += uint64(cell.Rune) + uint64(cell.HyperlinkID)
	}
	return sum
}

func newL302BenchmarkMux(b *testing.B) *Mux {
	b.Helper()
	m := New(&fakeFactory{err: l302BenchmarkSpawnErr}, Options{IngressCapacity: 2048})
	if _, pane, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); !errors.Is(err, l302BenchmarkSpawnErr) || pane != 1 {
		b.Fatalf("bootstrap pane=%d err=%v", pane, err)
	}
	b.Cleanup(func() { _ = m.Shutdown() })
	return m
}

func BenchmarkL302OwnerFastPathMutationBaseline(b *testing.B) {
	m := newL302BenchmarkMux(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		events, err := m.FeedFallback(1, []byte("x"))
		if err != nil {
			b.Fatal(err)
		}
		l302BenchmarkEvents = events
	}
}

func BenchmarkL302HeadlessFrameBaseline(b *testing.B) {
	m := newL302BenchmarkMux(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		layout, err := m.Layout()
		if err != nil {
			b.Fatal(err)
		}
		l302BenchmarkLayout = layout
		view, ok := m.PaneView(1)
		if !ok {
			b.Fatal("pane view unavailable")
		}
		l302BenchmarkFrameChecksum = l302ConsumePaneView(view)
	}
}

func newL302BenchmarkOwner(b *testing.B) *Owner {
	b.Helper()
	owner := NewOwner(&fakeFactory{err: l302BenchmarkSpawnErr}, Options{IngressCapacity: 2048})
	if _, pane, _, err := testWindowOwnerForOwner(owner).Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); !errors.Is(err, l302BenchmarkSpawnErr) || pane != 1 {
		b.Fatalf("bootstrap pane=%d err=%v", pane, err)
	}
	b.Cleanup(func() { _ = owner.Shutdown() })
	return owner
}

func BenchmarkL302OwnerFastPathMutationCandidate(b *testing.B) {
	owner := newL302BenchmarkOwner(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		events, err := testWindowOwnerForOwner(owner).FeedFallback(1, []byte("x"))
		if err != nil {
			b.Fatal(err)
		}
		l302BenchmarkEvents = events
	}
}

func BenchmarkL302HeadlessFrameCandidate(b *testing.B) {
	owner := newL302BenchmarkOwner(b)
	window := testWindowOwnerForOwner(owner)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		layout, err := window.Layout()
		if err != nil {
			b.Fatal(err)
		}
		l302BenchmarkLayout = layout
		view, ok := window.PaneView(1)
		if !ok {
			b.Fatal("pane view unavailable")
		}
		l302BenchmarkFrameChecksum = l302ConsumePaneView(view)
	}
}

func BenchmarkL302StartupBaseline(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := New(&fakeFactory{}, Options{IngressCapacity: 8})
		if _, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
			b.Fatal(err)
		}
		if err := m.Shutdown(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkL302StartupCandidate(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		owner := NewOwner(&fakeFactory{}, Options{IngressCapacity: 8})
		if _, _, _, err := testWindowOwnerForOwner(owner).Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
			b.Fatal(err)
		}
		if err := owner.Shutdown(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkL302StartupProxyCandidate(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := newMux(&fakeFactory{}, Options{IngressCapacity: 8})
		b.StopTimer()
		err := m.Shutdown()
		b.StartTimer()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func expandL302BenchmarkRegistry(b *testing.B, m *Mux) {
	b.Helper()
	pane, ok := m.sessions.lookup(1)
	if !ok {
		b.Fatal("benchmark pane unavailable")
	}
	m.sessions.mu.Lock()
	for id := PaneID(2); id <= 512; id++ {
		m.sessions.panes[id] = pane
	}
	m.sessions.mu.Unlock()
	b.Cleanup(func() {
		m.sessions.mu.Lock()
		for id := PaneID(2); id <= 512; id++ {
			delete(m.sessions.panes, id)
		}
		m.sessions.mu.Unlock()
	})
}

func BenchmarkL302ProcessMutationBaseline(b *testing.B) {
	m := newL302BenchmarkMux(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for record := 0; record < 1024; record++ {
			m.sessions.incoming <- ingressRecord{}
		}
		l302BenchmarkEvents = m.Drain(1024)
	}
}

func BenchmarkL302ProcessMutationCandidate(b *testing.B) {
	owner := newL302BenchmarkOwner(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for record := 0; record < 1024; record++ {
			owner.mux.sessions.incoming <- ingressRecord{}
		}
		events, err := owner.Drain(1024)
		if err != nil {
			b.Fatal(err)
		}
		l302BenchmarkEvents = events
	}
}

func BenchmarkL302WindowMutationBaseline(b *testing.B) {
	m := newL302BenchmarkMux(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		events, err := m.Resize(PixelRect{Width: 800 + (i & 1), Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
		if err != nil {
			b.Fatal(err)
		}
		l302BenchmarkEvents = events
	}
}

func BenchmarkL302WindowMutationCandidate(b *testing.B) {
	owner := newL302BenchmarkOwner(b)
	window, err := owner.ForWindow(1, func(identity WindowIdentity) bool { return identity.ID == 1 && identity.Incarnation != 0 })
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		events, resizeErr := window.Resize(PixelRect{Width: 800 + (i & 1), Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
		if resizeErr != nil {
			b.Fatal(resizeErr)
		}
		l302BenchmarkEvents = events
	}
}

func BenchmarkL302PaneMutationBaseline(b *testing.B) {
	m := newL302BenchmarkMux(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		events, err := m.FeedFallback(1, []byte("x"))
		if err != nil {
			b.Fatal(err)
		}
		l302BenchmarkEvents = events
	}
}

func BenchmarkL302PaneMutationCandidate(b *testing.B) {
	owner := newL302BenchmarkOwner(b)
	window, err := owner.ForWindow(1, func(identity WindowIdentity) bool { return identity.ID == 1 && identity.Incarnation != 0 })
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		events, feedErr := window.FeedFallback(1, []byte("x"))
		if feedErr != nil {
			b.Fatal(feedErr)
		}
		l302BenchmarkEvents = events
	}
}

func seedL302ImageStoreBaseline(b *testing.B, store *termimage.Store, owner *termimage.StoreOwner) {
	b.Helper()
	seedL302ImageStoreCandidate(b, store, owner)
}

func seedL302ImageStoreCandidate(b *testing.B, store *termimage.Store, owner *termimage.StoreOwner) {
	b.Helper()
	for image := uint32(1); image <= 256; image++ {
		candidate, err := store.NewDecodedCandidate(termimage.ImageID(image), 1, 1)
		if err != nil {
			b.Fatal(err)
		}
		prepared, _, err := owner.PrepareCandidate(candidate)
		if err != nil {
			b.Fatal(err)
		}
		if err := owner.PublishPrepared(prepared); err != nil {
			b.Fatal(err)
		}
		if err := prepared.Commit(); err != nil {
			b.Fatal(err)
		}
	}
	seedL302ImagePlacements(b, store)
}

func seedL302ImagePlacements(b *testing.B, store *termimage.Store) {
	b.Helper()
	for placement := 0; placement < 1024; placement++ {
		if _, err := store.ReservePlacements(1); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkL302ImageMutationBaseline(b *testing.B) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	owner := store.ClaimOwner()
	if owner == nil {
		b.Fatal("store owner unavailable")
	}
	b.Cleanup(func() { _ = owner.Close() })
	seedL302ImageStoreBaseline(b, store, owner)
	refs := store.ResourceRefs()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prepared, err := owner.PrepareResourceRemoval(refs)
		if err != nil {
			b.Fatal(err)
		}
		if err := prepared.Abort(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkL302ImageMutationCandidate(b *testing.B) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	owner := store.ClaimOwner()
	if owner == nil {
		b.Fatal("store owner unavailable")
	}
	b.Cleanup(func() { _ = owner.Close() })
	seedL302ImageStoreCandidate(b, store, owner)
	refs := store.ResourceRefs()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prepared, err := owner.PrepareResourceRemoval(refs)
		if err != nil {
			b.Fatal(err)
		}
		if err := prepared.Abort(); err != nil {
			b.Fatal(err)
		}
	}
}
