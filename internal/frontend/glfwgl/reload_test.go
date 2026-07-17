//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	termaction "cervterm/internal/action"
	"cervterm/internal/config"
	"cervterm/internal/fontglyph"
	termmux "cervterm/internal/mux"
	"cervterm/internal/script"
)

func writeReloadConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func copyReloadFile(t *testing.T, source, destination string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestConfigWatchDebouncesSelectedSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.tl")
	writeReloadConfig(t, path, "return {}")
	base := time.Now().Add(time.Second)
	if err := os.Chtimes(path, base, base); err != nil {
		t.Fatal(err)
	}
	watch := newConfigWatchState(path)
	writeReloadConfig(t, path, "return { colors = {} }")
	changed := base.Add(2 * time.Second)
	if err := os.Chtimes(path, changed, changed); err != nil {
		t.Fatal(err)
	}
	if watch.poll(changed) {
		t.Fatal("first observed change must start, not finish, debounce")
	}
	if watch.poll(changed.Add(100 * time.Millisecond)) {
		t.Fatal("poll before interval/debounce must not fire")
	}
	if !watch.poll(changed.Add(300 * time.Millisecond)) {
		t.Fatal("stable source change did not fire after debounce")
	}
	if watch.poll(changed.Add(600 * time.Millisecond)) {
		t.Fatal("unchanged source fired twice")
	}
}

func TestConfigWatchDebouncesGraphChangesAndDeletion(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "config.lua")
	include := filepath.Join(dir, "colors.lua")
	writeReloadConfig(t, primary, "return {}")
	writeReloadConfig(t, include, "return {value=1}")
	watch := newConfigWatchState(primary, include, primary)
	if len(watch.paths) != 2 {
		t.Fatalf("normalized watch paths = %#v", watch.paths)
	}
	base := time.Now().Add(time.Second)
	writeReloadConfig(t, include, "return {value=2}")
	if watch.poll(base) {
		t.Fatal("include change fired before debounce")
	}
	if !watch.poll(base.Add(300 * time.Millisecond)) {
		t.Fatal("stable include change did not fire")
	}
	if err := os.Remove(include); err != nil {
		t.Fatal(err)
	}
	deleted := base.Add(600 * time.Millisecond)
	if watch.poll(deleted) {
		t.Fatal("dependency deletion fired before debounce")
	}
	if !watch.poll(deleted.Add(300 * time.Millisecond)) {
		t.Fatal("stable dependency deletion did not fire")
	}
}

func TestConfigWatchCoalescesUntilWholeGraphIsStable(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "config.lua")
	include := filepath.Join(dir, "base.lua")
	writeReloadConfig(t, primary, "return {}")
	writeReloadConfig(t, include, "return {}")
	watch := newConfigWatchState(primary, include)
	base := time.Now().Add(time.Second)
	writeReloadConfig(t, primary, "return {config_version=2}")
	if watch.poll(base) {
		t.Fatal("first graph edit fired")
	}
	writeReloadConfig(t, include, "return {font={size=15}}")
	if watch.poll(base.Add(300 * time.Millisecond)) {
		t.Fatal("second graph edit did not restart stability debounce")
	}
	if !watch.poll(base.Add(600 * time.Millisecond)) {
		t.Fatal("coalesced graph did not fire after becoming stable")
	}
}

