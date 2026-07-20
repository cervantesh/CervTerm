//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"
	"os"

	"cervterm/internal/config"
	"cervterm/internal/layoutrestore"
	"cervterm/internal/layoutstate"
	"cervterm/internal/windowbounds"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func loadConfiguredRestorePlan(cfg config.Config) (layoutstate.Plan, bool, error) {
	if !cfg.LayoutPersistence.Enabled {
		return layoutstate.Plan{}, false, nil
	}
	store, err := layoutstate.NewStore(layoutstate.StoreOptions{Path: cfg.LayoutPersistence.Path})
	if err != nil {
		return layoutstate.Plan{}, false, err
	}
	return store.Load()
}

func prepareConfiguredRestore(cfg config.Config, plan layoutstate.Plan, monitors []windowbounds.Monitor) (layoutrestore.Blueprint, error) {
	targets := make([]layoutrestore.Target, 0, len(cfg.LaunchMenu))
	for _, target := range cfg.LaunchMenu {
		targets = append(targets, layoutrestore.Target{ID: target.ID, Program: target.Program, Args: append([]string(nil), target.Args...), CWD: target.CWD})
	}
	return layoutrestore.Prepare(plan, layoutrestore.Options{
		DefaultLaunch: layoutrestore.Launch{Program: cfg.Shell.Program, Args: append([]string(nil), cfg.Shell.Args...), CWD: cfg.Shell.WorkingDirectory},
		Targets:       targets,
		Monitors:      monitors,
		Policy:        windowbounds.Policy{FallbackWidth: cfg.Window.Width, FallbackHeight: cfg.Window.Height, MinWidth: 100, MinHeight: 100, ChromeHeight: 32, MinVisibleChromeX: 32, MinVisibleChromeY: 24},
		Appearance:    layoutrestore.BlueprintAppearance{ColorScheme: cfg.ColorScheme, BackgroundOpacity: cfg.Window.BackgroundOpacity, TextOpacity: cfg.Window.TextOpacity, Blur: cfg.Window.Blur, FontSize: cfg.Font.Size},
		CWDUsable:     restoreCWDUsable,
	})
}

func restoreCWDUsable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func currentGLFWMonitors() ([]windowbounds.Monitor, error) {
	native := glfw.GetMonitors()
	if len(native) == 0 || len(native) > windowbounds.MaxMonitors {
		return nil, fmt.Errorf("restore monitors unavailable")
	}
	primary := glfw.GetPrimaryMonitor()
	monitors := make([]windowbounds.Monitor, 0, len(native))
	for index, monitor := range native {
		if monitor == nil {
			return nil, fmt.Errorf("restore monitor %d is nil", index)
		}
		x, y, width, height := monitor.GetWorkarea()
		sx, sy := monitor.GetContentScale()
		monitors = append(monitors, windowbounds.Monitor{
			ID: fmt.Sprintf("%d:%s", index, monitor.GetName()), Name: monitor.GetName(),
			WorkArea: windowbounds.Rect{X: x, Y: y, Width: width, Height: height},
			ScaleX:   float64(sx), ScaleY: float64(sy), Primary: monitor == primary,
		})
	}
	return monitors, nil
}

func (a *App) commitStartupConfiguration() error {
	if a.startupConfigCommitted {
		return nil
	}
	if watchHashesChanged(a.configWatchHashes) {
		return fmt.Errorf("configuration sources changed during frontend preparation; reload the newest generation")
	}
	if a.scriptBundle != nil {
		if _, err := a.scriptBundle.PublishTeal(a.tealPublicationOptions); err != nil {
			return err
		}
	}
	if a.scriptActivation != nil {
		a.installScriptRuntime(a.scriptActivation.Commit())
		a.scriptActivation = nil
		a.initActionBindings()
	}
	if a.legacyTransition != nil {
		a.legacyTransition.Commit()
		a.legacyTransition = nil
	}
	a.startupConfigCommitted = true
	return nil
}

func (a *App) commitRestoredStartupConfiguration() error {
	if err := a.commitStartupConfiguration(); err != nil {
		return err
	}
	return a.controller.syncPendingRestoreApps(a)
}

func (a *App) tryRunRestoredWindow(blueprint layoutrestore.Blueprint) (bool, error) {
	freshConfig := a.cfg.Clone()
	a.controller = newWindowController(processServices{scriptRuntime: a.scriptRT, runtimeScopes: &a.runtimeScopes}, glfwEventPump{})
	if err := a.controller.startLoop(); err != nil {
		a.controller = nil
		return false, err
	}
	a.initMux()
	a.syncProcessServices()
	factory := &glfwRestoreProjectionFactory{owner: a, windows: restoreBlueprintWindows(blueprint)}
	if err := a.controller.restoreStartupProjectionsBeforeMux(blueprint, factory, a.commitRestoredStartupConfiguration); err != nil {
		a.resetFailedRestoreFrontend(freshConfig)
		return errors.Is(err, errRestoreBeforeMuxHook), err
	}
	defer a.closeInitialWindowController()
	a.needsRedraw = true
	return true, a.runLoop(a.window)
}

func (a *App) resetFailedRestoreFrontend(freshConfig config.Config) {
	if a.controller != nil {
		a.closeInitialWindowController()
	}
	a.shutdownProcessServices()
	a.controller = nil
	a.mux = nil
	a.window = nil
	a.windowID = 0
	a.cfg = freshConfig
	a.restoreAppearance = nil
	a.projectionBaseConfig = nil
}
