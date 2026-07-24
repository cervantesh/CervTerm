//go:build glfw

package glfwgl

import (
	"os"
	"testing"
	"time"

	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
	"cervterm/internal/script"
)

func loadFrontendScriptRuntime(t *testing.T, source string) (config.Config, *script.Runtime) {
	t.Helper()
	path := t.TempDir() + "/cervterm.lua"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, runtime, err := script.Load(path, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtime.Close)
	return cfg, runtime
}

func TestDeferredScriptLifecycleProjectsResizeBeforeScrollThenStatusAndOverlay(t *testing.T) {
	cfg, runtime := loadFrontendScriptRuntime(t, `local cervterm = require("cervterm")
local order = ""
local overlay = cervterm.overlay("lifecycle")
local function record(value)
  order = order .. value
  cervterm.status("order", order)
  overlay:clear()
  overlay:text(1, 1, order, "#ffffff")
  overlay:commit()
end
return { events = {
  resize = function(term, cols, rows) record(string.format("resize:%dx%d>", cols, rows)) end,
  scroll = function(term, offset) record(string.format("scroll:%d", offset)) end,
} }`)
	app := newMuxTestApp(t, 80, 24)
	app.cfg, app.scriptRT = cfg, runtime
	pane := app.focusedPane
	app.pendingPaneResize = map[termmux.PaneID]termmux.PaneGeometry{
		pane: {Pane: pane, Cols: 80, Rows: 24},
	}
	app.pendingPaneScroll[pane] = 7

	app.fireLifecycleEvents()
	if len(app.pendingPaneResize) != 0 || len(app.pendingPaneScroll) != 0 {
		t.Fatalf("deferred maps survived dispatch: resize=%v scroll=%v", app.pendingPaneResize, app.pendingPaneScroll)
	}
	if got := runtime.StatusSegments(); len(got) != 1 || got[0] != "resize:80x24>scroll:7" {
		t.Fatalf("lifecycle order=%q", got)
	}
	if scenes := runtime.Overlays(); len(scenes) != 1 || len(scenes[0].Prims) != 1 || scenes[0].Prims[0].Text != "resize:80x24>scroll:7" {
		t.Fatalf("runtime overlay=%#v", scenes)
	}
	if app.status.line != "" || len(app.overlays.scenes) != 0 {
		t.Fatalf("runtime state projected before sync: status=%q overlays=%#v", app.status.line, app.overlays.scenes)
	}
	app.syncStatusSegments()
	if app.status.line != "resize:80x24>scroll:7" || len(app.overlays.scenes) != 0 {
		t.Fatalf("status projection order: status=%q overlays=%#v", app.status.line, app.overlays.scenes)
	}
	app.syncOverlays()
	if len(app.overlays.scenes) != 1 || app.overlays.scenes[0].Prims[0].Text != app.status.line {
		t.Fatalf("overlay projection=%#v status=%q", app.overlays.scenes, app.status.line)
	}
}

func TestDeferredScriptLifecycleWithoutRuntimeClearsPendingMaps(t *testing.T) {
	app := &App{
		pendingPaneResize: map[termmux.PaneID]termmux.PaneGeometry{1: {Pane: 1, Cols: 80, Rows: 24}},
		pendingPaneScroll: map[termmux.PaneID]int{1: 4},
	}
	app.fireLifecycleEvents()
	if len(app.pendingPaneResize) != 0 || len(app.pendingPaneScroll) != 0 {
		t.Fatalf("no-runtime lifecycle retained pending maps: resize=%v scroll=%v", app.pendingPaneResize, app.pendingPaneScroll)
	}
}

func TestScriptTimerDeadlineFiresThroughActiveProjectionOnly(t *testing.T) {
	cfg, runtime := loadFrontendScriptRuntime(t, `local cervterm = require("cervterm")
return { keys = { { key = "t", action = function(term)
  cervterm.after(50, function(term) term:notify("active-timer") end)
end } } }`)
	first := newMuxTestApp(t, 20, 10)
	second := newMuxTestApp(t, 20, 10)
	cfg.Cursor.Blink = false
	first.cfg, first.scriptRT = cfg.Clone(), runtime
	second.cfg, second.scriptRT = cfg.Clone(), runtime
	first.needsRedraw, second.needsRedraw = false, false

	var log []string
	controller := newWindowController(processServices{scriptRuntime: runtime}, fakeNativePump{log: &log})
	if err := controller.attachApp(1, &fakeNativeWindow{id: "one", log: &log}, first, first.applyMuxEvents); err != nil {
		t.Fatal(err)
	}
	if err := controller.attachApp(2, &fakeNativeWindow{id: "two", log: &log}, second, second.applyMuxEvents); err != nil {
		t.Fatal(err)
	}
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	if err := controller.focus(2); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Dispatch(0, second.hostForFocused()); err != nil {
		t.Fatal(err)
	}
	deadline, ok := runtime.NextTimerDeadline()
	if !ok {
		t.Fatal("timer deadline was not published")
	}
	if got := second.nextWakeTimeout(deadline.Add(-25 * time.Millisecond)); got != 25*time.Millisecond {
		t.Fatalf("timer wake=%v want=25ms", got)
	}
	active := controller.activeProjectionApp()
	active.fireDueTimers(deadline.Add(-time.Nanosecond))
	if first.notice != "" || second.notice != "" {
		t.Fatalf("timer fired before deadline: first=%q second=%q", first.notice, second.notice)
	}
	active.fireDueTimers(deadline)
	if first.notice != "" || second.notice != "active-timer" {
		t.Fatalf("timer host routing: first=%q second=%q active=%d", first.notice, second.notice, controller.active)
	}
	if _, ok := runtime.NextTimerDeadline(); ok {
		t.Fatal("one-shot timer survived its deadline")
	}
}

