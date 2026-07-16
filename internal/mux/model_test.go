package mux

import (
	"errors"
	"reflect"
	"testing"
)

var testMetrics = CellMetrics{CellWidth: 8, CellHeight: 16}

func TestModelInitialIdentityAndMonotonicAllocation(t *testing.T) {
	model := NewModel()
	if model.TabID() == 0 {
		t.Fatal("implicit tab ID must be nonzero")
	}
	if got, want := model.PaneIDs(), []PaneID{1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("initial panes = %v, want %v", got, want)
	}
	if model.FocusedPane() != 1 {
		t.Fatalf("initial focus = %d, want 1", model.FocusedPane())
	}

	bounds := PixelRect{Width: 641, Height: 385}
	second, err := model.Split(1, SplitColumns, bounds, testMetrics)
	if err != nil {
		t.Fatalf("split root: %v", err)
	}
	third, err := model.Split(second, SplitRows, bounds, testMetrics)
	if err != nil {
		t.Fatalf("split second pane: %v", err)
	}
	if second != 2 || third != 3 {
		t.Fatalf("allocated pane IDs = %d, %d, want 2, 3", second, third)
	}
	if got, want := model.PaneIDs(), []PaneID{1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("visual DFS panes = %v, want %v", got, want)
	}
	if err := model.CheckInvariants(); err != nil {
		t.Fatalf("invariants: %v", err)
	}
}

func TestModelSplitValidationIsAtomic(t *testing.T) {
	tests := []struct {
		name    string
		axis    SplitAxis
		ratio   SplitRatio
		bounds  PixelRect
		metrics CellMetrics
		wantErr error
	}{
		{name: "invalid axis", axis: 99, ratio: DefaultSplitRatio, bounds: PixelRect{Width: 161, Height: 96}, metrics: testMetrics, wantErr: ErrInvalidAxis},
		{name: "zero ratio", axis: SplitColumns, ratio: 0, bounds: PixelRect{Width: 161, Height: 96}, metrics: testMetrics, wantErr: ErrInvalidRatio},
		{name: "full ratio", axis: SplitColumns, ratio: RatioScale, bounds: PixelRect{Width: 161, Height: 96}, metrics: testMetrics, wantErr: ErrInvalidRatio},
		{name: "invalid metrics", axis: SplitColumns, ratio: DefaultSplitRatio, bounds: PixelRect{Width: 161, Height: 96}, metrics: CellMetrics{}, wantErr: ErrInvalidGeometry},
		{name: "too few columns", axis: SplitColumns, ratio: DefaultSplitRatio, bounds: PixelRect{Width: 160, Height: 96}, metrics: testMetrics, wantErr: ErrSplitTooSmall},
		{name: "too few rows", axis: SplitRows, ratio: DefaultSplitRatio, bounds: PixelRect{Width: 160, Height: 96}, metrics: testMetrics, wantErr: ErrSplitTooSmall},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := NewModel()
			beforeIDs := model.PaneIDs()
			beforeFocus := model.FocusedPane()
			if _, err := model.SplitWithRatio(1, test.axis, test.ratio, test.bounds, test.metrics); !errors.Is(err, test.wantErr) {
				t.Fatalf("SplitWithRatio error = %v, want %v", err, test.wantErr)
			}
			if got := model.PaneIDs(); !reflect.DeepEqual(got, beforeIDs) {
				t.Fatalf("rejected split changed panes: before %v after %v", beforeIDs, got)
			}
			if model.FocusedPane() != beforeFocus {
				t.Fatalf("rejected split changed focus: before %d after %d", beforeFocus, model.FocusedPane())
			}
			created, err := model.Split(1, SplitColumns, PixelRect{Width: 161, Height: 96}, testMetrics)
			if err != nil {
				t.Fatalf("split after rejection: %v", err)
			}
			if created != 2 {
				t.Fatalf("rejected split consumed ID: next successful ID = %d, want 2", created)
			}
		})
	}
}

