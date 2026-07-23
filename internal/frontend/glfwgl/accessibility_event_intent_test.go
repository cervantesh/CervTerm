//go:build glfw

package glfwgl

import (
	"testing"

	"cervterm/internal/accessibility"
	termmux "cervterm/internal/mux"
)

func TestAccessibilityMuxIntentTruthTableAndPayloadExclusion(t *testing.T) {
	tests := []struct {
		kind         termmux.EventKind
		intent       accessibility.SemanticIntent
		announcement accessibility.AnnouncementKind
	}{
		{termmux.PaneOutput, accessibility.IntentText | accessibility.IntentCaret | accessibility.IntentSelection, accessibility.AnnouncementNone},
		{termmux.PaneDirty, accessibility.IntentNone, accessibility.AnnouncementNone},
		{termmux.PaneGeometryChanged, accessibility.IntentDocument, accessibility.AnnouncementNone},
		{termmux.PaneFocused, accessibility.IntentFocus, accessibility.AnnouncementNone},
		{termmux.PaneTransferred, accessibility.IntentTopology | accessibility.IntentFocus, accessibility.AnnouncementNone},
		{termmux.TabRenamed, accessibility.IntentTopology | accessibility.IntentFocus, accessibility.AnnouncementNone},
		{termmux.TabActivated, accessibility.IntentTopology | accessibility.IntentFocus, accessibility.AnnouncementNone},
		{termmux.WorkspaceActivated, accessibility.IntentTopology | accessibility.IntentFocus, accessibility.AnnouncementNone},
		{termmux.PaneBell, accessibility.IntentNone, accessibility.AnnouncementBell},
		{termmux.PaneNotificationRequested, accessibility.IntentNone, accessibility.AnnouncementNotification},
	}
	for _, test := range tests {
		event := termmux.Event{Kind: test.kind, Data: []byte("terminal-secret"), Text: "body-secret"}
		intent, announcement := accessibilityIntentForMuxEvent(event)
		if intent != test.intent || announcement != test.announcement {
			t.Fatalf("kind=%v intent=%v announcement=%v", test.kind, intent, announcement)
		}
	}
}

func TestAccessibilityInputRevisionIntent(t *testing.T) {
	if accessibilityIntentForInputRevision(false) != accessibility.IntentNone {
		t.Fatal("unchanged input emitted intent")
	}
	intent := accessibilityIntentForInputRevision(true)
	want := accessibility.IntentTopology | accessibility.IntentText | accessibility.IntentCaret | accessibility.IntentSelection | accessibility.IntentFocus
	if intent != want {
		t.Fatalf("intent=%v want=%v", intent, want)
	}
}