func TestScriptOutputFiltersEmptyAndUnwantedChunksAndPreservesExactErrorNotice(t *testing.T) {
	cfg, runtime := loadFrontendScriptRuntime(t, `return { events = { output = function(term, data)
  if data == "boom" then error("boom") end
  term:notify("output:" .. data)
end } }`)
	app := newMuxTestApp(t, 20, 10)
	app.cfg, app.scriptRT = cfg, runtime
	pane := app.focusedPane
	app.notice = "sentinel"
	app.applyMuxEvents([]termmux.Event{{Kind: termmux.PaneOutput, Pane: pane}})
	if app.notice != "sentinel" {
		t.Fatalf("empty output reached script handler: notice=%q", app.notice)
	}
	app.applyMuxEvents([]termmux.Event{{Kind: termmux.PaneOutput, Pane: pane, Data: []byte("x")}})
	if app.notice != "output:x" {
		t.Fatalf("non-empty output notice=%q", app.notice)
	}
	expectedErr := runtime.FireOutput(paneHost{app: app, pane: pane}, "boom")
	if expectedErr == nil {
		t.Fatal("erroring output handler returned nil")
	}
	app.notice = ""
	app.applyMuxEvents([]termmux.Event{{Kind: termmux.PaneOutput, Pane: pane, Data: []byte("boom")}})
	if want := "script error: " + expectedErr.Error(); app.notice != want {
		t.Fatalf("script error notice=%q want=%q", app.notice, want)
	}

	cfg, runtime = loadFrontendScriptRuntime(t, `return {}`)
	app.cfg, app.scriptRT, app.notice = cfg, runtime, "unwanted"
	if runtime.WantsOutput() {
		t.Fatal("runtime without output handler wants output")
	}
	app.applyMuxEvents([]termmux.Event{{Kind: termmux.PaneOutput, Pane: pane, Data: []byte("ignored")}})
	if app.notice != "unwanted" {
		t.Fatalf("unwanted output reached runtime: notice=%q", app.notice)
	}
}

// TestKnownDefect_L1_02_SiblingRuntimeConfigDiverges pins projection-local config ownership.
// expires Slice 3.4
func TestKnownDefect_L1_02_SiblingRuntimeConfigDiverges(t *testing.T) {
	base := config.Defaults()
	owner := &App{cfg: base.Clone(), desiredCfg: base.Clone(), composedCfg: base.Clone(), scriptGeneration: 3}
	child := newProjectionApp(owner)
	owner.cfg.Window.Opacity = 0.42

	var log []string
	controller := newWindowController(processServices{}, fakeNativePump{log: &log})
	if err := controller.attachApp(2, &fakeNativeWindow{id: "child", log: &log}, child, func([]termmux.Event) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	if err := controller.syncSharedProjectionState(owner); err != nil {
		t.Fatal(err)
	}
	ownerConfig := (paneHost{app: owner}).RuntimeConfig()
	childConfig := (paneHost{app: child}).RuntimeConfig()
	if ownerConfig.Window.Opacity != 0.42 || childConfig.Window.Opacity == ownerConfig.Window.Opacity {
		t.Fatalf("runtime configs converged unexpectedly: owner=%v child=%v", ownerConfig.Window.Opacity, childConfig.Window.Opacity)
	}
}

// TestKnownDefect_L2_04_TimerAndStatusRegistrationsAreUnbounded pins the absent aggregate bound.
// expires Slice 4.6
func TestKnownDefect_L2_04_TimerAndStatusRegistrationsAreUnbounded(t *testing.T) {
	cfg, runtime := loadFrontendScriptRuntime(t, `local cervterm = require("cervterm")
local fired = 0
for i = 1, 1024 do
  cervterm.status("status-" .. i, "value-" .. i)
  cervterm.after(1, function()
    fired = fired + 1
    cervterm.status("fired", tostring(fired))
  end)
end
return {}`)
	if got := runtime.StatusSegments(); len(got) != 1024 {
		t.Fatalf("status registrations=%d want=1024", len(got))
	}
	app := newMuxTestApp(t, 20, 10)
	app.cfg, app.scriptRT = cfg, runtime
	app.fireDueTimers(time.Now().Add(time.Hour))
	segments := runtime.StatusSegments()
	if len(segments) != 1025 || segments[len(segments)-1] != "1024" {
		t.Fatalf("unbounded timer execution: segments=%d last=%q", len(segments), segments[len(segments)-1])
	}
	if _, ok := runtime.NextTimerDeadline(); ok {
		t.Fatal("one-shot timer registrations survived execution")
	}
}
