//go:build glfw

package glfwgl

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	termaction "cervterm/internal/action"
	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
	"cervterm/internal/script"
	termsel "cervterm/internal/selection"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func configureActionBindings(t *testing.T, a *App) {
	t.Helper()
	a.cfg = config.Defaults()
	if spec, ok := parseStatsHotkey(a.cfg.Render.StatsHotkey); ok {
		a.statsSpec, a.statsSpecOK = spec, true
	}
	a.initZoomHotkeys()
	a.initActionBindings()
}

func TestTypedBuiltinsPreservePressRepeatPolicies(t *testing.T) {
	a := newMuxTestApp(t, 20, 10)
	configureActionBindings(t, a)
	if !a.dispatchBuiltinAction(glfw.KeyI, glfw.ModControl|glfw.ModShift, false) || !a.showStats {
		t.Fatal("stats press was not consumed and executed")
	}
	if a.dispatchBuiltinAction(glfw.KeyI, glfw.ModControl|glfw.ModShift, true) || !a.showStats {
		t.Fatal("stats repeat should fall through without execution")
	}

	state := a.ensurePaneUI(a.focusedPane)
	state.font.fontSize = a.cfg.Font.Size
	if !a.dispatchBuiltinAction(glfw.KeyEqual, glfw.ModControl, true) {
		t.Fatal("zoom repeat was not consumed")
	}
	if !state.font.pending || state.font.pendingTarget != a.cfg.Font.Size+zoomFontStep {
		t.Fatalf("zoom repeat state = %#v", state.font)
	}
}

func TestTypedBuiltinsPreserveModifierMatching(t *testing.T) {
	a := newRunningMuxTestApp(t)
	configureActionBindings(t, a)
	if a.dispatchBuiltinAction(glfw.KeyPageUp, glfw.ModShift|glfw.ModControl, false) {
		t.Fatal("Ctrl+Shift+PageUp must not match scrollback action")
	}
	if !a.dispatchBuiltinAction(glfw.KeyEqual, glfw.ModAlt|glfw.ModShift|glfw.ModControl, false) {
		t.Fatal("legacy mux split with extra Ctrl was not consumed")
	}
	if len(a.mux.PaneIDs()) != 2 {
		t.Fatalf("split panes = %v", a.mux.PaneIDs())
	}
	if !a.dispatchBuiltinAction(glfw.KeyInsert, glfw.ModControl|glfw.ModShift, false) {
		t.Fatal("Shift+Insert paste precedence was not preserved")
	}
}

func TestReservedActionsPreserveSearchAndReloadPrecedence(t *testing.T) {
	a := newMuxTestApp(t, 20, 10)
	a.search.redraw = func() {}
	if !searchActivationChord(glfw.KeyF, glfw.ModControl|glfw.ModShift|glfw.ModAlt) {
		t.Fatal("search chord should allow legacy extra modifiers")
	}
	if !a.dispatchReservedAction(termaction.ToggleSearch{}, glfw.KeyF, glfw.ModControl|glfw.ModShift, false) {
		t.Fatal("search activation was not consumed")
	}
	if !a.search.active {
		t.Fatal("search did not open")
	}
	if !reloadChord(glfw.KeyR, glfw.ModControl|glfw.ModShift) || reloadChord(glfw.KeyR, glfw.ModControl|glfw.ModShift|glfw.ModAlt) {
		t.Fatal("reload exact modifier policy changed")
	}
	if !a.dispatchReservedAction(termaction.ReloadConfig{}, glfw.KeyR, glfw.ModControl|glfw.ModShift, false) {
		t.Fatal("reload press was not consumed")
	}
	if !strings.Contains(a.notice, "no config source") {
		t.Fatalf("reload notice = %q", a.notice)
	}
	if a.dispatchReservedAction(termaction.ReloadConfig{}, glfw.KeyR, glfw.ModControl|glfw.ModShift, true) {
		t.Fatal("reload repeat should fall through")
	}
}

func TestNonConsumingRepeatFallsThroughToLaterBuiltin(t *testing.T) {
	a := newRunningMuxTestApp(t)
	configureActionBindings(t, a)
	stats, ok := parseStatsHotkey("alt+shift+equal")
	if !ok {
		t.Fatal("failed to parse collision test chord")
	}
	a.statsSpec, a.statsSpecOK = stats, true
	a.initActionBindings()
	if !a.dispatchBuiltinAction(glfw.KeyEqual, glfw.ModAlt|glfw.ModShift, true) {
		t.Fatal("repeat did not fall through from press-only stats to repeatable split")
	}
	if a.showStats {
		t.Fatal("stats executed on repeat")
	}
	if len(a.mux.PaneIDs()) != 2 {
		t.Fatalf("split panes = %v", a.mux.PaneIDs())
	}
}