func TestFocusExplicitNextAndDirectional(t *testing.T) {
	model := NewModel()
	bounds := PixelRect{Width: 641, Height: 385}
	second, err := model.Split(1, SplitColumns, bounds, testMetrics)
	if err != nil {
		t.Fatal(err)
	}
	third, err := model.Split(second, SplitRows, bounds, testMetrics)
	if err != nil {
		t.Fatal(err)
	}

	if err := model.Focus(1); err != nil {
		t.Fatalf("explicit focus: %v", err)
	}
	if got, err := model.FocusNext(); err != nil || got != second {
		t.Fatalf("FocusNext from pane 1 = %d, %v; want %d, nil", got, err, second)
	}
	if got, err := model.FocusNext(); err != nil || got != third {
		t.Fatalf("FocusNext from pane 2 = %d, %v; want %d, nil", got, err, third)
	}
	if got, err := model.FocusNext(); err != nil || got != 1 {
		t.Fatalf("FocusNext wrap = %d, %v; want 1, nil", got, err)
	}

	if got, err := model.FocusDirection(FocusRight, bounds, testMetrics); err != nil || got != second {
		t.Fatalf("right from pane 1 = %d, %v; want %d, nil", got, err, second)
	}
	if got, err := model.FocusDirection(FocusDown, bounds, testMetrics); err != nil || got != third {
		t.Fatalf("down from pane 2 = %d, %v; want %d, nil", got, err, third)
	}
	if got, err := model.FocusDirection(FocusUp, bounds, testMetrics); err != nil || got != second {
		t.Fatalf("up from pane 3 = %d, %v; want %d, nil", got, err, second)
	}
	if got, err := model.FocusDirection(FocusDown, bounds, testMetrics); err != nil || got != third {
		t.Fatalf("down from pane 2 after up = %d, %v; want %d, nil", got, err, third)
	}
	if got, err := model.FocusDirection(FocusLeft, bounds, testMetrics); err != nil || got != 1 {
		t.Fatalf("left from pane 3 = %d, %v; want 1, nil", got, err)
	}
	if got, err := model.FocusDirection(FocusLeft, bounds, testMetrics); !errors.Is(err, ErrNoPaneInDirection) || got != 0 {
		t.Fatalf("left edge result = %d, %v; want 0, %v", got, err, ErrNoPaneInDirection)
	}
	if model.FocusedPane() != 1 {
		t.Fatalf("failed directional focus changed focus to %d", model.FocusedPane())
	}
	if err := model.Focus(999); !errors.Is(err, ErrPaneNotFound) {
		t.Fatalf("focus missing pane error = %v, want %v", err, ErrPaneNotFound)
	}
}

func TestSetSplitRatioIsTargetedAndAtomic(t *testing.T) {
	model := NewModel()
	bounds := PixelRect{Width: 801, Height: 481}
	second, err := model.Split(1, SplitColumns, bounds, testMetrics)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := model.Split(second, SplitRows, bounds, testMetrics); err != nil {
		t.Fatal(err)
	}
	before, err := model.Layout(bounds, testMetrics)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Dividers) != 2 || before.Dividers[0].Split == 0 || before.Dividers[1].Split == 0 || before.Dividers[0].Split == before.Dividers[1].Split {
		t.Fatalf("split identities = %#v", before.Dividers)
	}
	nestedBefore := before.Dividers[1]
	if err := model.SetSplitRatio(before.Dividers[0].Split, 6_000, bounds, testMetrics); err != nil {
		t.Fatalf("resize root split: %v", err)
	}
	after, err := model.Layout(bounds, testMetrics)
	if err != nil {
		t.Fatal(err)
	}
	if after.Dividers[0].Pixels.X <= before.Dividers[0].Pixels.X {
		t.Fatalf("root divider did not move right: before=%#v after=%#v", before.Dividers[0], after.Dividers[0])
	}
	if after.Dividers[1].Split != nestedBefore.Split || after.Dividers[1].Axis != nestedBefore.Axis {
		t.Fatalf("nested split identity changed: before=%#v after=%#v", nestedBefore, after.Dividers[1])
	}
	accepted := after
	for _, test := range []struct {
		name  string
		split SplitID
		ratio SplitRatio
		err   error
	}{
		{name: "missing", split: 999, ratio: DefaultSplitRatio, err: ErrSplitNotFound},
		{name: "invalid ratio", split: after.Dividers[0].Split, ratio: 0, err: ErrInvalidRatio},
		{name: "below descendant minimum", split: after.Dividers[0].Split, ratio: 9_500, err: ErrSplitTooSmall},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := model.SetSplitRatio(test.split, test.ratio, bounds, testMetrics); !errors.Is(err, test.err) {
				t.Fatalf("SetSplitRatio error = %v, want %v", err, test.err)
			}
			got, layoutErr := model.Layout(bounds, testMetrics)
			if layoutErr != nil || !reflect.DeepEqual(got, accepted) {
				t.Fatalf("rejected ratio mutated layout: got=%#v err=%v want=%#v", got, layoutErr, accepted)
			}
		})
	}
}

