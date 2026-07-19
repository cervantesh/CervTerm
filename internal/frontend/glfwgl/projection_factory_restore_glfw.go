//go:build glfw

package glfwgl

import (
	"fmt"

	"cervterm/internal/config"
	"cervterm/internal/layoutrestore"
	termmux "cervterm/internal/mux"

	"github.com/go-gl/glfw/v3.3/glfw"
)

type projectionAppearance struct {
	ColorScheme       string
	BackgroundOpacity float64
	TextOpacity       float64
	Blur              bool
	FontSize          float64
}

func setRestoreAppearance(app *App, appearance layoutrestore.BlueprintAppearance) {
	if app == nil {
		return
	}
	value := projectionAppearance{ColorScheme: appearance.ColorScheme, BackgroundOpacity: appearance.BackgroundOpacity, TextOpacity: appearance.TextOpacity, Blur: appearance.Blur, FontSize: appearance.FontSize}
	if app.projectionBaseConfig == nil {
		base := app.cfg.Clone()
		app.projectionBaseConfig = &base
	}
	app.restoreAppearance = &value
	app.cfg = app.configWithRestoreAppearance(app.cfg)
}

func (a *App) configWithRestoreAppearance(base config.Config) config.Config {
	out := base.Clone()
	if a == nil || a.restoreAppearance == nil {
		return out
	}
	appearance := a.restoreAppearance
	out.Window.BackgroundOpacity = appearance.BackgroundOpacity
	out.Window.TextOpacity = appearance.TextOpacity
	out.Window.Blur = appearance.Blur
	out.Font.Size = appearance.FontSize
	return out
}

func (a *App) projectionBase() config.Config {
	if a != nil && a.projectionBaseConfig != nil {
		return a.projectionBaseConfig.Clone()
	}
	if a == nil {
		return config.Config{}
	}
	return a.cfg.Clone()
}

// glfwRestoreProjectionFactory realizes prepared blueprint windows in exact
// workspace/window traversal order. The controller keeps every host hidden
// until the mux restore transaction commits.
type glfwRestoreProjectionFactory struct {
	owner   *App
	windows []layoutrestore.Window
}

func (f *glfwRestoreProjectionFactory) PrepareRestore(index int) (*nativeProjectionBundle, termmux.RestoreWindowGeometry, error) {
	if f == nil || f.owner == nil || f.owner.controller == nil || f.owner.mux == nil || index < 0 || index >= len(f.windows) {
		return nil, termmux.RestoreWindowGeometry{}, errWindowProjectionMissing
	}
	windowPlan := f.windows[index]
	bounds := windowPlan.Bounds.Bounds
	child := f.owner
	if index > 0 {
		child = newProjectionApp(f.owner)
	}
	setRestoreAppearance(child, windowPlan.Appearance)
	child.cfg.Window.Width, child.cfg.Window.Height = bounds.Width, bounds.Height
	base := &glfwProjectionFactory{owner: f.owner}
	glfw.WindowHint(glfw.Visible, glfw.False)
	defer glfw.WindowHint(glfw.Visible, glfw.True)
	bundle, _, content, metrics, _, err := base.prepareProjection(child, bounds.Width, bounds.Height, bounds.X, bounds.Y, true, false)
	if err != nil {
		return bundle, termmux.RestoreWindowGeometry{}, fmt.Errorf("prepare restored window %d: %w", index, err)
	}
	return bundle, termmux.RestoreWindowGeometry{Content: content, Metrics: metrics}, nil
}

func restoreBlueprintWindows(blueprint layoutrestore.Blueprint) []layoutrestore.Window {
	snapshot := blueprint.Snapshot()
	count := 0
	for _, workspace := range snapshot.Workspaces {
		count += len(workspace.Windows)
	}
	windows := make([]layoutrestore.Window, 0, count)
	for _, workspace := range snapshot.Workspaces {
		windows = append(windows, workspace.Windows...)
	}
	return windows
}

var _ nativeRestoreProjectionFactory = (*glfwRestoreProjectionFactory)(nil)
