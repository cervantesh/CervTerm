//go:build glfw

package glfwgl

import (
	"errors"
	"strings"
	"testing"

	"cervterm/internal/modal"
	termmux "cervterm/internal/mux"
	"cervterm/internal/quickselect"

	"github.com/go-gl/glfw/v3.3/glfw"
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
	a.quickSelect.userActivation = true
	a.applyModalIntents(a.modal.Accept())
	if !a.modal.Active() || !strings.Contains(a.modal.Snapshot().Error, "http") {
		t.Fatalf("state=%#v", a.modal.Snapshot())
	}
}

func TestQuickSelectOpenUsesValidatedHTTPAdapter(t *testing.T) {
	a := quickSelectTestActivation(t, quickselect.ActionOpen, "https://example.test/path", "a")
	launcher := &recordingURLLauncher{}
	a.linkLauncher = launcher
	a.handleModalKey(glfw.KeyEnter, glfw.Press, 0)
	if len(launcher.opened) != 1 || launcher.opened[0] != "https://example.test/path" || a.modal.Active() {
		t.Fatalf("opened=%#v active=%v", launcher.opened, a.modal.Active())
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

func TestQuickSelectProgrammaticAcceptCannotOpen(t *testing.T) {
	a := quickSelectTestActivation(t, quickselect.ActionOpen, "https://example.test/secret", "a")
	launcher := &recordingURLLauncher{}
	a.linkLauncher = launcher
	a.applyModalIntents(a.modal.Accept())
	if len(launcher.opened) != 0 || !a.modal.Active() {
		t.Fatalf("opened=%#v state=%#v", launcher.opened, a.modal.Snapshot())
	}
}

func TestQuickSelectLauncherErrorIsRedacted(t *testing.T) {
	a := quickSelectTestActivation(t, quickselect.ActionOpen, "https://example.test/path?token=secret", "a")
	launcher := &recordingURLLauncher{err: errors.New("failed https://example.test/path?token=secret")}
	a.linkLauncher = launcher
	a.handleModalKey(glfw.KeyEnter, glfw.Press, 0)
	if !a.modal.Active() || strings.Contains(a.modal.Snapshot().Error, "token=secret") {
		t.Fatalf("state=%#v", a.modal.Snapshot())
	}
}

func TestQuickSelectEnterRepeatCannotGrantActivation(t *testing.T) {
	a := quickSelectTestActivation(t, quickselect.ActionOpen, "https://example.test/path", "a")
	launcher := &recordingURLLauncher{}
	a.linkLauncher = launcher
	a.handleModalKey(glfw.KeyEnter, glfw.Repeat, 0)
	if len(launcher.opened) != 0 || !a.modal.Active() || a.quickSelect.userActivation {
		t.Fatalf("opened=%#v state=%#v token=%v", launcher.opened, a.modal.Snapshot(), a.quickSelect.userActivation)
	}
}
