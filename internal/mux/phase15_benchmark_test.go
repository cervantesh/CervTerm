package mux

import "testing"

var phase15TopologyCount int

func BenchmarkPhase15ManyTabsWindowsSnapshot(b *testing.B) {
	factory := &fakeFactory{}
	m := New(factory, Options{IngressCapacity: 8})
	content := PixelRect{Width: 1200, Height: 800}
	metrics := CellMetrics{CellWidth: 8, CellHeight: 16}
	if _, _, _, err := m.Bootstrap(SpawnSpec{}, content, metrics); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = m.Shutdown() })
	for index := 1; index < 8; index++ {
		if _, _, err := m.CreateWindow(SpawnSpec{}, content, metrics, "window"); err != nil {
			b.Fatal(err)
		}
	}
	for index := 1; index < 32; index++ {
		if _, _, _, err := m.SpawnTab(SpawnSpec{}, metrics, "tab"); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		windows := m.Windows()
		tabs := m.Tabs()
		phase15TopologyCount = len(windows) + len(tabs)
	}
}