func TestPrepareRasterContextsIncludesEveryPaneSize(t *testing.T) {
	a := newRunningMuxTestApp(t)
	a.cfg = config.Defaults()
	a.contentScaleX, a.contentScaleY = 1, 1
	renderer := &atlasTestRenderer{}
	atlas, err := newGlyphAtlasWithBackendFactory(renderer, fontglyph.Spec{Family: a.cfg.Font.Family, Size: 14, DPI: 96, TextRaster: "subpixel"}, 1, 0, func(spec fontglyph.Spec) (fontglyph.Backend, error) {
		return &atlasTestBackend{cellW: int(spec.Size), cellH: int(spec.Size * 2), baseline: int(spec.Size)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	a.atlas = atlas
	t.Cleanup(atlas.close)

	first := a.focusedPane
	second, events, err := a.mux.Split(first, termmux.SplitColumns, termmux.SpawnSpec{})
	if err != nil {
		t.Fatal(err)
	}
	a.handleMuxEvents(events)
	a.ensurePaneUI(first).font.fontSize = 12
	a.ensurePaneUI(second).font.fontSize = 18

	prepared, err := a.prepareRasterContexts("go")
	if err != nil {
		t.Fatal(err)
	}
	defer closePreparedRasterContexts(prepared)
	if len(prepared) != 2 {
		t.Fatalf("prepared contexts = %d, want one for each pane size", len(prepared))
	}
	for _, size := range []float64{12, 18} {
		spec := fontglyph.Spec{Family: a.cfg.Font.Family, Size: size, DPI: 96, TextRaster: "go"}
		key := newAtlasFontKey(spec, a.cfg.Render.TextGamma, a.cfg.Render.TextDarken)
		if _, ok := prepared[key]; !ok {
			t.Fatalf("missing prepared context for size %.0f", size)
		}
	}
}

func TestLiveConfigPreparationDoesNotMutateAndAbortClosesResources(t *testing.T) {
	a := newRunningMuxTestApp(t)
	a.cfg = config.Defaults()
	a.cfg.Colors.Background = "#080B12"
	a.cfg.Render.TextRaster = "subpixel"
	a.contentScaleX, a.contentScaleY = 1, 1
	var created []*atlasTestBackend
	atlas, err := newGlyphAtlasWithBackendFactory(&atlasTestRenderer{}, fontglyph.Spec{Family: a.cfg.Font.Family, Size: a.cfg.Font.Size, DPI: 96, TextRaster: "subpixel"}, 1, 0, func(spec fontglyph.Spec) (fontglyph.Backend, error) {
		backend := &atlasTestBackend{cellW: int(spec.Size), cellH: int(spec.Size * 2), baseline: int(spec.Size)}
		created = append(created, backend)
		return backend, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	a.atlas = atlas
	t.Cleanup(atlas.close)
	created = nil // initial active context is atlas-owned
	a.paneUI = make(map[termmux.PaneID]*paneUIState)

	next := a.cfg
	next.Colors.Background = "#01020380"
	prepared, err := a.prepareLiveConfig(next)
	if err != nil {
		t.Fatal(err)
	}
	if a.cfg.Colors.Background == next.Colors.Background {
		t.Fatal("preparation mutated active config")
	}
	if len(a.paneUI) != 0 {
		t.Fatal("preparation created active pane UI state")
	}
	if len(created) != 1 || created[0].closeCalls != 0 {
		t.Fatalf("prepared resources = %#v", created)
	}
	prepared.Close()
	prepared.Close()
	if created[0].closeCalls != 1 {
		t.Fatalf("aborted backend close calls = %d", created[0].closeCalls)
	}
	if a.cfg.Colors.Background == next.Colors.Background {
		t.Fatal("abort mutated active config")
	}
}

func TestLiveConfigCommitTransfersPreparedResources(t *testing.T) {
	a := newRunningMuxTestApp(t)
	a.cfg = config.Defaults()
	a.cfg.Colors.Background = "#080B12"
	a.cfg.Render.TextRaster = "subpixel"
	for _, id := range a.mux.PaneIDs() {
		a.ensurePaneUI(id).font.fontSize = a.cfg.Font.Size
	}
	a.contentScaleX, a.contentScaleY = 1, 1
	var created []*atlasTestBackend
	atlas, err := newGlyphAtlasWithBackendFactory(&atlasTestRenderer{}, fontglyph.Spec{Family: a.cfg.Font.Family, Size: a.cfg.Font.Size, DPI: 96, TextRaster: "subpixel"}, 1, 0, func(spec fontglyph.Spec) (fontglyph.Backend, error) {
		backend := &atlasTestBackend{cellW: int(spec.Size), cellH: int(spec.Size * 2), baseline: int(spec.Size)}
		created = append(created, backend)
		return backend, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	a.atlas = atlas
	t.Cleanup(atlas.close)
	created = nil

	next := a.cfg
	next.Colors.Background = "#01020380"
	prepared, err := a.prepareLiveConfig(next)
	if err != nil {
		t.Fatal(err)
	}
	a.commitLiveConfig(prepared)
	prepared.Close()
	if a.cfg.Colors.Background != next.Colors.Background {
		t.Fatalf("committed background = %q", a.cfg.Colors.Background)
	}
	if len(created) != 1 || created[0].closeCalls != 0 {
		t.Fatalf("committed resource closed or missing: %#v", created)
	}
}

func TestConfigWatchDetectsSameMetadataContentChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	fixed := time.Unix(100, 0)
	writeReloadConfig(t, path, "return { value = 1 }\n")
	if err := os.Chtimes(path, fixed, fixed); err != nil {
		t.Fatal(err)
	}
	watch := newConfigWatchState(path)

	writeReloadConfig(t, path, "return { value = 2 }\n")
	if err := os.Chtimes(path, fixed, fixed); err != nil {
		t.Fatal(err)
	}
	observed := fixed.Add(time.Second)
	if watch.poll(observed) {
		t.Fatal("first content-hash change observation must start debounce")
	}
	if !watch.poll(observed.Add(300 * time.Millisecond)) {
		t.Fatal("same-size, same-mtime content change was not detected")
	}
}

func TestReloadFailurePreservesConfigAndRuntime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	writeReloadConfig(t, path, `return { keys = {{ key = "a", action = function(term) end }} }`)
	cfg, rt, err := script.Load(path, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	app := &App{cfg: cfg, scriptRT: rt, configPath: path, mux: termmux.New(nil, termmux.Options{})}
	defer func() {
		if app.scriptRT != nil {
			app.scriptRT.Close()
		}
	}()
	app.configWatch = newConfigWatchState(path)
	app.ensureConfigState()
	desiredBefore := app.DesiredConfig()
	pendingBefore := app.PendingConfigChanges()
	before := app.cfg
	writeReloadConfig(t, path, `return { colors = { background = "bad" } }`)
	if err := app.reloadConfig(); err == nil {
		t.Fatal("invalid candidate should fail")
	}
	if app.scriptRT != rt {
		t.Fatal("failed reload replaced active runtime")
	}
	if !reflect.DeepEqual(app.cfg, before) {
		t.Fatalf("failed reload mutated active config: %#v", app.cfg)
	}
	if !reflect.DeepEqual(app.DesiredConfig(), desiredBefore) || !reflect.DeepEqual(app.PendingConfigChanges(), pendingBefore) {
		t.Fatal("failed reload mutated desired or pending config state")
	}
	if app.LastConfigReloadError() == "" {
		t.Fatal("failed reload did not retain diagnostic error")
	}
	writeReloadConfig(t, path, `return {}`)
	if err := app.reloadConfig(); err != nil {
		t.Fatalf("recovery reload: %v", err)
	}
	if app.LastConfigReloadError() != "" {
		t.Fatalf("successful recovery retained error %q", app.LastConfigReloadError())
	}
}

func TestReloadCommitsLiveFieldsAndRetainsScopedPendingChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	writeReloadConfig(t, path, `return {}`)
	cfg, rt, err := script.Load(path, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	app := &App{cfg: cfg, scriptRT: rt, configPath: path, mux: termmux.New(nil, termmux.Options{})}
	app.configWatch = newConfigWatchState(path)
	writeReloadConfig(t, path, `return {
		window = { opacity = 0.8 },
		colors = { background = "#010203FF" },
		scrolling = { history = 7, wheel_multiplier = 4, hide_cursor_when_scrolled = false },
		cursor = { shape = "block", blink = false, blink_interval_ms = 500, thickness = 0.2 },
		shell = { program = "future-shell" },
		font = { family = "Future Font" },
	}`)
	if err := app.reloadConfig(); err != nil {
		t.Fatalf("reloadConfig: %v", err)
	}
	defer func() {
		if app.scriptRT != nil {
			app.scriptRT.Close()
		}
	}()
	if app.scriptRT == rt {
		t.Fatal("successful reload did not replace runtime")
	}
	if app.cfg.Window.Opacity != .8 || app.cfg.Colors.Background != "#010203FF" || app.cfg.Scrolling.History != 7 || app.cfg.Cursor.Shape != "block" || app.cfg.Cursor.Blink {
		t.Fatalf("live fields not committed: %#v", app.cfg)
	}
	if app.cfg.Shell.Program == "future-shell" || app.cfg.Font.Family == "Future Font" {
		t.Fatal("non-live scoped field was hot-applied")
	}
	if app.DesiredConfig().Shell.Program != "future-shell" || app.DesiredConfig().Font.Family != "Future Font" {
		t.Fatal("desired config did not retain candidate values")
	}
	wantPending := []config.ConfigChange{{Path: "font.family", Scope: config.ApplyRestart}, {Path: "shell.program", Scope: config.ApplyNewPane}}
	if got := app.PendingConfigChanges(); !reflect.DeepEqual(got, wantPending) {
		t.Fatalf("pending changes = %#v, want %#v", got, wantPending)
	}
	if !strings.Contains(app.notice, "font.family (restart)") || !strings.Contains(app.notice, "shell.program (new_pane)") {
		t.Fatalf("scoped notice = %q", app.notice)
	}
	if app.LastConfigReloadError() != "" {
		t.Fatalf("successful reload retained error %q", app.LastConfigReloadError())
	}
	writeReloadConfig(t, path, `return {}`)
	if err := app.reloadConfig(); err != nil {
		t.Fatalf("revert scoped config: %v", err)
	}
	if got := app.PendingConfigChanges(); len(got) != 0 {
		t.Fatalf("reverted pending changes = %#v", got)
	}
	if app.notice != "config reloaded" {
		t.Fatalf("revert notice = %q", app.notice)
	}
}

func TestRuntimeLiveSetterPreservesUnrelatedDesiredPendingChanges(t *testing.T) {
	app := newRunningMuxTestApp(t)
	app.cfg = config.Defaults()
	app.cfg.Colors.Background = "#080B12"
	app.desiredCfg = app.cfg.Clone()
	app.desiredCfg.Shell.Program = "future-shell"
	app.desiredCfg.Font.Family = "Future Font"
	app.composedCfg = app.desiredCfg.Clone()
	app.configStateInitialized = true
	app.pendingConfig = config.PendingConfigChanges(app.desiredCfg, app.cfg)

	next := app.RuntimeConfig()
	next.Window.Opacity = 0.8
	if err := app.ApplyRuntimeConfig(next); err != nil {
		t.Fatal(err)
	}
	if app.EffectiveConfig().Window.Opacity != 0.8 || app.DesiredConfig().Window.Opacity != 0.8 {
		t.Fatal("runtime live setter did not update desired and effective live value")
	}
	if app.DesiredConfig().Shell.Program != "future-shell" || app.DesiredConfig().Font.Family != "Future Font" {
		t.Fatal("runtime live setter erased unrelated desired values")
	}
	want := []config.ConfigChange{{Path: "font.family", Scope: config.ApplyRestart}, {Path: "shell.program", Scope: config.ApplyNewPane}}
	if got := app.PendingConfigChanges(); !reflect.DeepEqual(got, want) {
		t.Fatalf("pending after runtime setter = %#v", got)
	}
}

func TestRuntimeScopeSurvivesReloadRejectsInvalidCandidateAndClears(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	writeReloadConfig(t, path, `return {colors={background="#080B12"}}`)
	cfg, rt, err := script.Load(path, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	app := &App{cfg: cfg, scriptRT: rt, configPath: path, mux: termmux.New(nil, termmux.Options{})}
	defer func() {
		if app.scriptRT != nil {
			app.scriptRT.Close()
		}
	}()
	app.configWatch = newConfigWatchState(path)
	app.ensureConfigState()
	scope := app.ConfigScopeID()

	next := app.RuntimeConfig()
	next.Window.Opacity = 0.8
	if err := app.ApplyRuntimeConfig(next); err != nil {
		t.Fatal(err)
	}
	wantRecord := []config.RuntimeOverrideRecord{{Path: "window.opacity", Scope: scope}}
	if got := app.RuntimeConfigOverrides(); !reflect.DeepEqual(got, wantRecord) {
		t.Fatalf("runtime records = %#v", got)
	}
	provenance := app.RuntimeConfigProvenance()
	if len(provenance) != 1 || provenance[0].Path != "window.opacity" || provenance[0].Winner.ConfigScopeID != scope || provenance[0].Winner.Layer != config.LayerRuntime {
		t.Fatalf("runtime app provenance = %#v", provenance)
	}

	writeReloadConfig(t, path, `return {window={opacity=0.95},colors={foreground="#112233",background="#080B12"}}`)
	if err := app.reloadConfig(); err != nil {
		t.Fatal(err)
	}
	if app.EffectiveConfig().Window.Opacity != 0.8 || app.DesiredConfig().Window.Opacity != 0.8 || app.EffectiveConfig().Colors.Foreground != "#112233" {
		t.Fatalf("runtime scope did not survive reload: effective=%#v desired=%#v", app.EffectiveConfig(), app.DesiredConfig())
	}
	beforeEffective, beforeDesired, beforeRT := app.EffectiveConfig(), app.DesiredConfig(), app.scriptRT
	writeReloadConfig(t, path, `return {colors={background="#01020380"}}`)
	if err := app.reloadConfig(); err == nil || !strings.Contains(err.Error(), "cannot be enabled together") {
		t.Fatalf("invalid scoped reload error = %v", err)
	}
	if app.scriptRT != beforeRT || !reflect.DeepEqual(app.EffectiveConfig(), beforeEffective) || !reflect.DeepEqual(app.DesiredConfig(), beforeDesired) || !reflect.DeepEqual(app.RuntimeConfigOverrides(), wantRecord) {
		t.Fatal("invalid scoped reload mutated active state")
	}

	if err := app.ClearRuntimeConfigOverrides("window.opacity"); err != nil {
		t.Fatal(err)
	}
	if app.EffectiveConfig().Window.Opacity != 0.95 || len(app.RuntimeConfigOverrides()) != 0 {
		t.Fatalf("cleared scope effective=%#v records=%#v", app.EffectiveConfig(), app.RuntimeConfigOverrides())
	}
}

func TestNewPaneUsesDesiredShellWithoutChangingEffectiveWindowConfig(t *testing.T) {
	factory := &capturingTestFactory{}
	mux := termmux.New(factory, termmux.Options{})
	_, pane, events, err := mux.Bootstrap(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mux.Shutdown() }()
	app := &App{cfg: config.Defaults(), mux: mux, focusedPane: pane, paneUI: make(map[termmux.PaneID]*paneUIState), pendingPaneScroll: make(map[termmux.PaneID]int)}
	app.handleMuxEvents(events)
	app.ensureConfigState()
	app.desiredCfg.Shell.Program = "future-shell"
	app.desiredCfg.Shell.Args = []string{"--future"}
	app.desiredCfg.Shell.WorkingDirectory = "future-directory"
	app.desiredCfg.Shell.Env = map[string]string{"FUTURE": "1"}
	app.pendingConfig = config.PendingConfigChanges(app.desiredCfg, app.cfg)
	factory.reset()

	if err := app.executeSplitAction(pane, termaction.SplitPane{Axis: termaction.SplitColumns}); err != nil {
		t.Fatal(err)
	}
	spawn, ok := factory.last()
	if !ok {
		t.Fatal("split did not spawn a new pane")
	}
	if spawn.ShellProgram != "future-shell" || !reflect.DeepEqual(spawn.ShellArgs, []string{"--future"}) || spawn.WorkingDirectory != "future-directory" || spawn.Env["FUTURE"] != "1" {
		t.Fatalf("new pane spawn options = %#v", spawn)
	}
	if app.EffectiveConfig().Shell.Program == "future-shell" {
		t.Fatal("new-pane desired shell leaked into effective window config")
	}
}

func TestReloadActivatesExplicitV2BundleAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cervterm.lua")
	writeReloadConfig(t, path, `return {}`)
	loaded, err := script.LoadVersioned(path, config.Defaults(), script.CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{cfg: loaded.Config, scriptRT: loaded.Runtime, configPath: path, mux: termmux.New(nil, termmux.Options{})}
	app.configWatch = newConfigWatchState(path)
	writeReloadConfig(t, filepath.Join(dir, "base.lua"), `return {colors={foreground="#AABBCC"}}`)
	writeReloadConfig(t, path, `local c=require("cervterm"); return {config_version=2,includes={"base.lua"},window={opacity=0.8},colors={background="#080B12"},keys={{key="k",action=c.action.ScrollPage(1)}}}`)
	oldRT := app.scriptRT
	if err := app.reloadConfig(); err != nil {
		t.Fatalf("reload explicit v2: %v", err)
	}
	if app.scriptBundle == nil || app.scriptRT == nil || app.scriptRT == oldRT {
		t.Fatal("v2 reload did not atomically install bundle/runtime")
	}
	defer app.scriptBundle.Close()
	if app.cfg.Window.Opacity != 0.8 || app.cfg.Colors.Foreground != "#AABBCC" {
		t.Fatalf("v2 live config = %#v", app.cfg)
	}
	if len(app.scriptRT.Bindings()) != 1 {
		t.Fatalf("v2 runtime bindings = %#v", app.scriptRT.Bindings())
	}
}

func TestReloadV2PublicationFailurePreservesActiveState(t *testing.T) {
	if _, err := exec.LookPath("tl"); err != nil {
		t.Skip("tl not installed")
	}
	dir := t.TempDir()
	copyReloadFile(t, filepath.Join("..", "..", "..", "docs", "examples", "cervterm.d.tl"), filepath.Join(dir, "cervterm.d.tl"))
	path := filepath.Join(dir, "cervterm.tl")
	writeReloadConfig(t, path, `local c=require("cervterm")
	local cfg: c.Config = {}
	return cfg`)
	loaded, err := script.LoadVersioned(path, config.Defaults(), script.CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Runtime.Close()
	published := filepath.Join(dir, "cervterm.lua")
	if err := os.Remove(published); err != nil {
		t.Fatal(err)
	}
	app := &App{cfg: loaded.Config, scriptRT: loaded.Runtime, configPath: path, mux: termmux.New(nil, termmux.Options{})}
	app.configWatch = newConfigWatchState(path)
	app.tealPublicationOptions.FaultInjector = func(_ int, step string) error {
		if step == "marker" {
			return errors.New("publication fault")
		}
		return nil
	}
	writeReloadConfig(t, path, `local c=require("cervterm")
	local cfg: c.Config = {config_version=2,window={opacity=0.8},colors={background="#080B12"}}
	return cfg`)
	before, oldRT := app.cfg, app.scriptRT
	if err := app.reloadConfig(); err == nil || !strings.Contains(err.Error(), "publication fault") {
		t.Fatalf("publication reload error = %v", err)
	}
	if app.scriptRT != oldRT || app.scriptBundle != nil || !reflect.DeepEqual(app.cfg, before) {
		t.Fatal("failed publication mutated active config ownership")
	}
	if _, err := os.Stat(published); !os.IsNotExist(err) {
		t.Fatalf("failed publication left generated output: %v", err)
	}
	if _, err := os.Stat(config.TealOwnershipMarkerPath(published)); !os.IsNotExist(err) {
		t.Fatalf("failed publication left ownership marker: %v", err)
	}
}

func TestReloadV2ToV1PreparationFailureRestoresTealArtifacts(t *testing.T) {
	if _, err := exec.LookPath("tl"); err != nil {
		t.Skip("tl not installed")
	}
	dir := t.TempDir()
	copyReloadFile(t, filepath.Join("..", "..", "..", "docs", "examples", "cervterm.d.tl"), filepath.Join(dir, "cervterm.d.tl"))
	path := filepath.Join(dir, "cervterm.tl")
	writeReloadConfig(t, path, `local c=require("cervterm")
	local cfg: c.Config = {config_version=2,colors={background="#080B12"}}
	return cfg`)
	loaded, err := script.LoadVersioned(path, config.Defaults(), script.CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loaded.Candidate.PublishTeal(config.TealPublicationOptions{}); err != nil {
		loaded.Candidate.Close()
		t.Fatal(err)
	}
	activation, err := loaded.Candidate.PrepareActivation()
	if err != nil {
		loaded.Candidate.Close()
		t.Fatal(err)
	}
	app := &App{cfg: loaded.Config, scriptRT: activation.Commit(), scriptBundle: loaded.Candidate, configPath: path, mux: termmux.New(nil, termmux.Options{}), paneUI: make(map[termmux.PaneID]*paneUIState)}
	defer app.scriptBundle.Close()
	app.cfg.Render.TextRaster = "subpixel"
	app.contentScaleX, app.contentScaleY = 1, 1
	app.configWatch = newConfigWatchState(path)
	failFactory := false
	atlas, err := newGlyphAtlasWithBackendFactory(&atlasTestRenderer{}, fontglyph.Spec{Family: app.cfg.Font.Family, Size: app.cfg.Font.Size, DPI: 96, TextRaster: "subpixel"}, app.cfg.Render.TextGamma, app.cfg.Render.TextDarken, func(spec fontglyph.Spec) (fontglyph.Backend, error) {
		if failFactory {
			return nil, errors.New("raster preparation fault")
		}
		return &atlasTestBackend{cellW: int(spec.Size), cellH: int(spec.Size * 2), baseline: int(spec.Size)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	app.atlas = atlas
	t.Cleanup(atlas.close)
	failFactory = true

	published := filepath.Join(dir, "cervterm.lua")
	marker := config.TealOwnershipMarkerPath(published)
	oldOutput, err := os.ReadFile(published)
	if err != nil {
		t.Fatal(err)
	}
	oldMarker, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	writeReloadConfig(t, path, `local c=require("cervterm")
	local cfg: c.Config = {colors={background="#01020380"}}
	return cfg`)
	oldRT, oldBundle, oldConfig := app.scriptRT, app.scriptBundle, app.cfg
	if err := app.reloadConfig(); err == nil || !strings.Contains(err.Error(), "raster preparation fault") {
		t.Fatalf("v2-to-v1 preparation error = %v", err)
	}
	if app.scriptRT != oldRT || app.scriptBundle != oldBundle || !reflect.DeepEqual(app.cfg, oldConfig) {
		t.Fatal("failed v2-to-v1 preparation changed active ownership")
	}
	if output, _ := os.ReadFile(published); !reflect.DeepEqual(output, oldOutput) {
		t.Fatal("failed v2-to-v1 preparation did not restore generated Lua")
	}
	if ownership, _ := os.ReadFile(marker); !reflect.DeepEqual(ownership, oldMarker) {
		t.Fatal("failed v2-to-v1 preparation did not restore ownership marker")
	}
}

func TestReloadQueuesNewGenerationWhenActiveIncludeChangesDuringEvaluation(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "config.lua")
	include := filepath.Join(dir, "base.lua")
	writeReloadConfig(t, include, `return {font={family="Old"}}`)
	writeReloadConfig(t, primary, `return {config_version=2,includes={"base.lua"}}`)
	loaded, err := script.LoadVersioned(primary, config.Defaults(), script.CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	activation, err := loaded.Candidate.PrepareActivation()
	if err != nil {
		loaded.Candidate.Close()
		t.Fatal(err)
	}
	app := &App{cfg: loaded.Config, scriptRT: activation.Commit(), scriptBundle: loaded.Candidate, configPath: primary, mux: termmux.New(nil, termmux.Options{})}
	defer func() {
		if app.scriptBundle != nil {
			app.scriptBundle.Close()
		}
	}()
	app.configWatch = newConfigWatchState(loaded.WatchPaths...)
	body := fmt.Sprintf(`local f=assert(io.open(%q,"w"))
	f:write('return {font={family="New"}}')
	f:close()
	return {config_version=2,includes={"base.lua"}}`, include)
	writeReloadConfig(t, primary, body)
	if err := app.reloadConfig(); err != nil {
		t.Fatal(err)
	}
	if app.scriptBundle == nil || app.scriptBundle.Config().Font.Family != "New" {
		t.Fatal("candidate did not consume include edit made during evaluation")
	}
	if !app.reloadPending {
		t.Fatal("include edit during evaluation was acknowledged away")
	}
	if len(app.configWatch.paths) != 2 {
		t.Fatalf("active graph watch paths = %#v", app.configWatch.paths)
	}
}

func TestReloadQueuesNewlyIntroducedIncludeChangedDuringItsEvaluation(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "config.lua")
	include := filepath.Join(dir, "new.lua")
	writeReloadConfig(t, primary, `return {config_version=2}`)
	loaded, err := script.LoadVersioned(primary, config.Defaults(), script.CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	activation, err := loaded.Candidate.PrepareActivation()
	if err != nil {
		loaded.Candidate.Close()
		t.Fatal(err)
	}
	app := &App{cfg: loaded.Config, scriptRT: activation.Commit(), scriptBundle: loaded.Candidate, configPath: primary, mux: termmux.New(nil, termmux.Options{})}
	defer func() {
		if app.scriptBundle != nil {
			app.scriptBundle.Close()
		}
	}()
	app.configWatch = newConfigWatchState(loaded.WatchPaths...)
	includeBody := fmt.Sprintf(`local f=assert(io.open(%q,"w"))
	f:write('return {font={family="Newest"}}')
	f:close()
	return {font={family="Snapshot"}}`, include)
	writeReloadConfig(t, include, includeBody)
	writeReloadConfig(t, primary, `return {config_version=2,includes={"new.lua"}}`)
	if err := app.reloadConfig(); err != nil {
		t.Fatal(err)
	}
	if app.scriptBundle == nil || app.scriptBundle.Config().Font.Family != "Snapshot" {
		t.Fatal("candidate did not retain the evaluated include snapshot")
	}
	if !app.reloadPending {
		t.Fatal("new include edit during evaluation was acknowledged away")
	}
	if len(app.configWatch.paths) != 2 {
		t.Fatalf("new active graph watch paths = %#v", app.configWatch.paths)
	}
}

func TestWatchHashesDetectModuleMutationDuringRequire(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "config.lua")
	module := filepath.Join(dir, "mod.lua")
	moduleBody := fmt.Sprintf(`local f=assert(io.open(%q,"w"))
	f:write('return {value="new"}')
	f:close()
	return {value="snapshot"}`, module)
	writeReloadConfig(t, module, moduleBody)
	primaryBody := fmt.Sprintf(`package.path=%q..";"..package.path; local m=require("mod"); return {config_version=2}`, filepath.ToSlash(dir)+"/?.lua")
	writeReloadConfig(t, primary, primaryBody)
	loaded, err := script.LoadVersioned(primary, config.Defaults(), script.CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Candidate.Close()
	if !watchHashesChanged(loaded.WatchHashes) {
		t.Fatal("module mutation during require matched the evaluated generation")
	}
}

func TestWatchHashesDetectSymlinkRetargetWithIdenticalContent(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.lua")
	second := filepath.Join(dir, "second.lua")
	link := filepath.Join(dir, "config.lua")
	body := `return {config_version=2}`
	writeReloadConfig(t, first, body)
	writeReloadConfig(t, second, body)
	if err := os.Symlink(first, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	loaded, err := script.LoadVersioned(link, config.Defaults(), script.CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Candidate.Close()
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}
	if !watchHashesChanged(loaded.WatchHashes) {
		t.Fatal("symlink retarget with identical bytes was not detected")
	}
}

func TestWatchHashesKeepEveryDeclarativeSymlinkAlias(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.lua")
	other := filepath.Join(dir, "other.lua")
	firstAlias := filepath.Join(dir, "first.lua")
	secondAlias := filepath.Join(dir, "second.lua")
	primary := filepath.Join(dir, "config.lua")
	body := `return {font={family="Alias"}}`
	writeReloadConfig(t, target, body)
	writeReloadConfig(t, other, body)
	if err := os.Symlink(target, firstAlias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.Symlink(target, secondAlias); err != nil {
		t.Fatal(err)
	}
	writeReloadConfig(t, primary, `return {config_version=2,includes={"first.lua","second.lua"}}`)
	loaded, err := script.LoadVersioned(primary, config.Defaults(), script.CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Candidate.Close()
	if _, ok := loaded.WatchHashes[secondAlias]; !ok {
		t.Fatalf("duplicate alias omitted from watch hashes: %#v", loaded.WatchPaths)
	}
	if err := os.Remove(secondAlias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, secondAlias); err != nil {
		t.Fatal(err)
	}
	if !watchHashesChanged(loaded.WatchHashes) {
		t.Fatal("retargeted duplicate declarative alias was not detected")
	}
}

func TestWatchHashesKeepDependencySymlinkPath(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first-module.lua")
	second := filepath.Join(dir, "second-module.lua")
	moduleLink := filepath.Join(dir, "mod.lua")
	primary := filepath.Join(dir, "config.lua")
	body := `return {value="same"}`
	writeReloadConfig(t, first, body)
	writeReloadConfig(t, second, body)
	if err := os.Symlink(first, moduleLink); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	primaryBody := fmt.Sprintf(`package.path=%q..";"..package.path; local m=require("mod"); return {config_version=2}`, filepath.ToSlash(dir)+"/?.lua")
	writeReloadConfig(t, primary, primaryBody)
	loaded, err := script.LoadVersioned(primary, config.Defaults(), script.CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Candidate.Close()
	if _, ok := loaded.WatchHashes[moduleLink]; !ok {
		t.Fatalf("dependency symlink omitted from watch hashes: %#v", loaded.WatchPaths)
	}
	if err := os.Remove(moduleLink); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, moduleLink); err != nil {
		t.Fatal(err)
	}
	if !watchHashesChanged(loaded.WatchHashes) {
		t.Fatal("dependency symlink retarget was not detected")
	}
}

func TestRequestConfigReloadIsDeferred(t *testing.T) {
	app := &App{configPath: "cervterm.lua"}
	if !app.RequestConfigReload() || !app.reloadPending {
		t.Fatal("request should only mark pending reload")
	}
}

func TestReloadRetainsStartupSelectionAndCLIOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.lua")
	writeReloadConfig(t, path, `return {config_version=2,colors={background="#080B12"},window={opacity=0.4},environments={windows={window={opacity=0.5}}},profiles={work={window={opacity=0.6}}}}`)
	environment, profile := "windows", "work"
	options := script.CandidateOptions{Composition: config.CompositionOptions{
		Selection:    config.SelectionOptions{EnvironmentOverride: &environment, ProfileOverride: &profile, GOOS: "linux"},
		CLIOverrides: []config.CLIOverride{{ArgumentIndex: 7, Path: "window.opacity", Value: "0.7"}},
	}}
	loaded, err := script.LoadVersioned(path, config.Defaults(), options)
	if err != nil {
		t.Fatal(err)
	}
	activation, err := loaded.Candidate.PrepareActivation()
	if err != nil {
		loaded.Candidate.Close()
		t.Fatal(err)
	}
	app := &App{
		cfg: loaded.Config, desiredCfg: loaded.Config, composedCfg: loaded.Config, configStateInitialized: true,
		scriptRT: activation.Commit(), scriptBundle: loaded.Candidate, candidateOptions: loaded.Candidate.Options(),
		configPath: path, configWatch: newConfigWatchState(path), mux: termmux.New(nil, termmux.Options{}), paneUI: make(map[termmux.PaneID]*paneUIState),
	}
	defer app.scriptBundle.Close()
	if app.cfg.Window.Opacity != 0.7 {
		t.Fatalf("startup override opacity = %v", app.cfg.Window.Opacity)
	}
	// Mutating caller-owned inputs cannot change the frontend's reload snapshot.
	environment, profile = "mutated", "mutated"
	options.Composition.CLIOverrides[0].Value = "0.1"
	writeReloadConfig(t, path, `return {config_version=2,colors={background="#080B12"},window={opacity=0.45},environments={windows={window={opacity=0.55}}},profiles={work={window={opacity=0.65}}}}`)
	if err := app.reloadConfig(); err != nil {
		t.Fatal(err)
	}
	if app.cfg.Window.Opacity != 0.7 {
		t.Fatalf("reload lost CLI override: %v", app.cfg.Window.Opacity)
	}
	selection := app.scriptBundle.Selection()
	if selection.Environment == nil || selection.Environment.Name != "windows" || selection.Profile == nil || selection.Profile.Name != "work" {
		t.Fatalf("reload selection = %#v", selection)
	}
	var opacity config.ProvenanceRecord
	for _, record := range app.scriptBundle.Provenance() {
		if record.Path == "window.opacity" {
			opacity = record
			break
		}
	}
	if opacity.Winner.Layer != config.LayerCLI || !opacity.Winner.HasCLIArgumentIndex || opacity.Winner.CLIArgumentIndex != 7 {
		t.Fatalf("reload CLI provenance = %#v", opacity)
	}
	oldBundle, oldConfig := app.scriptBundle, app.cfg
	writeReloadConfig(t, path, `return {colors={background="#080B12"},window={opacity=0.9}}`)
	if err := app.reloadConfig(); err == nil || !strings.Contains(err.Error(), "require config_version=2") {
		t.Fatalf("v1 transition with explicit options error = %v", err)
	}
	if app.scriptBundle != oldBundle || !reflect.DeepEqual(app.cfg, oldConfig) {
		t.Fatal("rejected v1 transition changed active bundle or config")
	}
}

func TestReloadV1ToV2RetainsAmbientSelectionSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.lua")
	writeReloadConfig(t, path, `return {colors={background="#080B12"}}`)
	environment, profile := "windows", "work"
	options := script.CandidateOptions{Composition: config.CompositionOptions{Selection: config.SelectionOptions{
		EnvironmentVariableValue: &environment, ProfileVariableValue: &profile, GOOS: "linux",
	}}}
	loaded, err := script.LoadVersioned(path, config.Defaults(), options)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{
		cfg: loaded.Config, desiredCfg: loaded.Config, composedCfg: loaded.Config, configStateInitialized: true,
		scriptRT: loaded.Runtime, candidateOptions: loaded.Options, configPath: path, configWatch: newConfigWatchState(path),
		mux: termmux.New(nil, termmux.Options{}), paneUI: make(map[termmux.PaneID]*paneUIState),
	}
	defer func() {
		if app.scriptBundle != nil {
			app.scriptBundle.Close()
		} else if app.scriptRT != nil {
			app.scriptRT.Close()
		}
	}()
	environment, profile = "mutated", "mutated"
	writeReloadConfig(t, path, `return {config_version=2,colors={background="#080B12"},environments={windows={window={opacity=0.8}}},profiles={work={window={opacity=0.7}}}}`)
	if err := app.reloadConfig(); err != nil {
		t.Fatal(err)
	}
	selection := app.scriptBundle.Selection()
	if selection.Environment == nil || selection.Environment.Name != "windows" || selection.Environment.Basis != config.SelectionEnvironmentVariable || selection.Profile == nil || selection.Profile.Name != "work" || selection.Profile.Basis != config.SelectionEnvironmentVariable {
		t.Fatalf("v1-to-v2 retained selection = %#v", selection)
	}
	if app.cfg.Window.Opacity != 0.7 {
		t.Fatalf("v1-to-v2 selected profile opacity = %v", app.cfg.Window.Opacity)
	}
}
