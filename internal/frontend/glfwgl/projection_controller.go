//go:build glfw

package glfwgl

import (
	"cervterm/internal/core"
	termmux "cervterm/internal/mux"
)

// projectionController is the only process-routing surface retained by an App.
// It exposes explicit projection operations and never returns a controller,
// process service, Owner, Mux, or projection capability.
type projectionController interface {
	dispatch([]termmux.Event) bool
	withCurrent(termmux.WindowID, func()) error
	markDamage(termmux.WindowIdentity) error
	processReady() bool
	recordRuntimeFocus(termmux.WindowID) error
	projectionAvailable(termmux.WindowID) bool
	transferProjectionGeometry(termmux.WindowIdentity, termmux.WindowID, termmux.WindowID) (termmux.PixelRect, termmux.PixelRect, termmux.CellMetricsResolver, error)
	cancelProjectionTabComposition(termmux.WindowID, termmux.TabID)
	cancelProjectionPaneComposition(termmux.WindowID, termmux.PaneID)
	processWindows(termmux.WindowIdentity) ([]termmux.WindowView, error)
	processWorkspaces(termmux.WindowIdentity) ([]termmux.WorkspaceView, error)
	accessibilityWindow(termmux.WindowIdentity) (termmux.WindowView, termmux.WorkspaceView, error)
	windowForPane(termmux.WindowIdentity, termmux.PaneID) (termmux.WindowID, bool)
	transferTabBetweenWindows(termmux.WindowIdentity, termmux.TabTransferRequest) ([]termmux.Event, error)
	transferPaneBetweenWindows(termmux.WindowIdentity, termmux.PaneTransferRequest) ([]termmux.Event, error)
	createWorkspace(termmux.WindowIdentity, string) (termmux.WorkspaceView, []termmux.Event, error)
	renameWorkspace(termmux.WindowIdentity, termmux.WorkspaceID, string) ([]termmux.Event, error)
	switchWorkspace(termmux.WindowIdentity, termmux.WorkspaceID) ([]termmux.Event, error)
	moveWindowToWorkspace(termmux.WindowIdentity, termmux.WindowID, termmux.WorkspaceID) ([]termmux.Event, error)
	updateProcessConfig(termmux.WindowIdentity, core.PaletteBase, int, bool) error
	createRuntimeProjection(termmux.WindowIdentity) (termmux.WindowID, error)
	closeRuntimeProjection(termmux.WindowIdentity, termmux.WindowID) (termmux.CloseWindowResult, error)
	activateRuntimeProjection(termmux.WindowIdentity, termmux.WindowID) error
}

// projectionMessageRouter is a field-opaque message port. Function values are
// not navigable service locators, and every operation remains typed and explicit.
type projectionMessageRouter struct {
	dispatchFn                        func([]termmux.Event) bool
	withCurrentFn                     func(termmux.WindowID, func()) error
	markDamageFn                      func(termmux.WindowIdentity) error
	processReadyFn                    func() bool
	recordRuntimeFocusFn              func(termmux.WindowID) error
	projectionAvailableFn             func(termmux.WindowID) bool
	transferProjectionGeometryFn      func(termmux.WindowIdentity, termmux.WindowID, termmux.WindowID) (termmux.PixelRect, termmux.PixelRect, termmux.CellMetricsResolver, error)
	cancelProjectionTabCompositionFn  func(termmux.WindowID, termmux.TabID)
	cancelProjectionPaneCompositionFn func(termmux.WindowID, termmux.PaneID)
	processWindowsFn                  func(termmux.WindowIdentity) ([]termmux.WindowView, error)
	processWorkspacesFn               func(termmux.WindowIdentity) ([]termmux.WorkspaceView, error)
	accessibilityWindowFn             func(termmux.WindowIdentity) (termmux.WindowView, termmux.WorkspaceView, error)
	windowForPaneFn                   func(termmux.WindowIdentity, termmux.PaneID) (termmux.WindowID, bool)
	transferTabBetweenWindowsFn       func(termmux.WindowIdentity, termmux.TabTransferRequest) ([]termmux.Event, error)
	transferPaneBetweenWindowsFn      func(termmux.WindowIdentity, termmux.PaneTransferRequest) ([]termmux.Event, error)
	createWorkspaceFn                 func(termmux.WindowIdentity, string) (termmux.WorkspaceView, []termmux.Event, error)
	renameWorkspaceFn                 func(termmux.WindowIdentity, termmux.WorkspaceID, string) ([]termmux.Event, error)
	switchWorkspaceFn                 func(termmux.WindowIdentity, termmux.WorkspaceID) ([]termmux.Event, error)
	moveWindowToWorkspaceFn           func(termmux.WindowIdentity, termmux.WindowID, termmux.WorkspaceID) ([]termmux.Event, error)
	updateProcessConfigFn             func(termmux.WindowIdentity, core.PaletteBase, int, bool) error
	createRuntimeProjectionFn         func(termmux.WindowIdentity) (termmux.WindowID, error)
	closeRuntimeProjectionFn          func(termmux.WindowIdentity, termmux.WindowID) (termmux.CloseWindowResult, error)
	activateRuntimeProjectionFn       func(termmux.WindowIdentity, termmux.WindowID) error
}

