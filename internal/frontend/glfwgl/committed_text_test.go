//go:build glfw

package glfwgl

import (
	"errors"
	"strings"
	"testing"
	"time"

	"cervterm/internal/modal"
	"cervterm/internal/quickselect"
)

func TestCommittedTextRoutesWholeUTF8ToCapturedPaneOnce(t *testing.T) {
	app, factory := newRecordingActionApp(t)
	target, err := app.captureCommittedTextTarget()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.routeCommittedText(target, "日本語😀"); err != nil {
		t.Fatal(err)
	}
	if got := factory.sessions[0].text(); got != "日本語😀" {
		t.Fatalf("pane input=%q", got)
	}
	focused := app.focusedPane
	app.setFocusedPane(focused + 1)
	app.setFocusedPane(focused)
	if err := app.routeCommittedText(target, "old"); !errors.Is(err, errTextTargetStale) || factory.sessions[0].text() != "日本語😀" {
		t.Fatalf("focus-cycle route err=%v input=%q", err, factory.sessions[0].text())
	}
	stale, err := app.captureCommittedTextTarget()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.captureCommittedTextTarget(); err != nil {
		t.Fatal(err)
	}
	if err := app.routeCommittedText(stale, "old"); !errors.Is(err, errTextTargetStale) || factory.sessions[0].text() != "日本語😀" {
		t.Fatalf("new-capture route err=%v input=%q", err, factory.sessions[0].text())
	}
}

func TestCommittedTextRoutesByStableModalAndSearchActivation(t *testing.T) {
	app, _ := newRecordingActionApp(t)
	paneTarget, err := app.captureCommittedTextTarget()
	if err != nil {
		t.Fatal(err)
	}
	if !app.modal.Open(modal.ModeCommandPalette, modal.PaneIdentity(app.focusedPane), modal.FocusIdentity(app.focusedPane), []modal.Entry{{ID: "one", Label: "日本語"}}) {
		t.Fatal("modal open failed")
	}
	if err := app.routeCommittedText(paneTarget, "stale"); !errors.Is(err, errTextTargetStale) {
		t.Fatalf("pane target survived modal open: %v", err)
	}
	modalTarget, err := app.captureCommittedTextTarget()
	if err != nil {
		t.Fatal(err)
	}
	app.modal.AppendRune('x') // content revision changes, activation does not.
	if err := app.routeCommittedText(modalTarget, "日本"); err != nil || string(app.modal.Snapshot().Query) != "x日本" {
		t.Fatalf("modal query=%q err=%v", string(app.modal.Snapshot().Query), err)
	}
	if !app.modal.Replace(modal.ModeLaunchMenu, []modal.Entry{{ID: "two", Label: "two"}}) {
		t.Fatal("modal replace failed")
	}
	if err := app.routeCommittedText(modalTarget, "stale"); !errors.Is(err, errTextTargetStale) {
		t.Fatalf("replaced modal route err=%v", err)
	}
	app.modal.Close()

	app.search.redraw = func() {}
	if !app.search.open() {
		t.Fatal("search open failed")
	}
	searchTarget, err := app.captureCommittedTextTarget()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.routeCommittedText(searchTarget, "한글"); err != nil || string(app.search.query) != "한글" {
		t.Fatalf("search query=%q err=%v", string(app.search.query), err)
	}
	if !app.modal.Open(modal.ModeCommandPalette, modal.PaneIdentity(app.focusedPane), modal.FocusIdentity(app.focusedPane), []modal.Entry{{ID: "blocking", Label: "blocking"}}) {
		t.Fatal("blocking modal open failed")
	}
	if err := app.routeCommittedText(searchTarget, "stale"); !errors.Is(err, errTextTargetStale) {
		t.Fatalf("search target survived modal open: %v", err)
	}
	app.modal.Close()
	app.search.close()
	if !app.search.open() {
		t.Fatal("search reopen failed")
	}
	if err := app.routeCommittedText(searchTarget, "stale"); !errors.Is(err, errTextTargetStale) {
		t.Fatalf("reopened search route err=%v", err)
	}
}

func TestCommittedTextRejectsInvalidInputAtomically(t *testing.T) {
	app, factory := newRecordingActionApp(t)
	target, _ := app.captureCommittedTextTarget()
	for _, text := range []string{"", "bad\ntext", string([]byte{0xff})} {
		if err := app.routeCommittedText(target, text); !errors.Is(err, errCommittedTextInvalid) {
			t.Fatalf("text=%q err=%v", text, err)
		}
	}
	if got := factory.sessions[0].text(); got != "" {
		t.Fatalf("invalid input reached pane: %q", got)
	}
}

