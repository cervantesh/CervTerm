package accessibility

import (
	"errors"
	"reflect"
	"testing"
)

func TestRangeTextRectanglesComparisonAndStaleness(t *testing.T) {
	document := testDocument(t, 1)
	_, pane := testIDs()
	whole, err := NewRange(document, pane, 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	if text, err := whole.Text(document); err != nil || text != "A好\ne\u0301" {
		t.Fatalf("text=%q err=%v", text, err)
	}
	rectangles, err := whole.Rectangles(document)
	if err != nil {
		t.Fatal(err)
	}
	wantRects := []Rect{{X: 1, Y: 2, Width: 3, Height: 4}, {X: 4, Y: 2, Width: 6, Height: 4}, {X: 1, Y: 6, Width: 3, Height: 4}}
	if !reflect.DeepEqual(rectangles, wantRects) {
		t.Fatalf("rectangles=%#v want=%#v", rectangles, wantRects)
	}
	rectangles[0].X = 999
	again, _ := whole.Rectangles(document)
	if again[0].X != 1 {
		t.Fatal("caller mutated range rectangles")
	}

	middle, err := NewRange(document, pane, 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if text, _ := middle.Text(document); text != "好\n" {
		t.Fatalf("middle text=%q", text)
	}
	if comparison, err := CompareEndpoints(document, middle, true, whole, true); err != nil || comparison != 1 {
		t.Fatalf("comparison=%d err=%v", comparison, err)
	}
	if middle.Clone() != middle || middle.Span() != (Span{Start: 1, End: 3}) || middle.NodeID() != pane {
		t.Fatalf("range accessors drifted: %#v", middle)
	}

	replacement := testDocument(t, 1)
	if _, err := whole.Text(replacement); !errors.Is(err, ErrStaleRange) {
		t.Fatalf("same-generation replacement err=%v", err)
	}

	next := testDocument(t, 2)
	if _, err := whole.Text(next); !errors.Is(err, ErrStaleRange) {
		t.Fatalf("stale text err=%v", err)
	}
	if _, err := CompareEndpoints(document, whole, true, Range{providerID: 9, generation: 1, node: pane}, true); !errors.Is(err, ErrStaleRange) {
		t.Fatalf("stale comparison err=%v", err)
	}
}

func TestRangeRejectsInvalidNodeAndEndpoints(t *testing.T) {
	document := testDocument(t, 1)
	root, pane := testIDs()
	for _, span := range []Span{{Start: -1, End: 0}, {Start: 2, End: 1}, {Start: 0, End: 5}} {
		if _, err := NewRange(document, pane, span.Start, span.End); !errors.Is(err, ErrInvalidRange) {
			t.Fatalf("span=%#v err=%v", span, err)
		}
	}
	missing := root
	missing.Object = 99
	if _, err := NewRange(document, missing, 0, 0); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("missing node err=%v", err)
	}
}

func TestRangeUnitsMovementEndpointsAndFindText(t *testing.T) {
	document := testDocument(t, 1)
	_, pane := testIDs()
	value, err := NewRange(document, pane, 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	line, err := value.Expand(document, TextUnitLine)
	if err != nil || line.Span() != (Span{Start: 0, End: 3}) {
		t.Fatalf("line=%#v err=%v", line.Span(), err)
	}
	character, err := value.Expand(document, TextUnitCharacter)
	if err != nil || character.Span() != (Span{Start: 1, End: 2}) {
		t.Fatalf("character=%#v err=%v", character.Span(), err)
	}
	documentRange, err := value.Expand(document, TextUnitDocument)
	if err != nil || documentRange.Span() != (Span{Start: 0, End: 4}) {
		t.Fatalf("document=%#v err=%v", documentRange.Span(), err)
	}
	moved, count, err := line.Move(document, TextUnitLine, 1)
	if err != nil || count != 1 || moved.Span() != (Span{Start: 3, End: 4}) {
		t.Fatalf("moved=%#v count=%d err=%v", moved.Span(), count, err)
	}
	moved, count, err = value.MoveEndpoint(document, true, TextUnitCharacter, 2)
	if err != nil || count != 2 || moved.Span() != (Span{Start: 3, End: 3}) {
		t.Fatalf("start moved=%#v count=%d err=%v", moved.Span(), count, err)
	}
	moved, count, err = value.MoveEndpoint(document, false, TextUnitCharacter, -2)
	if err != nil || count != -2 || moved.Span() != (Span{Start: 1, End: 1}) {
		t.Fatalf("end moved=%#v count=%d err=%v", moved.Span(), count, err)
	}
	target, _ := NewRange(document, pane, 3, 4)
	moved, err = value.MoveEndpointTo(document, true, target, false)
	if err != nil || moved.Span() != (Span{Start: 4, End: 4}) {
		t.Fatalf("endpoint-to=%#v err=%v", moved.Span(), err)
	}
	found, ok, err := documentRange.FindText(document, "E\u0301", false, true)
	if err != nil || !ok || found.Span() != (Span{Start: 3, End: 4}) {
		t.Fatalf("found=%#v ok=%v err=%v", found.Span(), ok, err)
	}
	equal, err := found.Equal(document, target)
	if err != nil || !equal {
		t.Fatalf("equal=%v err=%v", equal, err)
	}
	if _, err := value.Expand(document, TextUnit(99)); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("invalid unit err=%v", err)
	}
}
