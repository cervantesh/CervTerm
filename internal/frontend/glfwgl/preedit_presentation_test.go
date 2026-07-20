//go:build glfw

package glfwgl

import (
	"strings"
	"testing"

	"cervterm/internal/ime"
	termmux "cervterm/internal/mux"
)

func activePreedit(text string, cursor, targetStart, targetEnd int) ime.Snapshot {
	return ime.Snapshot{
		Active: true, Target: ime.Target{Kind: ime.TargetPane, ID: 1, Activation: 1},
		Revision: 4, Text: text, Runes: []rune(text), CursorRune: cursor,
		TargetRuneSpan: ime.Span{Start: targetStart, End: targetEnd},
	}
}

func TestPreeditPresentationKeepsClustersAndMapsCaretAndTarget(t *testing.T) {
	text := "A\u0301日本👨‍👩‍👧‍👦"
	runes := []rune(text)
	presentation := preparePreeditPresentation(activePreedit(text, len(runes), 2, 4), 32)
	if !presentation.active || presentation.truncated || presentation.cells != 7 || presentation.caretCell != 7 {
		t.Fatalf("presentation=%#v", presentation)
	}
	if len(presentation.clusters) != 4 || presentation.clusters[0].text != "A\u0301" || presentation.clusters[3].text != "👨‍👩‍👧‍👦" {
		t.Fatalf("clusters=%#v", presentation.clusters)
	}
	if len(presentation.targetSpans) != 1 || presentation.targetSpans[0] != (preeditCellSpan{start: 1, end: 5}) {
		t.Fatalf("target spans=%#v", presentation.targetSpans)
	}
}

func TestPreeditPresentationClipsOnlyAtClusterBoundaries(t *testing.T) {
	text := "A\u0301日本😀"
	presentation := preparePreeditPresentation(activePreedit(text, len([]rune(text)), 0, len([]rune(text))), 3)
	if !presentation.active || !presentation.truncated || presentation.cells != 3 || len(presentation.clusters) != 2 {
		t.Fatalf("presentation=%#v", presentation)
	}
	if presentation.clusters[0].text != "A\u0301" || presentation.clusters[1].text != "日" || presentation.caretCell != 3 || len(presentation.targetSpans) != 1 || presentation.targetSpans[0] != (preeditCellSpan{start: 0, end: 3}) {
		t.Fatalf("clusters=%#v caret=%d target=%#v", presentation.clusters, presentation.caretCell, presentation.targetSpans)
	}
}

func TestPreeditPresentationAcceptsRTLAndBoundsVisualWork(t *testing.T) {
	rtl := "שלום"
	presentation := preparePreeditPresentation(activePreedit(rtl, len([]rune(rtl)), 0, len([]rune(rtl))), 20)
	if !presentation.active || presentation.cells != 4 || len(presentation.clusters) != 4 || presentation.caretCell != 0 || len(presentation.targetSpans) != 1 || presentation.targetSpans[0] != (preeditCellSpan{start: 0, end: 4}) {
		t.Fatalf("rtl=%#v", presentation)
	}
	for logical, wantStart := range []int{3, 2, 1, 0} {
		if presentation.clusters[logical].cellStart != wantStart {
			t.Fatalf("rtl cluster %d start=%d want=%d", logical, presentation.clusters[logical].cellStart, wantStart)
		}
	}

	long := strings.Repeat("x", maxPreeditVisualCells+50)
	presentation = preparePreeditPresentation(activePreedit(long, len(long), 0, len(long)), len(long))
	if !presentation.truncated || presentation.cells != maxPreeditVisualCells || len(presentation.clusters) != maxPreeditVisualCells {
		t.Fatalf("bounded cells=%d clusters=%d truncated=%v", presentation.cells, len(presentation.clusters), presentation.truncated)
	}
}

func TestPreeditPresentationRejectsInactiveEmptyAndZeroGeometry(t *testing.T) {
	for _, snapshot := range []ime.Snapshot{
		{},
		{Active: true, Target: ime.Target{Kind: ime.TargetPane, ID: 1, Activation: 1}},
		activePreedit("x", 1, 0, 1),
	} {
		available := 8
		if snapshot.Active && snapshot.Text == "x" {
			available = 0
		}
		if got := preparePreeditPresentation(snapshot, available); got.active || len(got.clusters) != 0 {
			t.Fatalf("unexpected presentation=%#v", got)
		}
	}
}

