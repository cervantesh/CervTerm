//go:build glfw

package glfwgl

import (
	"os"
	"path/filepath"
	"testing"

	termaction "cervterm/internal/action"
	"cervterm/internal/config"
	"cervterm/internal/core"
	termsel "cervterm/internal/selection"
)

func TestPhase10PowerShellFixtureRoutesMetadataThroughTrustedActions(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "phase10", "powershell-osc633.vt"))
	if err != nil {
		t.Fatal(err)
	}
	app := newMuxTestApp(t, 40, 8)
	notifications := &fakeNotificationSink{}
	launcher := &recordingURLLauncher{}
	app.notificationState.sink = notifications
	app.linkLauncher = launcher
	copied := ""
	app.clipboardSetter = func(text string) { copied = text }

	for start := 0; start < len(fixture); {
		end := min(start+7, len(fixture))
		feedTestPane(t, app, fixture[start:end])
		start = end
	}
	if len(notifications.requests) != 0 {
		t.Fatalf("default-off notification reached sink: %#v", notifications.requests)
	}
	if err := executeFocusedAction(app, termaction.CopySemanticZone{Zone: termaction.SemanticZoneInput}); err != nil || copied != "echo ok" {
		t.Fatalf("input=%q err=%v", copied, err)
	}
	if err := executeFocusedAction(app, termaction.CopySemanticZone{Zone: termaction.SemanticZoneOutput}); err != nil || copied != "ok" {
		t.Fatalf("output=%q err=%v", copied, err)
	}

	app.syncFocusedProjection()
	app.refreshLinks()
	if len(launcher.opened) != 0 {
		t.Fatal("fixture output opened a link without activation")
	}
	var point termsel.Point
	found := false
	for _, link := range app.link.links {
		if link.explicit && link.url == "https://example.test/docs" {
			point = termsel.Point{Row: link.row, Col: link.startCol}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("explicit fixture link not projected: %#v", app.link.links)
	}
	app.captureLinkPress(point)
	if !app.handleLinkClick(point) || len(launcher.opened) != 1 || launcher.opened[0] != "https://example.test/docs" {
		t.Fatalf("explicit activation opened=%#v", launcher.opened)
	}

	app.cfg.Notification = config.NotificationConfig{Enabled: true, Focus: "always", RateLimitMS: 100}
	feedTestPane(t, app, []byte("\x1b]777;notify;Build;complete\x1b\\"))
	if len(notifications.requests) != 1 || notifications.requests[0] != (core.NotificationRequest{Title: "Build", Body: "complete"}) {
		t.Fatalf("consented notifications=%#v", notifications.requests)
	}
}

func TestPhase10MaliciousPTYOutputCannotReachExternalEffects(t *testing.T) {
	app := newMuxTestApp(t, 40, 4)
	notifications := &fakeNotificationSink{}
	launcher := &recordingURLLauncher{}
	app.notificationState.sink = notifications
	app.linkLauncher = launcher
	app.cfg.Notification = config.NotificationConfig{Enabled: true, Focus: "always", RateLimitMS: 100}

	feedTestPane(t, app, []byte("\x1b]9;bad\nbody\x07"))
	feedTestPane(t, app, []byte("\x1b]777;open;title;body\x1b\\"))
	if len(notifications.requests) != 0 {
		t.Fatalf("malformed notification reached consented sink: %#v", notifications.requests)
	}
	feedTestPane(t, app, []byte("\x1b]777;notify;Control;accepted\x1b\\"))
	if len(notifications.requests) != 1 || notifications.requests[0].Title != "Control" {
		t.Fatalf("enabled control notification did not reach sink: %#v", notifications.requests)
	}
	notifications.requests = nil

	feedTestPane(t, app, []byte("\x1b]8;;javascript:alert(1)\x1b\\x\x1b]8;;\x1b\\"))
	app.syncFocusedProjection()
	app.refreshLinks()
	if len(app.link.links) != 1 || !app.link.links[0].explicit {
		t.Fatalf("dangerous metadata projection=%#v", app.link.links)
	}
	point := termsel.Point{Row: app.link.links[0].row, Col: app.link.links[0].startCol}
	app.captureLinkPress(point)
	if !app.handleLinkClick(point) || len(launcher.opened) != 0 {
		t.Fatalf("dangerous link opened=%#v", launcher.opened)
	}

	feedTestPane(t, app, []byte("\x1bc\x1b]8;;https://truncated.test"))
	app.syncFocusedProjection()
	app.refreshLinks()
	if len(app.link.links) != 0 {
		t.Fatalf("truncated OSC projected metadata: %#v", app.link.links)
	}
	app.captureLinkPress(termsel.Point{})
	if app.handleLinkClick(termsel.Point{}) || len(launcher.opened) != 0 || len(notifications.requests) != 0 {
		t.Fatalf("truncated OSC produced effects: links=%#v notifications=%#v", launcher.opened, notifications.requests)
	}
}
