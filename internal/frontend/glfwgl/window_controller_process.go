//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"

	termaction "cervterm/internal/action"
	"cervterm/internal/core"
	"cervterm/internal/ime"
	termmux "cervterm/internal/mux"
	"cervterm/internal/ownerthread"
)

type detachedWindowAttestation struct {
	id             termmux.WindowID
	host           nativeWindowHost
	threadSource   ownerthread.Source
	loopThread     ownerthread.Attestation
	loopEpoch      uint64
	lease          *windowLoopLease
	contextCurrent nativeContextCurrent
}

func (a detachedWindowAttestation) attest(identity termmux.WindowIdentity) bool {
	return identity.ID == a.id && identity.Incarnation != 0 && a.lease != nil &&
		a.loopEpoch != 0 && a.lease.epoch.Load() == a.loopEpoch &&
		a.loopThread.Current(a.threadSource) && a.contextCurrent != nil &&
		a.contextCurrent(a.host)
}

// acquireWindowCapability is the sole projection-capability minting path. The
// returned attestor closes over only detached native identity values and the
// loop lease; it cannot lead back to App, windowController, Owner, or Mux.
func (c *windowController) acquireWindowCapability(id termmux.WindowID, app *App, host nativeWindowHost) (windowMuxCapability, termmux.WindowIdentity, error) {
	if err := c.requireLoop(); err != nil {
		return nil, termmux.WindowIdentity{}, err
	}
	if id == 0 || app == nil || host == nil || app.controller == nil || app.windowID != id || c.services.windowCapabilities == nil {
		return nil, termmux.WindowIdentity{}, errWindowProjectionMissing
	}
	if projection := c.windows[id]; projection != nil && (projection.closed || projection.app != app || projection.host != host) {
		return nil, termmux.WindowIdentity{}, errWindowProjectionMissing
	}
	proof := detachedWindowAttestation{
		id: id, host: host, threadSource: c.threadSource, loopThread: c.loopThread,
		loopEpoch: c.activeLoopEpoch, lease: c.loopLease, contextCurrent: c.contextCurrent,
	}
	window, err := c.services.windowCapabilities.ForWindow(id, proof.attest)
	if err != nil {
		return nil, termmux.WindowIdentity{}, err
	}
	identity := window.Identity()
	if window.WindowID() != id || !proof.attest(identity) {
		return nil, termmux.WindowIdentity{}, fmt.Errorf("bind projection: %w", termmux.ErrWrongOwnerThread)
	}
	if projection := c.windows[id]; projection != nil {
		projection.identity = identity
	}
	if c.boundOrigins == nil {
		c.boundOrigins = make(map[termmux.WindowIdentity]*App)
	}
	c.boundOrigins[identity] = app
	return window, identity, nil
}

func (c *windowController) requireOrigin(origin termmux.WindowIdentity) error {
	if err := c.requireLoop(); err != nil {
		return err
	}
	if origin.ID == 0 || origin.Incarnation == 0 {
		return termmux.ErrWrongOrigin
	}
	if projection := c.windows[origin.ID]; projection != nil && !projection.closed && projection.identity == origin && projection.app != nil && projection.app.windowIdentity == origin {
		return nil
	}
	if app := c.boundOrigins[origin]; app != nil && app.windowIdentity == origin {
		return nil
	}
	return termmux.ErrWrongOrigin
}

func (c *windowController) processWindows(origin termmux.WindowIdentity) ([]termmux.WindowView, error) {
	if err := c.requireOrigin(origin); err != nil || c.services.commands == nil {
		if err != nil {
			return nil, err
		}
		return nil, errWindowProjectionMissing
	}
	return c.services.commands.Windows(), nil
}

func (c *windowController) processWorkspaces(origin termmux.WindowIdentity) ([]termmux.WorkspaceView, error) {
	if err := c.requireOrigin(origin); err != nil || c.services.commands == nil {
		if err != nil {
			return nil, err
		}
		return nil, errWindowProjectionMissing
	}
	return c.services.commands.Workspaces(), nil
}

func (c *windowController) accessibilityWindow(origin termmux.WindowIdentity) (termmux.WindowView, termmux.WorkspaceView, error) {
	windows, err := c.processWindows(origin)
	if err != nil {
		return termmux.WindowView{}, termmux.WorkspaceView{}, err
	}
	for _, window := range windows {
		if window.ID == origin.ID {
			return window, c.services.commands.ActiveWorkspace(), nil
		}
	}
	return termmux.WindowView{}, termmux.WorkspaceView{}, errWindowProjectionMissing
}

func (c *windowController) windowForPane(origin termmux.WindowIdentity, pane termmux.PaneID) (termmux.WindowID, bool) {
	if c.requireOrigin(origin) != nil || c.services.commands == nil {
		return 0, false
	}
	return c.services.commands.WindowForPane(pane)
}

func (c *windowController) transferTabBetweenWindows(origin termmux.WindowIdentity, request termmux.TabTransferRequest) ([]termmux.Event, error) {
	if err := c.requireOrigin(origin); err != nil {
		return nil, err
	}
	if request.SourceWindow != origin.ID || c.services.commands == nil {
		return nil, termmux.ErrWrongOrigin
	}
	return c.services.commands.TransferTabBetweenWindows(request)
}

func (c *windowController) transferPaneBetweenWindows(origin termmux.WindowIdentity, request termmux.PaneTransferRequest) ([]termmux.Event, error) {
	if err := c.requireOrigin(origin); err != nil {
		return nil, err
	}
	if request.SourceWindow != origin.ID || c.services.commands == nil {
		return nil, termmux.ErrWrongOrigin
	}
	return c.services.commands.TransferPaneBetweenWindows(request)
}

