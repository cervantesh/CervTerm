//go:build glfw

package glfwgl

import (
	"strings"
	"testing"

	"cervterm/internal/modal"
	termmux "cervterm/internal/mux"
	"cervterm/internal/quickselect"
)

func quickSelectTestActivation(t *testing.T, action quickselect.Action, text, label string) *App {
	t.Helper()
	a := newRunningMuxTestApp(t)
	snapshot, ok := a.mux.QuickSelectSnapshot(a.focusedPane, 0, 0)
	if !ok {
		t.Fatal("snapshot")
	}
	candidate := quickselect.Candidate{Text: text, Label: label, Action: action}
	a.quickSelect = quickSelectActivation{snapshot: snapshot, candidates: map[string]quickselect.Candidate{label: candidate}}
	if !a.modal.Open(modal.ModeQuickSelect, modal.PaneIdentity(a.focusedPane), modal.FocusIdentity(a.focusedPane), []modal.Entry{{ID: label, Label: label, Detail: text}}) {
		t.Fatal("open")
	}
	return a
}

func TestQuickSelectCopiesExactTextAndClosesAfterLabel(t *testing.T) {
	a := quickSelectTestActivation(t, quickselect.ActionCopy, "ISSUE-42", "aa")
	copied := ""
	a.quickSelect.setClipboard = func(text string) { copied = text }
	if !a.handleModalChar('a') || !a.modal.Active() {
		t.Fatal("first label rune did not retain modal")
	}
	if !a.handleModalChar('a') || a.modal.Active() {
		t.Fatalf("complete label did not close modal: state=%#v copied=%q", a.modal.Snapshot(), copied)
	}
	if copied != "ISSUE-42" {
		t.Fatalf("copied=%q", copied)
	}
}

func TestQuickSelectOpenRejectsUnsafeSchemeAndPreservesModal(t *testing.T) {
	a := quickSelectTestActivation(t, quickselect.ActionOpen, "javascript:alert(1)", "a")
	called := false
	a.quickSelect.openURL = func(string) error { called = true; return nil }
	a.applyModalIntents(a.modal.Accept())
	if called || !a.modal.Active() || !strings.Contains(a.modal.Snapshot().Error, "http") {
		t.Fatalf("called=%v state=%#v", called, a.modal.Snapshot())
	}
}

func TestQuickSelectOpenUsesValidatedHTTPAdapter(t *testing.T) {
	a := quickSelectTestActivation(t, quickselect.ActionOpen, "https://example.test/path", "a")
	opened := ""
	a.quickSelect.openURL = func(value string) error { opened = value; return nil }
	a.applyModalIntents(a.modal.Accept())
	if opened != "https://example.test/path" || a.modal.Active() {
		t.Fatalf("opened=%q active=%v", opened, a.modal.Active())
	}
}

func TestQuickSelectStaleResizeNeverActs(t *testing.T) {
	a := quickSelectTestActivation(t, quickselect.ActionCopy, "secret", "a")
	called := false
	a.quickSelect.setClipboard = func(string) { called = true }
	if _, err := a.mux.ResizeBounds(termmux.PixelRect{Width: 640, Height: 400}); err != nil {
		t.Fatal(err)
	}
	a.applyModalIntents(a.modal.Accept())
	if called || !a.modal.Active() || !strings.Contains(a.modal.Snapshot().Error, "stale") {
		t.Fatalf("called=%v state=%#v", called, a.modal.Snapshot())
	}
}
