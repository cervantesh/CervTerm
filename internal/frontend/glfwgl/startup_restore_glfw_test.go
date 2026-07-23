//go:build glfw

package glfwgl

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cervterm/internal/config"
	"cervterm/internal/layoutrestore"
	"cervterm/internal/layoutstate"
	termmux "cervterm/internal/mux"
	"cervterm/internal/windowbounds"
)

func startupRestorePlan(t *testing.T, launch layoutstate.Launch) layoutstate.Plan {
	t.Helper()
	plan, err := layoutstate.NewPlan(layoutstate.Document{Version: 1, Workspaces: []layoutstate.Workspace{{Name: "default", ActiveWindow: 0, Windows: []layoutstate.Window{{
		Title: "restored", Bounds: layoutstate.Bounds{X: 10, Y: 20, Width: 900, Height: 600}, ActiveTab: 0,
		Tabs: []layoutstate.Tab{{Title: "tab", FocusedLeaf: 0, Root: layoutstate.Node{Type: "pane", Launch: &launch}}},
	}}}}})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestLoadConfiguredRestorePlanIsOptInAndUsesConfiguredStore(t *testing.T) {
	cfg := config.Defaults()
	if _, found, err := loadConfiguredRestorePlan(cfg); err != nil || found {
		t.Fatalf("disabled=%v,%v", found, err)
	}
	path := filepath.Join(t.TempDir(), "layout.json")
	store, err := layoutstate.NewStore(layoutstate.StoreOptions{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	plan := startupRestorePlan(t, layoutstate.Launch{})
	if err := store.Save(plan); err != nil {
		t.Fatal(err)
	}
	cfg.LayoutPersistence.Enabled, cfg.LayoutPersistence.Path = true, path
	loaded, found, err := loadConfiguredRestorePlan(cfg)
	if err != nil || !found || loaded.Snapshot().Workspaces[0].Windows[0].Title != "restored" {
		t.Fatalf("loaded=%#v found=%v err=%v", loaded.Snapshot(), found, err)
	}
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, found, err := loadConfiguredRestorePlan(cfg); err == nil || !found {
		t.Fatalf("corrupt found=%v err=%v", found, err)
	}
}

func TestRestoreLoadPolicyTurnsInvalidStateIntoFreshValueFreeFallback(t *testing.T) {
	plan := startupRestorePlan(t, layoutstate.Launch{})
	got, found, disposition := applyRestoreLoadPolicy(plan, true, errors.New("SECRET layout path and payload"))
	if found || got.Snapshot().Version != 0 || disposition != "invalid-or-unavailable" {
		t.Fatalf("fallback plan=%#v found=%v disposition=%q", got.Snapshot(), found, disposition)
	}
	if strings.Contains(disposition, "SECRET") {
		t.Fatalf("fallback disposition leaked source error: %q", disposition)
	}
}

func TestPrepareConfiguredRestoreUsesCurrentTargetsAppearanceAndNoEnvironment(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program, cfg.Shell.Args, cfg.Shell.WorkingDirectory = "default-shell", []string{"--default"}, t.TempDir()
	cfg.Shell.Env = map[string]string{"SECRET": "no"}
	targetCWD := t.TempDir()
	cfg.LaunchMenu = []config.LaunchTarget{{ID: "dev", Program: "pwsh", Args: []string{"-NoLogo"}, CWD: targetCWD, Env: map[string]string{"TOKEN": "secret"}}}
	cfg.ColorScheme, cfg.Window.BackgroundOpacity, cfg.Window.TextOpacity, cfg.Window.Blur, cfg.Font.Size = "scheme", .8, .7, true, 15
	blueprint, err := prepareConfiguredRestore(cfg, startupRestorePlan(t, layoutstate.Launch{TargetID: "dev", Program: "stale", Args: []string{"bad"}}), []windowbounds.Monitor{{ID: "one", WorkArea: windowbounds.Rect{Width: 1920, Height: 1080}, ScaleX: 1, ScaleY: 1, Primary: true}})
	if err != nil {
		t.Fatal(err)
	}
	window := blueprint.Snapshot().Workspaces[0].Windows[0]
	launch := window.Tabs[0].Root.Launch
	if launch.Program != "pwsh" || len(launch.Args) != 1 || launch.Args[0] != "-NoLogo" || launch.CWD != targetCWD {
		t.Fatalf("launch=%#v", launch)
	}
	if window.Appearance.ColorScheme != "scheme" || window.Appearance.BackgroundOpacity != .8 || window.Appearance.TextOpacity != .7 || !window.Appearance.Blur || window.Appearance.FontSize != 15 {
		t.Fatalf("appearance=%#v", window.Appearance)
	}
}

func TestRestoreBlueprintWindowsAndAppearanceProjectionAreDetached(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "shell"
	blueprint, err := prepareConfiguredRestore(cfg, startupRestorePlan(t, layoutstate.Launch{}), []windowbounds.Monitor{{ID: "one", WorkArea: windowbounds.Rect{Width: 1920, Height: 1080}, ScaleX: 1, ScaleY: 1, Primary: true}})
	if err != nil {
		t.Fatal(err)
	}
	windows := restoreBlueprintWindows(blueprint)
	if len(windows) != 1 {
		t.Fatalf("windows=%d", len(windows))
	}
	windows[0].Title = "mutated"
	if blueprint.Snapshot().Workspaces[0].Windows[0].Title == "mutated" {
		t.Fatal("window flattening aliases blueprint")
	}
	appearance := windows[0].Appearance
	appearance.ColorScheme, appearance.BackgroundOpacity, appearance.TextOpacity, appearance.Blur, appearance.FontSize = "other", .6, .5, true, 17
	cfg.ColorScheme = "current"
	app := &App{cfg: cfg}
	setRestoreAppearance(app, appearance)
	if app.cfg.ColorScheme != "current" || app.cfg.Window.BackgroundOpacity != .6 || app.cfg.Window.TextOpacity != .5 || !app.cfg.Window.Blur || app.cfg.Font.Size != 17 {
		t.Fatalf("cfg=%#v", app.cfg)
	}
	reloaded := cfg.Clone()
	reloaded.Window.BackgroundOpacity, reloaded.Window.TextOpacity, reloaded.Window.Blur, reloaded.Font.Size = 1, 1, false, 12
	reloaded = app.configWithRestoreAppearance(reloaded)
	if reloaded.Window.BackgroundOpacity != .6 || reloaded.Window.TextOpacity != .5 || !reloaded.Window.Blur || reloaded.Font.Size != 17 {
		t.Fatalf("reload lost appearance: %#v", reloaded)
	}
}

func TestRestoredProjectionAppearanceSurvivesSharedReloadProjection(t *testing.T) {
	base := config.Defaults()
	base.Shell.Program = "shell"
	owner := &App{cfg: base.Clone(), desiredCfg: base.Clone(), composedCfg: base.Clone(), scriptGeneration: 2, paneUI: make(map[termmux.PaneID]*paneUIState), pendingPaneScroll: make(map[termmux.PaneID]int), pendingPaneResize: make(map[termmux.PaneID]termmux.PaneGeometry)}
	setRestoreAppearance(owner, layoutrestore.BlueprintAppearance{ColorScheme: base.ColorScheme, BackgroundOpacity: .3, TextOpacity: .5, Blur: true, FontSize: 19})
	runtimeWindow := newProjectionApp(owner)
	if runtimeWindow.cfg.Window.BackgroundOpacity != base.Window.BackgroundOpacity || runtimeWindow.cfg.Window.TextOpacity != base.Window.TextOpacity || runtimeWindow.cfg.Window.Blur != base.Window.Blur || runtimeWindow.cfg.Font.Size != base.Font.Size {
		t.Fatalf("runtime window inherited restored overlay: %#v", runtimeWindow.cfg)
	}
	child := newProjectionApp(owner)
	setRestoreAppearance(child, layoutrestore.BlueprintAppearance{ColorScheme: base.ColorScheme, BackgroundOpacity: .4, TextOpacity: .6, Blur: true, FontSize: 18})
	child.scriptGeneration = 1
	var log []string
	controller := newWindowController(processServices{}, fakeNativePump{log: &log})
	if err := controller.attachApp(2, &fakeNativeWindow{id: "child", log: &log}, child, func([]termmux.Event) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	owner.cfg.Window.BackgroundOpacity, owner.cfg.Window.TextOpacity, owner.cfg.Window.Blur, owner.cfg.Font.Size = 1, 1, false, 12
	if err := controller.syncSharedProjectionState(owner); err != nil {
		t.Fatal(err)
	}
	if child.cfg.Window.BackgroundOpacity != .4 || child.cfg.Window.TextOpacity != .6 || !child.cfg.Window.Blur || child.cfg.Font.Size != 18 {
		t.Fatalf("child reload cfg=%#v", child.cfg)
	}
}

func TestResetFailedRestoreFrontendClearsAbandonedAppearance(t *testing.T) {
	base := config.Defaults()
	a := &App{cfg: base.Clone()}
	setRestoreAppearance(a, layoutrestore.BlueprintAppearance{BackgroundOpacity: .4, TextOpacity: .5, Blur: true, FontSize: 18})
	a.resetFailedRestoreFrontend(base)
	if a.restoreAppearance != nil || a.projectionBaseConfig != nil || a.cfg.Window.BackgroundOpacity != base.Window.BackgroundOpacity {
		t.Fatalf("reset app=%#v", a)
	}
}
