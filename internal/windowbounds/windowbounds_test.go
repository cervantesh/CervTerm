package windowbounds

import (
	"math"
	"reflect"
	"testing"

	"cervterm/internal/layoutstate"
)

func standardPolicy() Policy {
	return Policy{FallbackWidth: 800, FallbackHeight: 600, MinWidth: 200, MinHeight: 100, ChromeHeight: 30, MinVisibleChromeX: 50, MinVisibleChromeY: 20}
}

func monitor(id, name string, x, y, w, h int) Monitor {
	return Monitor{ID: id, Name: name, WorkArea: Rect{X: x, Y: y, Width: w, Height: h}, ScaleX: 1, ScaleY: 1}
}

func TestRecoverHintSelectionAndNoDPIScaling(t *testing.T) {
	monitors := []Monitor{monitor("a", "Left", 0, 0, 1000, 800), monitor("b", "Right", 1000, 0, 1000, 800)}
	monitors[1].ScaleX, monitors[1].ScaleY = 2, 1.5
	for _, tc := range []struct {
		name string
		hint string
	}{
		{"id", "b"},
		{"unique name", "Right"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saved := layoutstate.Bounds{X: 1100, Y: 100, Width: 400, Height: 300, MonitorHint: tc.hint}
			plan, err := Recover(saved, monitors, standardPolicy())
			if err != nil {
				t.Fatal(err)
			}
			if plan.MonitorID != "b" || !plan.HintMatched || plan.ScaleX != 2 || plan.ScaleY != 1.5 {
				t.Fatalf("unexpected plan: %+v", plan)
			}
			if plan.Bounds.X != 1100 || plan.Bounds.Y != 100 || plan.Bounds.Width != 400 || plan.Bounds.Height != 300 {
				t.Fatalf("saved geometry was scaled: %+v", plan.Bounds)
			}
			if tc.hint == "b" && plan.Outcome != Kept {
				t.Fatalf("outcome = %q", plan.Outcome)
			}
			if plan.Bounds.MonitorHint != "b" {
				t.Fatalf("hint = %q", plan.Bounds.MonitorHint)
			}
		})
	}
}

