//go:build glfw

package glfwgl

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"cervterm/internal/bellpolicy"
	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
	"cervterm/internal/script"
)

type fakeBellEffectSink struct {
	audible int
	taskbar int
}

func (s *fakeBellEffectSink) Audible() error { s.audible++; return nil }
func (s *fakeBellEffectSink) Taskbar()       { s.taskbar++ }

func TestBellEffectsAreFocusAwareAndThrottled(t *testing.T) {
	now := time.Unix(100, 0)
	sink := &fakeBellEffectSink{}
	app := &App{cfg: config.Defaults(), bellState: bellState{bellGate: bellpolicy.NewGate(func() time.Time { return now }), bellSink: sink}}
	app.cfg.Bell = config.BellConfig{Mode: "taskbar", Focus: "unfocused", ThrottleMS: 250, VisualDurationMS: 120}
	app.applyBellEffectWithFocus(true)
	if sink.taskbar != 0 {
		t.Fatal("focused event reached unfocused-only taskbar sink")
	}
	app.applyBellEffectWithFocus(false)
	app.applyBellEffectWithFocus(false)
	if sink.taskbar != 1 {
		t.Fatalf("taskbar calls = %d, want throttled 1", sink.taskbar)
	}
	now = now.Add(250 * time.Millisecond)
	app.applyBellEffectWithFocus(false)
	if sink.taskbar != 2 {
		t.Fatalf("taskbar calls = %d after boundary", sink.taskbar)
	}
}

func TestBellEffectsRouteAudibleAndVisual(t *testing.T) {
	now := time.Unix(200, 0)
	sink := &fakeBellEffectSink{}
	app := &App{cfg: config.Defaults(), bellState: bellState{bellGate: bellpolicy.NewGate(func() time.Time { return now }), bellSink: sink}}
	app.cfg.Bell = config.BellConfig{Mode: "audible", Focus: "always", VisualDurationMS: 180}
	app.applyBellEffectWithFocus(true)
	if sink.audible != 1 {
		t.Fatalf("audible calls = %d", sink.audible)
	}
	app.cfg.Bell.Mode = "visual"
	app.applyBellEffectWithFocus(true)
	if want := now.Add(180 * time.Millisecond); !app.bellVisualUntil.Equal(want) || !app.needsRedraw {
		t.Fatalf("visual until=%v redraw=%t, want %v true", app.bellVisualUntil, app.needsRedraw, want)
	}
}

func TestVisualBellSchedulesOneExpiryRepaint(t *testing.T) {
	now := time.Unix(300, 0)
	app := &App{cfg: config.Defaults(), bellState: bellState{bellVisualUntil: now.Add(120 * time.Millisecond)}}
	wake := app.nextWakeTimeout(now)
	if wake != 120*time.Millisecond {
		t.Fatalf("wake = %v, want 120ms", wake)
	}
	if !app.redrawWanted(now.Add(120 * time.Millisecond)) {
		t.Fatal("visual bell expiry must request a clearing repaint")
	}
}

func TestDisabledBellStillDeliversEveryLuaCallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	if err := os.WriteFile(path, []byte(`local n=0; return { events={ bell=function(term) n=n+1; term:notify(tostring(n)) end } }`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, runtime, err := script.Load(path, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	sink := &fakeBellEffectSink{}
	app := &App{cfg: cfg, scriptRT: runtime, bellState: bellState{bellSink: sink}}
	app.applyMuxEvents([]termmux.Event{{Kind: termmux.PaneBell, Pane: 1}, {Kind: termmux.PaneBell, Pane: 1}, {Kind: termmux.PaneBell, Pane: 1}})
	if app.notice != "3" {
		t.Fatalf("last callback notice = %q, want 3", app.notice)
	}
	if sink.audible != 0 || sink.taskbar != 0 || !app.bellVisualUntil.IsZero() {
		t.Fatalf("disabled bell reached effects: sink=%#v visual=%v", sink, app.bellVisualUntil)
	}
}

func TestBellCatchUpRecoversDroppedEventsWithoutDuplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	if err := os.WriteFile(path, []byte(`local n=0; return { events={ bell=function(term) n=n+1; term:notify(tostring(n)) end } }`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, runtime, err := script.Load(path, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	app := newMuxTestApp(t, 80, 24)
	app.cfg, app.scriptRT = cfg, runtime
	events, err := app.mux.FeedFallback(app.focusedPane, []byte("\a\a\a"))
	if err != nil {
		t.Fatal(err)
	}
	app.catchUpBellEvents()
	if app.notice != "3" || app.bellDelivered[app.focusedPane] != 3 {
		t.Fatalf("catch-up notice=%q delivered=%d", app.notice, app.bellDelivered[app.focusedPane])
	}
	app.applyMuxEvents(events)
	if app.notice != "3" || app.bellDelivered[app.focusedPane] != 3 {
		t.Fatalf("stale events duplicated delivery: notice=%q delivered=%d", app.notice, app.bellDelivered[app.focusedPane])
	}
}