func TestCharSuppressionRunsBeforeModalRouting(t *testing.T) {
	app, _ := newRecordingActionApp(t)
	if !app.modal.Open(modal.ModeCommandPalette, modal.PaneIdentity(app.focusedPane), modal.FocusIdentity(app.focusedPane), []modal.Entry{{ID: "one", Label: "one"}}) {
		t.Fatal("modal open failed")
	}
	app.charSuppression.armBinding(true)
	app.routeGLFWChar('x')
	if query := string(app.modal.Snapshot().Query); query != "" {
		t.Fatalf("binding echo reached modal: %q", query)
	}
	app.routeGLFWChar('y')
	if query := string(app.modal.Snapshot().Query); query != "y" {
		t.Fatalf("unsuppressed char query=%q", query)
	}

	now := time.Now()
	if !app.charSuppression.armIMEEcho(1, "日本", now) {
		t.Fatal("IME echo did not arm")
	}
	app.routeGLFWChar('日')
	app.routeGLFWChar('本')
	if query := string(app.modal.Snapshot().Query); query != "y" {
		t.Fatalf("IME echo reached modal: %q", query)
	}
	app.routeGLFWChar('語')
	if query := string(app.modal.Snapshot().Query); query != "y語" {
		t.Fatalf("post-echo char query=%q", query)
	}
}

func TestCommittedTextQuickSelectConsumesOnceAndRetainsSideEffectError(t *testing.T) {
	app := quickSelectTestActivation(t, quickselect.ActionOpen, "javascript:alert(1)", "a")
	target, err := app.captureCommittedTextTarget()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.routeCommittedText(target, "a"); err != nil {
		t.Fatalf("accepted text consumption failed: %v", err)
	}
	state := app.modal.Snapshot()
	if !app.modal.Active() || !strings.Contains(state.Error, "http") || string(state.Query) != "a" {
		t.Fatalf("state=%#v", state)
	}
	if err := app.routeCommittedText(target, "a"); err != nil || string(app.modal.Snapshot().Query) != "aa" {
		t.Fatalf("second route was not a single mutation: state=%#v err=%v", app.modal.Snapshot(), err)
	}
}

func TestCommittedTextMissingPaneFailsWithoutFallback(t *testing.T) {
	app, factory := newRecordingActionApp(t)
	target, err := app.captureCommittedTextTarget()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.mux.ClosePane(app.focusedPane); err != nil {
		t.Fatal(err)
	}
	if err := app.routeCommittedText(target, "lost"); err == nil {
		t.Fatal("missing pane accepted committed text")
	}
	if got := factory.sessions[0].text(); got != "" {
		t.Fatalf("missing pane wrote %q", got)
	}
}

func TestCommittedTextClassifiesModalAndSearchCapacityAtomically(t *testing.T) {
	app, _ := newRecordingActionApp(t)
	if !app.modal.Open(modal.ModeCommandPalette, modal.PaneIdentity(app.focusedPane), modal.FocusIdentity(app.focusedPane), []modal.Entry{{ID: "one", Label: "one"}}) {
		t.Fatal("modal open failed")
	}
	modalTarget, _ := app.captureCommittedTextTarget()
	fullModal := strings.Repeat("x", modal.MaxQueryRunes)
	if !app.modal.AppendText(modal.ActivationID(modalTarget.Activation), fullModal) {
		t.Fatal("modal fill failed")
	}
	if err := app.routeCommittedText(modalTarget, "y"); !errors.Is(err, errCommittedTextInvalid) || string(app.modal.Snapshot().Query) != fullModal {
		t.Fatalf("modal capacity err=%v query-len=%d", err, len(app.modal.Snapshot().Query))
	}
	app.modal.Close()

	app.search.redraw = func() {}
	if !app.search.open() {
		t.Fatal("search open failed")
	}
	searchTarget, _ := app.captureCommittedTextTarget()
	fullSearch := strings.Repeat("x", maxSearchQueryRunes)
	if !app.search.appendText(app.search.activation, fullSearch) {
		t.Fatal("search fill failed")
	}
	if err := app.routeCommittedText(searchTarget, "y"); !errors.Is(err, errCommittedTextInvalid) || string(app.search.query) != fullSearch {
		t.Fatalf("search capacity err=%v query-len=%d", err, len(app.search.query))
	}
}