func TestRecoverDuplicateNameFallsBackToIntersection(t *testing.T) {
	monitors := []Monitor{monitor("a", "same", 0, 0, 100, 100), monitor("b", "same", 100, 0, 100, 100)}
	plan, err := Recover(layoutstate.Bounds{X: 120, Y: 10, Width: 50, Height: 50, MonitorHint: "same"}, monitors, Policy{60, 60, 20, 20, 10, 5, 5})
	if err != nil {
		t.Fatal(err)
	}
	if plan.MonitorID != "b" || plan.HintMatched {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestRecoverIntersectionTieBreaks(t *testing.T) {
	policy := Policy{40, 40, 20, 20, 10, 5, 5}
	saved := layoutstate.Bounds{X: 75, Y: 0, Width: 50, Height: 50}
	a := monitor("a", "", 0, 0, 100, 100)
	b := monitor("b", "", 100, 0, 100, 100)
	b.Primary = true
	plan, err := Recover(saved, []Monitor{a, b}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if plan.MonitorID != "b" {
		t.Fatalf("primary tie-break selected %q", plan.MonitorID)
	}
	b.Primary = false
	plan, err = Recover(saved, []Monitor{b, a}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if plan.MonitorID != "a" {
		t.Fatalf("lexical tie-break selected %q", plan.MonitorID)
	}
}

func TestRecoverDefaultIsOrderIndependent(t *testing.T) {
	policy := standardPolicy()
	saved := layoutstate.Bounds{X: 10000, Y: 10000, Width: 300, Height: 200}
	a := monitor("a", "", -1000, -500, 800, 600)
	b := monitor("b", "", 0, 0, 1000, 800)
	for _, monitors := range [][]Monitor{{b, a}, {a, b}} {
		plan, err := Recover(saved, monitors, policy)
		if err != nil {
			t.Fatal(err)
		}
		if plan.MonitorID != "a" || plan.Outcome != FallbackOffscreen || plan.Bounds != (layoutstate.Bounds{X: -1000, Y: -500, Width: 800, Height: 600, MonitorHint: "a"}) {
			t.Fatalf("unexpected plan: %+v", plan)
		}
	}
	b.Primary = true
	plan, err := Recover(saved, []Monitor{a, b}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if plan.MonitorID != "b" {
		t.Fatalf("primary not selected: %+v", plan)
	}
}

func TestRecoverClampAndSmallWorkArea(t *testing.T) {
	policy := standardPolicy()
	m := monitor("m", "", -500, -300, 150, 80)
	plan, err := Recover(layoutstate.Bounds{X: -1000, Y: -1000, Width: 1000, Height: 20, MonitorHint: "m"}, []Monitor{m}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Bounds.Width != 150 || plan.Bounds.Height != 80 || plan.Outcome != Clamped {
		t.Fatalf("unexpected dimensions: %+v", plan)
	}
	if plan.Bounds.X > -350 || plan.Bounds.X+plan.Bounds.Width < -450 {
		t.Fatalf("horizontal chrome not visible: %+v", plan.Bounds)
	}
}

func TestRecoverOffscreenHintClampsButNoHintFallsBack(t *testing.T) {
	m := monitor("m", "Display", 0, 0, 1000, 800)
	saved := layoutstate.Bounds{X: 5000, Y: 5000, Width: 300, Height: 200, MonitorHint: "m"}
	plan, err := Recover(saved, []Monitor{m}, standardPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if !plan.HintMatched || plan.Outcome != Clamped {
		t.Fatalf("unexpected hinted plan: %+v", plan)
	}
	saved.MonitorHint = ""
	plan, err = Recover(saved, []Monitor{m}, standardPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if plan.HintMatched || plan.Outcome != FallbackOffscreen || plan.Bounds.X != 100 || plan.Bounds.Y != 100 {
		t.Fatalf("unexpected fallback: %+v", plan)
	}
}

func TestRecoverInvalidSavedFallback(t *testing.T) {
	m := monitor("m", "", -100, -100, 1000, 800)
	m.Primary = true
	for _, saved := range []layoutstate.Bounds{{Width: 0, Height: 10}, {X: math.MaxInt, Width: 2, Height: 10}} {
		plan, err := Recover(saved, []Monitor{m}, standardPolicy())
		if err != nil {
			t.Fatal(err)
		}
		if plan.Outcome != FallbackInvalid || plan.Bounds != (layoutstate.Bounds{X: 0, Y: 0, Width: 800, Height: 600, MonitorHint: "m"}) {
			t.Fatalf("unexpected fallback: %+v", plan)
		}
	}
}

func TestRecoverRejectsBadInputs(t *testing.T) {
	validMonitor := monitor("m", "", 0, 0, 100, 100)
	validPolicy := Policy{40, 40, 20, 20, 10, 5, 5}
	badMonitors := [][]Monitor{
		nil,
		{{ID: "", WorkArea: Rect{Width: 1, Height: 1}, ScaleX: 1, ScaleY: 1}},
		{validMonitor, validMonitor},
		{{ID: "m", WorkArea: Rect{X: math.MaxInt, Width: 2, Height: 1}, ScaleX: 1, ScaleY: 1}},
		{{ID: "m", WorkArea: Rect{Width: 1, Height: 1}, ScaleX: math.NaN(), ScaleY: 1}},
	}
	many := make([]Monitor, MaxMonitors+1)
	for i := range many {
		many[i] = monitor(string(rune(i+1)), "", 0, 0, 1, 1)
	}
	badMonitors = append(badMonitors, many)
	p1, p2 := validMonitor, monitor("n", "", 0, 0, 100, 100)
	p1.Primary, p2.Primary = true, true
	badMonitors = append(badMonitors, []Monitor{p1, p2})
	for i, monitors := range badMonitors {
		if _, err := Recover(layoutstate.Bounds{Width: 1, Height: 1}, monitors, validPolicy); err == nil {
			t.Fatalf("bad monitors %d accepted", i)
		}
	}

	badPolicies := []Policy{{}, {40, 40, 20, 20, 10, 21, 5}, {40, 40, 20, 20, 10, 5, 11}, {40, 40, 20, 20, 21, 5, 5}}
	for i, policy := range badPolicies {
		if _, err := Recover(layoutstate.Bounds{Width: 1, Height: 1}, []Monitor{validMonitor}, policy); err == nil {
			t.Fatalf("bad policy %d accepted", i)
		}
	}
}

func TestRecoverDoesNotMutateAndIsDeterministic(t *testing.T) {
	monitors := []Monitor{monitor("b", "", 100, 0, 100, 100), monitor("a", "", 0, 0, 100, 100)}
	before := append([]Monitor(nil), monitors...)
	saved := layoutstate.Bounds{X: 80, Y: 10, Width: 40, Height: 40}
	policy := Policy{40, 40, 20, 20, 10, 5, 5}
	first, err := Recover(saved, monitors, policy)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		next, err := Recover(saved, monitors, policy)
		if err != nil {
			t.Fatal(err)
		}
		if next != first {
			t.Fatalf("non-deterministic: %+v != %+v", next, first)
		}
	}
	if !reflect.DeepEqual(monitors, before) {
		t.Fatalf("monitors mutated: %#v", monitors)
	}
}

func TestRecoverTinyWorkAreaKeepsBestEffortChromeVisible(t *testing.T) {
	monitor := monitor("tiny", "", -20, -10, 10, 5)
	saved := layoutstate.Bounds{X: -1000, Y: -1000, Width: 300, Height: 200, MonitorHint: "tiny"}
	plan, err := Recover(saved, []Monitor{monitor}, standardPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Bounds != (layoutstate.Bounds{X: -20, Y: -10, Width: 10, Height: 5, MonitorHint: "tiny"}) {
		t.Fatalf("bounds=%#v", plan.Bounds)
	}
}

func TestRecoverClampSaturatesAtMinimumCoordinates(t *testing.T) {
	monitor := monitor("edge", "", math.MinInt, math.MinInt, 100, 100)
	saved := layoutstate.Bounds{X: math.MinInt, Y: math.MinInt, Width: 1000, Height: 1000, MonitorHint: "edge"}
	plan, err := Recover(saved, []Monitor{monitor}, Policy{FallbackWidth: 80, FallbackHeight: 80, MinWidth: 20, MinHeight: 20, ChromeHeight: 10, MinVisibleChromeX: 5, MinVisibleChromeY: 5})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Bounds.X != math.MinInt || plan.Bounds.Y != math.MinInt || plan.Bounds.Width != 100 || plan.Bounds.Height != 100 {
		t.Fatalf("bounds=%#v", plan.Bounds)
	}
}
