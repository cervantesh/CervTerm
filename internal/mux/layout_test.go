package mux

import (
	"errors"
	"math/rand"
	"reflect"
	"testing"
)

func TestLayoutAxesRemainderAndHalfOpenRectangles(t *testing.T) {
	tests := []struct {
		name        string
		axis        SplitAxis
		bounds      PixelRect
		wantFirst   PixelRect
		wantDivider PixelRect
		wantSecond  PixelRect
	}{
		{
			name:        "columns give horizontal residue to second child",
			axis:        SplitColumns,
			bounds:      PixelRect{X: 7, Y: 11, Width: 102, Height: 60},
			wantFirst:   PixelRect{X: 7, Y: 11, Width: 50, Height: 60},
			wantDivider: PixelRect{X: 57, Y: 11, Width: 1, Height: 60},
			wantSecond:  PixelRect{X: 58, Y: 11, Width: 51, Height: 60},
		},
		{
			name:        "rows give vertical residue to second child",
			axis:        SplitRows,
			bounds:      PixelRect{X: 7, Y: 11, Width: 100, Height: 102},
			wantFirst:   PixelRect{X: 7, Y: 11, Width: 100, Height: 50},
			wantDivider: PixelRect{X: 7, Y: 61, Width: 100, Height: 1},
			wantSecond:  PixelRect{X: 7, Y: 62, Width: 100, Height: 51},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := NewModel()
			largeBounds := PixelRect{Width: 321, Height: 193}
			if _, err := model.Split(1, test.axis, largeBounds, CellMetrics{CellWidth: 8, CellHeight: 8}); err != nil {
				t.Fatalf("prepare split: %v", err)
			}
			layout, err := model.Layout(test.bounds, CellMetrics{CellWidth: 1, CellHeight: 1})
			if err != nil {
				t.Fatalf("layout: %v", err)
			}
			if len(layout.Panes) != 2 || len(layout.Dividers) != 1 {
				t.Fatalf("layout shape: panes=%d dividers=%d", len(layout.Panes), len(layout.Dividers))
			}
			if layout.Panes[0].Pixels != test.wantFirst || layout.Dividers[0].Pixels != test.wantDivider || layout.Panes[1].Pixels != test.wantSecond {
				t.Fatalf("rectangles = %#v, %#v, %#v; want %#v, %#v, %#v", layout.Panes[0].Pixels, layout.Dividers[0].Pixels, layout.Panes[1].Pixels, test.wantFirst, test.wantDivider, test.wantSecond)
			}
			if layout.Dividers[0].Split == 0 || layout.Dividers[0].Container != test.bounds {
				t.Fatalf("divider identity/container = %#v, want nonzero split and %#v", layout.Dividers[0], test.bounds)
			}
			if layout.Panes[0].Pixels.Right() != layout.Dividers[0].Pixels.X && test.axis == SplitColumns {
				t.Fatal("column rectangles are not half-open and adjacent")
			}
			if layout.Panes[0].Pixels.Bottom() != layout.Dividers[0].Pixels.Y && test.axis == SplitRows {
				t.Fatal("row rectangles are not half-open and adjacent")
			}
		})
	}
}

