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
	defer rt.Close()
	app := &App{cfg: cfg, scriptRT: rt, configPath: path, mux: termmux.New(nil, termmux.Options{})}
	app.configWatch = newConfigWatchState(path)
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
}

func TestReloadCommitsLiveFieldsAndReportsRestartFields(t *testing.T) {
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
		shell = { program = "not-live" },
	}`)
	if err := app.reloadConfig(); err != nil {
		t.Fatalf("reloadConfig: %v", err)
	}
	defer app.scriptRT.Close()
	if app.scriptRT == rt {
		t.Fatal("successful reload did not replace runtime")
	}
	if app.cfg.Window.Opacity != .8 || app.cfg.Colors.Background != "#010203FF" || app.cfg.Scrolling.History != 7 {
		t.Fatalf("live fields not committed: %#v", app.cfg)
	}
	if app.cfg.Shell.Program == "not-live" {
		t.Fatal("startup-only shell field was hot-applied")
	}
	if app.notice == "" {
		t.Fatal("reload did not produce visible notice")
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
