package fontdesc

import (
	"math"
	"testing"
)

func TestMetricProjectionIdentityAndFieldMutation(t *testing.T) {
	base, err := NewMetricProjection(1, 1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	baseID, _ := base.ID()
	mutations := []MetricProjection{
		{LineHeight: 1.1, CellWidth: 1},
		{LineHeight: 1, CellWidth: 1.1},
		{LineHeight: 1, CellWidth: 1, BaselineOffset: 1},
		{LineHeight: 1, CellWidth: 1, GlyphOffsetX: 1},
		{LineHeight: 1, CellWidth: 1, GlyphOffsetY: 1},
	}
	for index, mutation := range mutations {
		id, err := mutation.ID()
		if err != nil {
			t.Fatalf("mutation %d: %v", index, err)
		}
		if id == baseID {
			t.Fatalf("metric mutation %d did not change identity", index)
		}
	}
	negativeZero, err := NewMetricProjection(1, 1, math.Copysign(0, -1), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	negativeID, _ := negativeZero.ID()
	if negativeID != baseID {
		t.Fatal("negative zero changed metric identity")
	}
}

func TestMetricProjectionValidationAndBoundaries(t *testing.T) {
	if _, err := NewMetricProjection(MetricScaleMinimum, MetricScaleMaximum, MetricOffsetMinimum, MetricOffsetMaximum, 0); err != nil {
		t.Fatalf("inclusive boundaries rejected: %v", err)
	}
	cases := []MetricProjection{
		{LineHeight: 0.49, CellWidth: 1}, {LineHeight: 1, CellWidth: 3.01},
		{LineHeight: 1, CellWidth: 1, BaselineOffset: -65},
		{LineHeight: 1, CellWidth: 1, GlyphOffsetX: 65},
		{LineHeight: 1, CellWidth: 1, GlyphOffsetY: math.NaN()},
	}
	for index, value := range cases {
		if err := value.Validate(); err == nil {
			t.Fatalf("invalid projection %d accepted", index)
		}
	}
}

func TestMetricProjectionProjectsFixedCellGeometry(t *testing.T) {
	projection, err := NewMetricProjection(1.5, 1.25, 2, 3, -4)
	if err != nil {
		t.Fatal(err)
	}
	width, height, baseline := projection.ProjectCellMetrics(8, 16, 12)
	if width != 10 || height != 24 || baseline != 18 {
		t.Fatalf("projected metrics=%d/%d/%d, want 10/24/18", width, height, baseline)
	}
	clipped, _ := NewMetricProjection(0.5, 1, -64, 0, 0)
	_, height, baseline = clipped.ProjectCellMetrics(8, 16, 12)
	if baseline != 0 || height != 8 {
		t.Fatalf("clipped baseline metrics=%d/%d", height, baseline)
	}
}