func (c *windowController) createWorkspace(origin termmux.WindowIdentity, name string) (termmux.WorkspaceView, []termmux.Event, error) {
	if err := c.requireOrigin(origin); err != nil {
		return termmux.WorkspaceView{}, nil, err
	}
	return c.services.commands.CreateWorkspace(name)
}

func (c *windowController) renameWorkspace(origin termmux.WindowIdentity, id termmux.WorkspaceID, name string) ([]termmux.Event, error) {
	if err := c.requireOrigin(origin); err != nil {
		return nil, err
	}
	return c.services.commands.RenameWorkspace(id, name)
}

func (c *windowController) switchWorkspace(origin termmux.WindowIdentity, id termmux.WorkspaceID) ([]termmux.Event, error) {
	if err := c.requireOrigin(origin); err != nil {
		return nil, err
	}
	return c.services.commands.SwitchWorkspace(id)
}

func (c *windowController) moveWindowToWorkspace(origin termmux.WindowIdentity, window termmux.WindowID, workspace termmux.WorkspaceID) ([]termmux.Event, error) {
	if err := c.requireOrigin(origin); err != nil {
		return nil, err
	}
	return c.services.commands.MoveWindowToWorkspace(window, workspace)
}

func (c *windowController) updateProcessConfig(origin termmux.WindowIdentity, palette core.PaletteBase, scrollback int, hideCursor bool) error {
	if err := c.requireOrigin(origin); err != nil {
		return err
	}
	if c.services.commands == nil {
		return errWindowProjectionMissing
	}
	return errors.Join(
		c.services.commands.SetScrollbackCapacity(scrollback),
		c.services.commands.SetHideCursorWhenScrolled(hideCursor),
		c.services.commands.SetPaletteBase(palette),
	)
}

func (c *windowController) projectionAvailable(id termmux.WindowID) bool {
	if c.requireLoop() != nil {
		return false
	}
	projection := c.windows[id]
	return projection != nil && !projection.closed && projection.app != nil
}

func (c *windowController) transferProjectionGeometry(origin termmux.WindowIdentity, source, destination termmux.WindowID) (termmux.PixelRect, termmux.PixelRect, termmux.CellMetricsResolver, error) {
	if err := c.requireLoop(); err != nil {
		return termmux.PixelRect{}, termmux.PixelRect{}, nil, err
	}
	if err := c.requireOrigin(origin); err != nil {
		return termmux.PixelRect{}, termmux.PixelRect{}, nil, err
	}
	src, dst := c.projectionApp(source), c.projectionApp(destination)
	if src == nil || dst == nil {
		return termmux.PixelRect{}, termmux.PixelRect{}, nil, termaction.ErrTargetUnavailable
	}
	sw, sh := src.lastFBW, src.lastFBH
	dw, dh := dst.lastFBW, dst.lastFBH
	if (sw <= 0 || sh <= 0) && src.window != nil {
		sw, sh = src.window.GetFramebufferSize()
	}
	if (dw <= 0 || dh <= 0) && dst.window != nil {
		dw, dh = dst.window.GetFramebufferSize()
	}
	if sw <= 0 || sh <= 0 || dw <= 0 || dh <= 0 {
		return termmux.PixelRect{}, termmux.PixelRect{}, nil, termaction.ErrTargetUnavailable
	}
	resolve := func(id termmux.PaneID) (termmux.CellMetrics, bool) {
		windowID, ok := c.windowForPane(origin, id)
		if !ok {
			return termmux.CellMetrics{}, false
		}
		projection := c.projectionApp(windowID)
		if projection == nil {
			return termmux.CellMetrics{}, false
		}
		cellW, cellH := projection.cellW, projection.cellH
		if state := projection.paneUI[id]; state != nil && state.font.cellW > 0 && state.font.cellH > 0 {
			cellW, cellH = state.font.cellW, state.font.cellH
		}
		return termmux.CellMetrics{CellWidth: max(1, int(cellW)), CellHeight: max(1, int(cellH))}, true
	}
	return src.muxContentBounds(sw, sh), dst.muxContentBounds(dw, dh), resolve, nil
}

func (c *windowController) cancelProjectionTabComposition(id termmux.WindowID, tab termmux.TabID) {
	if c.requireLoop() != nil {
		return
	}
	if projection := c.projectionApp(id); projection != nil && projection.compositionTargetsTab(tab) {
		_ = projection.cancelComposition(ime.CancelTargetChanged)
	}
}

func (c *windowController) cancelProjectionPaneComposition(id termmux.WindowID, pane termmux.PaneID) {
	if c.requireLoop() != nil {
		return
	}
	if projection := c.projectionApp(id); projection != nil && projection.compositionTargetsPane(pane) {
		_ = projection.cancelComposition(ime.CancelTargetChanged)
	}
}

func (c *windowController) shutdownServices(primary *App) error {
	if c == nil || primary == nil || c.primary != primary {
		return termmux.ErrWrongOwner
	}
	if len(c.windows) != 0 || c.inLoop {
		return errWindowLoopInactive
	}
	if c.services.commands == nil {
		return nil
	}
	err := c.services.commands.Shutdown()
	if c.services.commands.Closed() {
		c.services.commands = nil
		c.services.windowCapabilities = nil
		c.runtimeWindows = nil
		c.restoreWindows = nil
		c.boundOrigins = nil
	}
	return err
}

func (c *windowController) processReady() bool {
	return c != nil && c.services.commands != nil && !c.services.commands.Closed()
}

func (c *windowController) processClosed() bool {
	return c == nil || c.services.commands == nil || c.services.commands.Closed()
}