func TestLayoutNestedVisualDFSAndCellGeometry(t *testing.T) {
	model := NewModel()
	bounds := PixelRect{Width: 321, Height: 193}
	second, err := model.Split(1, SplitColumns, bounds, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	third, err := model.Split(second, SplitRows, bounds, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}

	layout, err := model.Layout(bounds, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	wantPanes := []PaneGeometry{
		{Pane: 1, Pixels: PixelRect{Width: 160, Height: 193}, Cols: 20, Rows: 12},
		{Pane: second, Pixels: PixelRect{X: 161, Width: 160, Height: 96}, Cols: 20, Rows: 6},
		{Pane: third, Pixels: PixelRect{X: 161, Y: 97, Width: 160, Height: 96}, Cols: 20, Rows: 6},
	}
	wantDividers := []Divider{
		{Split: 1, Axis: SplitColumns, Pixels: PixelRect{X: 160, Width: 1, Height: 193}, Container: bounds},
		{Split: 2, Axis: SplitRows, Pixels: PixelRect{X: 161, Y: 96, Width: 160, Height: 1}, Container: PixelRect{X: 161, Width: 160, Height: 193}},
	}
	if !reflect.DeepEqual(layout.Panes, wantPanes) {
		t.Fatalf("panes = %#v, want %#v", layout.Panes, wantPanes)
	}
	if !reflect.DeepEqual(layout.Dividers, wantDividers) {
		t.Fatalf("dividers = %#v, want %#v", layout.Dividers, wantDividers)
	}
	if layout.Compressed {
		t.Fatal("fitting nested layout reported compressed")
	}
}

func TestLayoutCellMetricsPaddingAndCompressedExistingTopology(t *testing.T) {
	model := NewModel()
	layout, err := model.Layout(PixelRect{Width: 100, Height: 70}, CellMetrics{CellWidth: 10, CellHeight: 20, PaddingX: 5, PaddingY: 5})
	if err != nil {
		t.Fatal(err)
	}
	if got := layout.Panes[0]; got.Cols != 9 || got.Rows != 3 {
		t.Fatalf("padded cell geometry = %dx%d, want 9x3", got.Cols, got.Rows)
	}

	if _, err := model.Split(1, SplitColumns, PixelRect{Width: 201, Height: 70}, CellMetrics{CellWidth: 10, CellHeight: 20}); err != nil {
		t.Fatalf("prepare topology: %v", err)
	}
	compressed, err := model.Layout(PixelRect{Width: 1, Height: 1}, CellMetrics{CellWidth: 10, CellHeight: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !compressed.Compressed {
		t.Fatal("undersized existing topology was not marked compressed")
	}
	if len(compressed.Panes) != 2 || len(compressed.Dividers) != 1 {
		t.Fatalf("compressed layout lost topology: panes=%d dividers=%d", len(compressed.Panes), len(compressed.Dividers))
	}
	if compressed.Panes[0].Pixels.Width != 0 || compressed.Dividers[0].Pixels.Width != 1 || compressed.Panes[1].Pixels.Width != 0 {
		t.Fatalf("unexpected one-pixel compressed partition: %#v", compressed)
	}
}

func TestLayoutRejectsInvalidValueGeometry(t *testing.T) {
	model := NewModel()
	for _, test := range []struct {
		name    string
		bounds  PixelRect
		metrics CellMetrics
	}{
		{name: "negative extent", bounds: PixelRect{Width: -1}, metrics: CellMetrics{CellWidth: 1, CellHeight: 1}},
		{name: "zero cell width", bounds: PixelRect{}, metrics: CellMetrics{CellHeight: 1}},
		{name: "negative padding", bounds: PixelRect{}, metrics: CellMetrics{CellWidth: 1, CellHeight: 1, PaddingX: -1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := model.Layout(test.bounds, test.metrics); !errors.Is(err, ErrInvalidGeometry) {
				t.Fatalf("layout error = %v, want %v", err, ErrInvalidGeometry)
			}
		})
	}
}

func TestRandomModelOperationsPreserveInvariants(t *testing.T) {
	const (
		seeds = 32
		steps = 250
	)
	metrics := CellMetrics{CellWidth: 4, CellHeight: 8, PaddingX: 1, PaddingY: 1}
	operationBounds := PixelRect{Width: 4097, Height: 2049}

	for seed := int64(0); seed < seeds; seed++ {
		t.Run(randomTestName(seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			model := NewModel()
			allocated := map[PaneID]struct{}{1: {}}
			maxID := PaneID(1)

			for step := 0; step < steps; step++ {
				ids := model.PaneIDs()
				if len(ids) == 0 {
					break
				}
				switch rng.Intn(6) {
				case 0:
					target := ids[rng.Intn(len(ids))]
					axis := SplitAxis(rng.Intn(2) + int(SplitColumns))
					ratios := []SplitRatio{2500, DefaultSplitRatio, 7500}
					created, err := model.SplitWithRatio(target, axis, ratios[rng.Intn(len(ratios))], operationBounds, metrics)
					if err == nil {
						if created <= maxID {
							t.Fatalf("step %d reused/non-monotonic ID %d after %d", step, created, maxID)
						}
						if _, duplicate := allocated[created]; duplicate {
							t.Fatalf("step %d reused ID %d", step, created)
						}
						allocated[created] = struct{}{}
						maxID = created
					} else if !errors.Is(err, ErrSplitTooSmall) {
						t.Fatalf("step %d split: %v", step, err)
					}
				case 1:
					if err := model.Focus(ids[rng.Intn(len(ids))]); err != nil {
						t.Fatalf("step %d focus: %v", step, err)
					}
				case 2:
					if _, err := model.FocusNext(); err != nil {
						t.Fatalf("step %d next focus: %v", step, err)
					}
				case 3:
					direction := Direction(rng.Intn(4) + int(FocusLeft))
					if _, err := model.FocusDirection(direction, operationBounds, metrics); err != nil && !errors.Is(err, ErrNoPaneInDirection) {
						t.Fatalf("step %d directional focus: %v", step, err)
					}
				case 4:
					if len(ids) > 1 {
						if _, err := model.Close(ids[rng.Intn(len(ids))]); err != nil {
							t.Fatalf("step %d close: %v", step, err)
						}
					}
				case 5:
					// Exercise compressed projections without changing the topology.
				}

				if err := model.CheckInvariants(); err != nil {
					t.Fatalf("step %d invariants: %v", step, err)
				}
				bounds := PixelRect{X: rng.Intn(5), Y: rng.Intn(5), Width: rng.Intn(500), Height: rng.Intn(300)}
				layout, err := model.Layout(bounds, metrics)
				if err != nil {
					t.Fatalf("step %d layout: %v", step, err)
				}
				assertLayoutPartition(t, step, bounds, model.PaneIDs(), layout)
			}
		})
	}
}

func randomTestName(seed int64) string {
	const digits = "0123456789"
	if seed == 0 {
		return "seed_0"
	}
	var reversed [20]byte
	i := len(reversed)
	for seed > 0 {
		i--
		reversed[i] = digits[seed%10]
		seed /= 10
	}
	return "seed_" + string(reversed[i:])
}

func assertLayoutPartition(t *testing.T, step int, bounds PixelRect, ids []PaneID, layout Layout) {
	t.Helper()
	if len(layout.Panes) != len(ids) {
		t.Fatalf("step %d pane geometry count = %d, want %d", step, len(layout.Panes), len(ids))
	}
	if len(layout.Dividers) != len(ids)-1 {
		t.Fatalf("step %d divider count = %d, want %d", step, len(layout.Dividers), len(ids)-1)
	}
	for i, pane := range layout.Panes {
		if pane.Pane != ids[i] {
			t.Fatalf("step %d layout order[%d] = %d, want %d", step, i, pane.Pane, ids[i])
		}
		if !rectInside(pane.Pixels, bounds) {
			t.Fatalf("step %d pane %d rect %#v outside %#v", step, pane.Pane, pane.Pixels, bounds)
		}
	}
	for i := range layout.Panes {
		for j := i + 1; j < len(layout.Panes); j++ {
			if positiveOverlap(layout.Panes[i].Pixels, layout.Panes[j].Pixels) {
				t.Fatalf("step %d panes %d and %d overlap: %#v %#v", step, layout.Panes[i].Pane, layout.Panes[j].Pane, layout.Panes[i].Pixels, layout.Panes[j].Pixels)
			}
		}
	}
}

func rectInside(rect, bounds PixelRect) bool {
	return rect.Width >= 0 && rect.Height >= 0 && rect.X >= bounds.X && rect.Y >= bounds.Y && rect.Right() <= bounds.Right() && rect.Bottom() <= bounds.Bottom()
}

func positiveOverlap(a, b PixelRect) bool {
	return a.X < b.Right() && b.X < a.Right() && a.Y < b.Bottom() && b.Y < a.Bottom()
}

func TestLayoutWithMetricsResolvesEqualPixelRectanglesPerPane(t *testing.T) {
	model := NewModel()
	bounds := PixelRect{Width: 401, Height: 200}
	second, err := model.Split(1, SplitColumns, bounds, testMetrics)
	if err != nil {
		t.Fatal(err)
	}
	metrics := map[PaneID]CellMetrics{
		1:      {CellWidth: 8, CellHeight: 16},
		second: {CellWidth: 10, CellHeight: 20},
	}
	resolve := func(id PaneID) (CellMetrics, bool) {
		value, ok := metrics[id]
		return value, ok
	}
	layout, err := model.LayoutWithMetrics(bounds, resolve)
	if err != nil {
		t.Fatal(err)
	}
	if layout.Panes[0].Pixels.Width != layout.Panes[1].Pixels.Width || layout.Panes[0].Pixels.Height != layout.Panes[1].Pixels.Height {
		t.Fatalf("pane rectangles differ: %#v", layout.Panes)
	}
	if got, want := layout.Panes[0], (PaneGeometry{Pane: 1, Pixels: PixelRect{Width: 200, Height: 200}, Cols: 25, Rows: 12}); got != want {
		t.Fatalf("first geometry = %#v, want %#v", got, want)
	}
	if got, want := layout.Panes[1], (PaneGeometry{Pane: second, Pixels: PixelRect{X: 201, Width: 200, Height: 200}, Cols: 20, Rows: 10}); got != want {
		t.Fatalf("second geometry = %#v, want %#v", got, want)
	}

	uniform, err := model.Layout(bounds, testMetrics)
	if err != nil {
		t.Fatal(err)
	}
	resolvedUniform, err := model.LayoutWithMetrics(bounds, UniformCellMetrics(testMetrics))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resolvedUniform, uniform) {
		t.Fatalf("uniform wrapper changed behavior: resolved=%#v wrapper=%#v", resolvedUniform, uniform)
	}

	delete(metrics, second)
	if _, err := model.LayoutWithMetrics(bounds, resolve); !errors.Is(err, ErrPaneNotFound) {
		t.Fatalf("missing pane metrics error = %v, want %v", err, ErrPaneNotFound)
	}
	metrics[second] = CellMetrics{}
	if _, err := model.LayoutWithMetrics(bounds, resolve); !errors.Is(err, ErrInvalidGeometry) {
		t.Fatalf("invalid pane metrics error = %v, want %v", err, ErrInvalidGeometry)
	}
}
