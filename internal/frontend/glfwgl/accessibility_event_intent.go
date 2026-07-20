//go:build glfw

package glfwgl

import (
	"cervterm/internal/accessibility"
	termmux "cervterm/internal/mux"
)

func accessibilityIntentForMuxEvent(event termmux.Event) (accessibility.SemanticIntent, accessibility.AnnouncementKind) {
	switch event.Kind {
	case termmux.PaneOutput:
		return accessibility.IntentText | accessibility.IntentCaret | accessibility.IntentSelection, accessibility.AnnouncementNone
	case termmux.PaneDirty, termmux.PaneTitleChanged, termmux.PaneCWDChanged, termmux.PaneWriteFailed, termmux.PaneResizeFailed, termmux.PaneCloseFailed:
		return accessibility.IntentNone, accessibility.AnnouncementNone
	case termmux.PaneGeometryChanged:
		return accessibility.IntentDocument, accessibility.AnnouncementNone
	case termmux.PaneFocused, termmux.WindowActivated:
		return accessibility.IntentFocus, accessibility.AnnouncementNone
	case termmux.TabActivated, termmux.WorkspaceActivated:
		return accessibility.IntentTopology | accessibility.IntentFocus, accessibility.AnnouncementNone
	case termmux.PaneBell:
		return accessibility.IntentNone, accessibility.AnnouncementBell
	case termmux.PaneNotificationRequested, termmux.PaneNotificationOverflow:
		return accessibility.IntentNone, accessibility.AnnouncementNotification
	case termmux.PaneStarted, termmux.PaneExited, termmux.PaneClosed, termmux.PaneTransferred, termmux.TabEmpty,
		termmux.TabSpawned, termmux.TabRenamed, termmux.TabMoved, termmux.TabRevisionChanged, termmux.TabClosed,
		termmux.WindowTabsEmpty, termmux.WindowCreated, termmux.WindowRenamed, termmux.WindowClosed,
		termmux.WorkspaceCreated, termmux.WorkspaceRenamed, termmux.WindowWorkspaceChanged:
		return accessibility.IntentTopology | accessibility.IntentFocus, accessibility.AnnouncementNone
	default:
		return accessibility.IntentNone, accessibility.AnnouncementNone
	}
}

func accessibilityIntentForInputRevision(changed bool) accessibility.SemanticIntent {
	if !changed {
		return accessibility.IntentNone
	}
	return accessibility.IntentTopology | accessibility.IntentText | accessibility.IntentCaret | accessibility.IntentSelection | accessibility.IntentFocus
}
