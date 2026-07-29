package mux

import (
	"reflect"
	"sort"
	"testing"
)

// l302MutationInventory is the executable coverage authority for every mux
// mutation family entering the Slice 3.1 owner boundary.
var l302MutationInventory = map[string][]string{
	"window":              {"CreateWindow", "ActivateWindow", "CloseWindow", "RollbackWindow"},
	"tab":                 {"SpawnTab", "ActivateTab", "RenameTab", "MoveTab", "CloseTab"},
	"pane":                {"Bootstrap", "SpawnSplit", "Write", "FeedFallback", "SetTitle", "ClosePane"},
	"topology":            {"Split", "TransferPane", "TransferPaneBetweenWindows", "TransferTabBetweenWindows", "ResizeCurrentPane", "SwapCurrentPane", "MoveCurrentPane", "SetSplitRatio"},
	"focus":               {"FocusPane", "FocusDirection", "FocusNext"},
	"resize":              {"Resize", "ResizeGrid", "ResizeBounds", "ResizePaneGrid", "ApplyResize"},
	"session-ingress":     {"Drain"},
	"restore":             {"PrepareRestore", "CommitRestore", "AbortRestore"},
	"workspace":           {"CreateWorkspace", "RenameWorkspace", "SwitchWorkspace", "MoveWindowToWorkspace"},
	"layout":              {"SetSplitRatio", "ResizeCurrentPane", "SwapCurrentPane", "MoveCurrentPane"},
	"termimage":           {"Drain", "FeedFallback"},
	"protocol-scheduling": {"Drain", "FeedFallback"},
	"close":               {"ClosePane", "CloseTab", "CloseWindow", "Shutdown"},
}

func TestL302MutationFamilyInventory(t *testing.T) {
	wantFamilies := []string{"close", "focus", "layout", "pane", "protocol-scheduling", "resize", "restore", "session-ingress", "tab", "termimage", "topology", "window", "workspace"}
	gotFamilies := make([]string, 0, len(l302MutationInventory))
	for family, entryPoints := range l302MutationInventory {
		gotFamilies = append(gotFamilies, family)
		if len(entryPoints) == 0 {
			t.Fatalf("mutation family %q has no entry point", family)
		}
		seen := make(map[string]struct{}, len(entryPoints))
		for _, entryPoint := range entryPoints {
			if entryPoint == "" {
				t.Fatalf("mutation family %q contains an empty entry point", family)
			}
			if _, duplicate := seen[entryPoint]; duplicate {
				t.Fatalf("mutation family %q repeats %q", family, entryPoint)
			}
			seen[entryPoint] = struct{}{}
		}
	}
	sort.Strings(gotFamilies)
	if !reflect.DeepEqual(gotFamilies, wantFamilies) {
		t.Fatalf("mutation families=%v want=%v", gotFamilies, wantFamilies)
	}
}

// TestKnownDefect_L3_02_PublicMuxMutationNeedsNoCapability expires Slice 3.1.
func TestKnownDefect_L3_02_PublicMuxMutationNeedsNoCapability(t *testing.T) {
	m, _, _ := newTestMux(t)
	before := m.Workspaces()
	result := make(chan error, 1)
	go func() {
		_, _, err := m.CreateWorkspace("unguarded")
		result <- err
	}()
	if err := <-result; err != nil {
		t.Fatalf("unguarded mutation error=%v", err)
	}
	after := m.Workspaces()
	if reflect.DeepEqual(before, after) || len(after) != len(before)+1 {
		t.Fatalf("unguarded mutation did not publish: before=%#v after=%#v", before, after)
	}
}

var l302BenchmarkEvents []Event
var l302BenchmarkLayout Layout

func newL302BenchmarkMux(b *testing.B) *Mux {
	b.Helper()
	m := New(&fakeFactory{}, Options{IngressCapacity: 8})
	if _, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = m.Shutdown() })
	return m
}

func BenchmarkL302OwnerFastPathDrainBaseline(b *testing.B) {
	m := newL302BenchmarkMux(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l302BenchmarkEvents = m.Drain(1)
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
	}
}