func newRecordingActionApp(t *testing.T) (*App, *recordingPaneFactory) {
	t.Helper()
	factory := &recordingPaneFactory{}
	mux := termmux.New(factory, termmux.Options{})
	_, pane, events, err := mux.Bootstrap(termmux.SpawnSpec{}, termmux.PixelRect{Width: 80, Height: 24}, termmux.CellMetrics{CellWidth: 1, CellHeight: 1})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{
		mux: mux, focusedPane: pane, paneUI: make(map[termmux.PaneID]*paneUIState),
		pendingPaneScroll: make(map[termmux.PaneID]int), cellW: 1, cellH: 1,
	}
	app.handleMuxEvents(events)
	app.syncFocusedProjection()
	configureActionBindings(t, app)
	t.Cleanup(func() { _ = mux.Shutdown() })
	return app, factory
}

func TestKeyPipelineKeepsCtrlCInterruptWithoutSelection(t *testing.T) {
	a, factory := newRecordingActionApp(t)
	a.handleKeyEvent(glfw.KeyC, glfw.Press, glfw.ModControl)
	if got := factory.sessions[0].text(); got != "\x03" {
		t.Fatalf("Ctrl+C write = %q, want interrupt byte", got)
	}
}

func TestKeyPipelineConsumesSelectionCopyRepeats(t *testing.T) {
	a, factory := newRecordingActionApp(t)
	if _, err := factory.sessions[0].writer.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		a.drainIncoming()
		view, _ := a.mux.PaneView(a.focusedPane)
		if len(view.Snapshot.Cells) > 0 && view.Snapshot.Cells[0].Rune == 'a' {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for pane output")
		}
		runtime.Gosched()
	}
	a.selection = selectionState{active: true, start: termsel.Point{Row: 0, Col: 0}, end: termsel.Point{Row: 0, Col: 2}}
	if a.Selection() == "" {
		t.Fatal("test selection is empty")
	}
	a.handleKeyEvent(glfw.KeyC, glfw.Repeat, glfw.ModControl)
	if got := factory.sessions[0].text(); got != "" {
		t.Fatalf("selection-copy repeat leaked to PTY: %q", got)
	}
}

func TestPressOnlyStatsRepeatPreservesLegacyFallthrough(t *testing.T) {
	a, factory := newRecordingActionApp(t)
	a.handleKeyEvent(glfw.KeyI, glfw.Repeat, glfw.ModControl|glfw.ModShift)
	if a.showStats {
		t.Fatal("stats executed on repeat")
	}
	if got := factory.sessions[0].text(); got != "\t" {
		t.Fatalf("stats repeat fallthrough write = %q, want Ctrl+I tab", got)
	}
}

func TestKeyPipelinePreservesReservedAndScriptPrecedence(t *testing.T) {
	path := t.TempDir() + "/cervterm.lua"
	source := `return { keys = {
		{ key = "equal", mods = "ctrl", action = function(term) term:notify("script-zoom") end },
		{ key = "r", mods = "ctrl+shift", action = function(term) term:notify("script-reload") end },
		{ key = "f", mods = "ctrl+shift", action = function(term) term:notify("script-search") end },
	} }`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, runtime, err := script.Load(path, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtime.Close)
	a := newMuxTestApp(t, 20, 10)
	a.cfg, a.scriptRT = cfg, runtime
	a.search.redraw = func() {}
	a.initZoomHotkeys()
	a.initActionBindings()
	a.ensurePaneUI(a.focusedPane).font.fontSize = cfg.Font.Size

	a.handleKeyEvent(glfw.KeyEqual, glfw.Press, glfw.ModControl)
	if a.notice != "script-zoom" {
		t.Fatalf("script did not override zoom: notice=%q", a.notice)
	}
	if a.ensurePaneUI(a.focusedPane).font.pending {
		t.Fatal("built-in zoom executed after script binding")
	}

	a.notice = ""
	a.handleKeyEvent(glfw.KeyR, glfw.Press, glfw.ModControl|glfw.ModShift)
	if !strings.Contains(a.notice, "no config source") {
		t.Fatalf("reload did not remain reserved: notice=%q", a.notice)
	}

	a.notice = ""
	a.handleKeyEvent(glfw.KeyF, glfw.Press, glfw.ModControl|glfw.ModShift)
	if !a.search.active || a.notice == "script-search" {
		t.Fatalf("search activation did not remain reserved: active=%v notice=%q", a.search.active, a.notice)
	}
}