func TestCloseCollapseFinalEmptyAndNeverReuseIDs(t *testing.T) {
	model := NewModel()
	bounds := PixelRect{Width: 641, Height: 385}
	second, err := model.Split(1, SplitColumns, bounds, testMetrics)
	if err != nil {
		t.Fatal(err)
	}
	third, err := model.Split(second, SplitRows, bounds, testMetrics)
	if err != nil {
		t.Fatal(err)
	}

	if err := model.Focus(second); err != nil {
		t.Fatal(err)
	}
	result, err := model.Close(second)
	if err != nil {
		t.Fatalf("close nested pane: %v", err)
	}
	if !result.Closed || result.Empty {
		t.Fatalf("close result = %#v", result)
	}
	if got, want := model.PaneIDs(), []PaneID{1, third}; !reflect.DeepEqual(got, want) {
		t.Fatalf("collapsed panes = %v, want %v", got, want)
	}
	if err := model.CheckInvariants(); err != nil {
		t.Fatalf("invariants after collapse: %v", err)
	}

	repeated, err := model.Close(second)
	if err != nil {
		t.Fatalf("idempotent close: %v", err)
	}
	if repeated.Closed {
		t.Fatalf("repeated close reported a transition: %#v", repeated)
	}

	fourth, err := model.Split(1, SplitRows, bounds, testMetrics)
	if err != nil {
		t.Fatalf("split after close: %v", err)
	}
	if fourth != 4 {
		t.Fatalf("closed ID was reused: allocated %d, want 4", fourth)
	}

	for _, pane := range append([]PaneID(nil), model.PaneIDs()...) {
		result, err = model.Close(pane)
		if err != nil {
			t.Fatalf("close pane %d: %v", pane, err)
		}
	}
	if !result.Empty || !model.Empty() || model.FocusedPane() != 0 || len(model.PaneIDs()) != 0 {
		t.Fatalf("final close did not produce empty model: result=%#v focus=%d panes=%v", result, model.FocusedPane(), model.PaneIDs())
	}
	if _, err := model.FocusNext(); !errors.Is(err, ErrEmptyModel) {
		t.Fatalf("FocusNext on empty error = %v, want %v", err, ErrEmptyModel)
	}
	if err := model.CheckInvariants(); err != nil {
		t.Fatalf("empty invariants: %v", err)
	}
}

func TestModelInvariantCheckerRejectsMalformedTree(t *testing.T) {
	model := NewModel()
	model.focused = 99
	if err := model.CheckInvariants(); !errors.Is(err, ErrInvariant) {
		t.Fatalf("malformed focus error = %v, want invariant error", err)
	}

	model = NewModel()
	model.root = branchNode(1, SplitColumns, DefaultSplitRatio, model.root, nil)
	model.nextSplitID = 2
	if err := model.CheckInvariants(); !errors.Is(err, ErrInvariant) {
		t.Fatalf("nil child error = %v, want invariant error", err)
	}
}
