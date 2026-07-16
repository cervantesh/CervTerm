//go:build glfw

package glfwgl

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestRequestConfigReloadIsDeferred(t *testing.T) {
	app := &App{configPath: "cervterm.lua"}
	if !app.RequestConfigReload() || !app.reloadPending {
		t.Fatal("request should only mark pending reload")
	}
}