func newProjectionMessageRouter(c *windowController) projectionController {
	if c == nil {
		return nil
	}
	return &projectionMessageRouter{
		dispatchFn: c.dispatch, withCurrentFn: c.withCurrent, markDamageFn: c.markDamageFrom, processReadyFn: c.processReady,
		recordRuntimeFocusFn: c.recordRuntimeFocus, projectionAvailableFn: c.projectionAvailable,
		transferProjectionGeometryFn:      c.transferProjectionGeometry,
		cancelProjectionTabCompositionFn:  c.cancelProjectionTabComposition,
		cancelProjectionPaneCompositionFn: c.cancelProjectionPaneComposition,
		processWindowsFn:                  c.processWindows, processWorkspacesFn: c.processWorkspaces,
		accessibilityWindowFn: c.accessibilityWindow, windowForPaneFn: c.windowForPane,
		transferTabBetweenWindowsFn:  c.transferTabBetweenWindows,
		transferPaneBetweenWindowsFn: c.transferPaneBetweenWindows,
		createWorkspaceFn:            c.createWorkspace, renameWorkspaceFn: c.renameWorkspace,
		switchWorkspaceFn: c.switchWorkspace, moveWindowToWorkspaceFn: c.moveWindowToWorkspace,
		updateProcessConfigFn: c.updateProcessConfig, createRuntimeProjectionFn: c.createRuntimeProjectionFrom,
		closeRuntimeProjectionFn: c.closeRuntimeProjectionFrom, activateRuntimeProjectionFn: c.activateRuntimeProjectionFrom,
	}
}

func (r *projectionMessageRouter) dispatch(v []termmux.Event) bool { return r.dispatchFn(v) }
func (r *projectionMessageRouter) withCurrent(id termmux.WindowID, fn func()) error {
	return r.withCurrentFn(id, fn)
}
func (r *projectionMessageRouter) markDamage(origin termmux.WindowIdentity) error {
	return r.markDamageFn(origin)
}
func (r *projectionMessageRouter) processReady() bool { return r.processReadyFn() }
func (r *projectionMessageRouter) recordRuntimeFocus(id termmux.WindowID) error {
	return r.recordRuntimeFocusFn(id)
}
func (r *projectionMessageRouter) projectionAvailable(id termmux.WindowID) bool {
	return r.projectionAvailableFn(id)
}
func (r *projectionMessageRouter) transferProjectionGeometry(origin termmux.WindowIdentity, source, destination termmux.WindowID) (termmux.PixelRect, termmux.PixelRect, termmux.CellMetricsResolver, error) {
	return r.transferProjectionGeometryFn(origin, source, destination)
}
func (r *projectionMessageRouter) cancelProjectionTabComposition(id termmux.WindowID, tab termmux.TabID) {
	r.cancelProjectionTabCompositionFn(id, tab)
}
func (r *projectionMessageRouter) cancelProjectionPaneComposition(id termmux.WindowID, pane termmux.PaneID) {
	r.cancelProjectionPaneCompositionFn(id, pane)
}
func (r *projectionMessageRouter) processWindows(origin termmux.WindowIdentity) ([]termmux.WindowView, error) {
	return r.processWindowsFn(origin)
}
func (r *projectionMessageRouter) processWorkspaces(origin termmux.WindowIdentity) ([]termmux.WorkspaceView, error) {
	return r.processWorkspacesFn(origin)
}
func (r *projectionMessageRouter) accessibilityWindow(origin termmux.WindowIdentity) (termmux.WindowView, termmux.WorkspaceView, error) {
	return r.accessibilityWindowFn(origin)
}
func (r *projectionMessageRouter) windowForPane(origin termmux.WindowIdentity, pane termmux.PaneID) (termmux.WindowID, bool) {
	return r.windowForPaneFn(origin, pane)
}
func (r *projectionMessageRouter) transferTabBetweenWindows(origin termmux.WindowIdentity, req termmux.TabTransferRequest) ([]termmux.Event, error) {
	return r.transferTabBetweenWindowsFn(origin, req)
}
func (r *projectionMessageRouter) transferPaneBetweenWindows(origin termmux.WindowIdentity, req termmux.PaneTransferRequest) ([]termmux.Event, error) {
	return r.transferPaneBetweenWindowsFn(origin, req)
}
func (r *projectionMessageRouter) createWorkspace(origin termmux.WindowIdentity, name string) (termmux.WorkspaceView, []termmux.Event, error) {
	return r.createWorkspaceFn(origin, name)
}
func (r *projectionMessageRouter) renameWorkspace(origin termmux.WindowIdentity, id termmux.WorkspaceID, name string) ([]termmux.Event, error) {
	return r.renameWorkspaceFn(origin, id, name)
}
func (r *projectionMessageRouter) switchWorkspace(origin termmux.WindowIdentity, id termmux.WorkspaceID) ([]termmux.Event, error) {
	return r.switchWorkspaceFn(origin, id)
}
func (r *projectionMessageRouter) moveWindowToWorkspace(origin termmux.WindowIdentity, window termmux.WindowID, workspace termmux.WorkspaceID) ([]termmux.Event, error) {
	return r.moveWindowToWorkspaceFn(origin, window, workspace)
}
func (r *projectionMessageRouter) updateProcessConfig(origin termmux.WindowIdentity, palette core.PaletteBase, scrollback int, hide bool) error {
	return r.updateProcessConfigFn(origin, palette, scrollback, hide)
}
func (r *projectionMessageRouter) createRuntimeProjection(origin termmux.WindowIdentity) (termmux.WindowID, error) {
	return r.createRuntimeProjectionFn(origin)
}
func (r *projectionMessageRouter) closeRuntimeProjection(origin termmux.WindowIdentity, id termmux.WindowID) (termmux.CloseWindowResult, error) {
	return r.closeRuntimeProjectionFn(origin, id)
}
func (r *projectionMessageRouter) activateRuntimeProjection(origin termmux.WindowIdentity, id termmux.WindowID) error {
	return r.activateRuntimeProjectionFn(origin, id)
}

var _ projectionController = (*projectionMessageRouter)(nil)