func TestCompositionPresentationNotifiesOnlyOnVisibleStateMutation(t *testing.T) {
	target := ime.Target{Kind: ime.TargetPane, ID: 1, Activation: 1}
	var coordinator compositionCoordinator
	coordinator.bind(func() (ime.Target, error) { return target, nil }, func(ime.Target, string) error { return nil })
	notifications := 0
	var revisions []uint64
	coordinator.bindPresentation(func(snapshot ime.Snapshot) {
		notifications++
		revisions = append(revisions, snapshot.Revision)
	})
	generation, err := coordinator.start()
	if err != nil {
		t.Fatal(err)
	}
	if err := coordinator.update(generation, ime.NativeUpdate{UTF16: utf16Text("日本"), CursorUTF16: 2}); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.update(generation+1, ime.NativeUpdate{}); err == nil {
		t.Fatal("stale update succeeded")
	}
	if err := coordinator.cancel(ime.CancelExplicit); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.cancel(ime.CancelExplicit); err != nil {
		t.Fatal(err)
	}
	if notifications != 3 || len(revisions) != 3 || !(revisions[0] < revisions[1] && revisions[1] < revisions[2]) {
		t.Fatalf("notifications=%d revisions=%v", notifications, revisions)
	}
}

func TestAppPreeditMutationRequestsOneOnDemandFrame(t *testing.T) {
	app := &App{}
	app.initCompositionCoordinator()
	target := ime.Target{Kind: ime.TargetPane, ID: 1, Activation: 1}
	app.composition.bind(func() (ime.Target, error) { return target, nil }, func(ime.Target, string) error { return nil })
	generation, err := app.composition.start()
	if err != nil || !app.needsRedraw {
		t.Fatalf("start err=%v redraw=%v", err, app.needsRedraw)
	}
	app.needsRedraw = false
	if err := app.composition.update(generation+1, ime.NativeUpdate{}); err == nil || app.needsRedraw {
		t.Fatalf("stale update err=%v redraw=%v", err, app.needsRedraw)
	}
	if err := app.composition.update(generation, ime.NativeUpdate{UTF16: utf16Text("日"), CursorUTF16: 1}); err != nil || !app.needsRedraw {
		t.Fatalf("update err=%v redraw=%v", err, app.needsRedraw)
	}
	app.needsRedraw = false
	if err := app.composition.cancel(ime.CancelExplicit); err != nil || !app.needsRedraw {
		t.Fatalf("cancel err=%v redraw=%v", err, app.needsRedraw)
	}
}

func TestPreeditMixedDirectionTargetKeepsDisjointVisualSpans(t *testing.T) {
	text := "aאבb"
	presentation := preparePreeditPresentation(activePreedit(text, 2, 0, 2), 20)
	if len(presentation.targetSpans) != 2 {
		t.Fatalf("target spans=%#v clusters=%#v", presentation.targetSpans, presentation.clusters)
	}
	if presentation.targetSpans[0] != (preeditCellSpan{start: 0, end: 1}) || presentation.targetSpans[1] != (preeditCellSpan{start: 2, end: 3}) {
		t.Fatalf("target spans=%#v", presentation.targetSpans)
	}
}

func TestPreeditTrailingCaretStaysInsideClip(t *testing.T) {
	presentation := preparePreeditPresentation(activePreedit("abc", 3, 0, 0), 3)
	if offset := preeditCaretOffset(presentation, 10, 2); offset != 28 || offset+2 > 30 {
		t.Fatalf("caret offset=%v", offset)
	}
}

func TestPreeditRedrawMarksOnlyOwningProjection(t *testing.T) {
	var log []string
	controller := newWindowController(processServices{}, fakeNativePump{log: &log})
	first, second := &App{windowID: 1, controller: controller}, &App{windowID: 2, controller: controller}
	if err := controller.attachApp(1, &fakeNativeWindow{id: "one", log: &log}, first, func([]termmux.Event) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if err := controller.attachApp(2, &fakeNativeWindow{id: "two", log: &log}, second, func([]termmux.Event) bool { return true }); err != nil {
		t.Fatal(err)
	}
	controller.clearDamage(1)
	controller.clearDamage(2)
	first.initCompositionCoordinator()
	target := ime.Target{Kind: ime.TargetPane, ID: 1, Activation: 1}
	first.composition.bind(func() (ime.Target, error) { return target, nil }, func(ime.Target, string) error { return nil })
	if _, err := first.composition.start(); err != nil {
		t.Fatal(err)
	}
	if !controller.windows[1].dirty || controller.windows[2].dirty || !first.needsRedraw || second.needsRedraw {
		t.Fatalf("first=%#v second=%#v", controller.windows[1], controller.windows[2])
	}
}
