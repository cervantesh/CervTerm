//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"

	"cervterm/internal/layoutrestore"
	termmux "cervterm/internal/mux"
)

var errRestoreProjectionTransaction = errors.New("invalid restore projection transaction")
var errRestoreBeforeMuxHook = errors.New("restore before-mux hook failed")

// nativeRestoreProjectionFactory prepares one hidden native projection in
// persisted workspace/window traversal order.
type nativeRestoreProjectionFactory interface {
	PrepareRestore(index int) (*nativeProjectionBundle, termmux.RestoreWindowGeometry, error)
}

type restoreWindowLifecycle interface {
	PrepareRestore(layoutrestore.Blueprint, []termmux.RestoreWindowGeometry) (*termmux.RestoreCandidate, error)
	RestoreWindowIDs(*termmux.RestoreCandidate) ([]termmux.WindowID, error)
	CommitRestore(*termmux.RestoreCandidate) ([]termmux.Event, error)
	AbortRestore(*termmux.RestoreCandidate) error
}

func (c *windowController) setRestoreWindows(windows restoreWindowLifecycle) {
	c.restoreWindows = windows
}

func (c *windowController) restoreStartupProjections(blueprint layoutrestore.Blueprint, factory nativeRestoreProjectionFactory) error {
	return c.restoreStartupProjectionsBeforeMux(blueprint, factory, nil)
}

func (c *windowController) restoreStartupProjectionsBeforeMux(blueprint layoutrestore.Blueprint, factory nativeRestoreProjectionFactory, beforeMux func() error) error {
	if c.restoreWindows == nil {
		return errRestoreProjectionTransaction
	}
	snapshot := blueprint.Snapshot()
	count := 0
	for _, workspace := range snapshot.Workspaces {
		count += len(workspace.Windows)
	}
	projection, err := c.prepareRestoreProjections(factory, count)
	if err != nil {
		return err
	}
	if beforeMux != nil {
		if err := beforeMux(); err != nil {
			return errors.Join(errRestoreBeforeMuxHook, err, c.abortRestoreProjections(projection, nil))
		}
	}
	geometries, err := c.restoreGeometries(projection)
	if err != nil {
		return errors.Join(err, c.abortRestoreProjections(projection, nil))
	}
	restore, err := c.restoreWindows.PrepareRestore(blueprint, geometries)
	if err != nil {
		return errors.Join(err, c.abortRestoreProjections(projection, nil))
	}
	ids, err := c.restoreWindows.RestoreWindowIDs(restore)
	if err != nil {
		return errors.Join(err, c.restoreWindows.AbortRestore(restore), c.abortRestoreProjections(projection, nil))
	}
	if err := c.publishRestoreProjections(projection, ids); err != nil {
		return errors.Join(err, c.restoreWindows.AbortRestore(restore))
	}
	if !c.validRestoreProjectionCandidate(projection, true) {
		return errors.Join(errRestoreProjectionTransaction, c.restoreWindows.AbortRestore(restore), c.abortRestoreProjections(projection, nil))
	}
	events, err := c.restoreWindows.CommitRestore(restore)
	if err != nil {
		return errors.Join(err, c.restoreWindows.AbortRestore(restore), c.abortRestoreProjections(projection, nil))
	}
	return c.activateRestoreProjections(projection, events)
}

type restoreProjectionCandidate struct {
	owner      *windowController
	bundles    []*nativeProjectionBundle
	geometries []termmux.RestoreWindowGeometry
	ids        []termmux.WindowID
	bound      []int
	published  bool
	committed  bool
	aborted    bool
}

func (c *windowController) syncPendingRestoreApps(owner *App) error {
	if owner == nil || c.restorePending == nil || !c.validRestoreProjectionCandidate(c.restorePending, false) {
		return errRestoreProjectionTransaction
	}
	for _, bundle := range c.restorePending.bundles {
		child := bundle.app
		if child == nil || child == owner {
			continue
		}
		child.scriptRT = owner.scriptRT
		child.scriptGeneration = owner.scriptGeneration
		child.initActionBindings()
	}
	return nil
}

func (c *windowController) prepareRestoreProjections(factory nativeRestoreProjectionFactory, count int) (*restoreProjectionCandidate, error) {
	if err := c.requireLoop(); err != nil {
		return nil, err
	}
	if factory == nil || count <= 0 || c.restorePending != nil || len(c.windows) != 0 || len(c.order) != 0 || c.active != 0 || c.current != 0 {
		return nil, errRestoreProjectionTransaction
	}
	candidate := &restoreProjectionCandidate{owner: c, bundles: make([]*nativeProjectionBundle, 0, count), geometries: make([]termmux.RestoreWindowGeometry, 0, count)}
	c.restorePending = candidate
	for index := 0; index < count; index++ {
		bundle, geometry, err := factory.PrepareRestore(index)
		if err != nil {
			return nil, errors.Join(err, c.abortRestoreProjections(candidate, bundle))
		}
		if bundle == nil || bundle.host == nil || bundle.handle == nil || geometry.Content.Empty() || geometry.Metrics.CellWidth <= 0 || geometry.Metrics.CellHeight <= 0 {
			return nil, errors.Join(errRestoreProjectionTransaction, c.abortRestoreProjections(candidate, bundle))
		}
		bundle.host.Hide()
		candidate.bundles = append(candidate.bundles, bundle)
		candidate.geometries = append(candidate.geometries, geometry)
	}
	return candidate, nil
}

func (c *windowController) restoreGeometries(candidate *restoreProjectionCandidate) ([]termmux.RestoreWindowGeometry, error) {
	if !c.validRestoreProjectionCandidate(candidate, false) {
		return nil, errRestoreProjectionTransaction
	}
	return append([]termmux.RestoreWindowGeometry(nil), candidate.geometries...), nil
}

// publishRestoreProjections binds stable mux identities and atomically makes the
// hidden projections addressable. Abort remains valid until mux restore commits.
func (c *windowController) publishRestoreProjections(candidate *restoreProjectionCandidate, ids []termmux.WindowID) error {
	if !c.validRestoreProjectionCandidate(candidate, false) {
		return errRestoreProjectionTransaction
	}
	if len(ids) != len(candidate.bundles) {
		return errors.Join(errRestoreProjectionTransaction, c.abortRestoreProjections(candidate, nil))
	}
	seen := make(map[termmux.WindowID]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			return errors.Join(errRestoreProjectionTransaction, c.abortRestoreProjections(candidate, nil))
		}
		if _, duplicate := seen[id]; duplicate {
			return errors.Join(errWindowProjectionExists, c.abortRestoreProjections(candidate, nil))
		}
		if _, exists := c.windows[id]; exists {
			return errors.Join(errWindowProjectionExists, c.abortRestoreProjections(candidate, nil))
		}
		seen[id] = struct{}{}
	}
	for index, bundle := range candidate.bundles {
		if bundle.bind == nil {
			continue
		}
		if bundle.unbind == nil {
			return errors.Join(errRestoreProjectionTransaction, c.abortRestoreProjections(candidate, nil))
		}
		candidate.bound = append(candidate.bound, index)
		if err := bundle.bind(ids[index]); err != nil {
			return errors.Join(fmt.Errorf("bind restore projection %d: %w", index, err), c.abortRestoreProjections(candidate, nil))
		}
	}
	candidate.ids = append([]termmux.WindowID(nil), ids...)
	for index, id := range candidate.ids {
		bundle := candidate.bundles[index]
		c.windows[id] = &windowProjection{id: id, host: bundle.host, app: bundle.app, handle: bundle.handle, bundle: bundle, dirty: true, visible: false}
	}
	c.order = append(c.order, candidate.ids...)
	candidate.published = true
	return nil
}

// activateRestoreProjections finalizes native ownership after mux commit. The
// commit events select workspace visibility/focus before any restored host is shown.
func (c *windowController) activateRestoreProjections(candidate *restoreProjectionCandidate, events []termmux.Event) error {
	if !c.validRestoreProjectionCandidate(candidate, true) {
		return errRestoreProjectionTransaction
	}
	candidate.committed = true
	c.restorePending = nil
	c.dispatch(events)
	return nil
}

func (c *windowController) abortRestoreProjections(candidate *restoreProjectionCandidate, partial *nativeProjectionBundle) error {
	if candidate == nil || candidate.owner != c || candidate.committed || candidate.aborted || c.restorePending != candidate {
		if partial != nil {
			return errors.Join(errRestoreProjectionTransaction, closeRestoreProjectionBundle(partial))
		}
		return errRestoreProjectionTransaction
	}
	candidate.aborted = true
	c.restorePending = nil
	if candidate.published {
		for _, id := range candidate.ids {
			delete(c.windows, id)
			delete(c.pending, id)
			if c.active == id {
				c.active = 0
			}
			if c.current == id {
				c.current = 0
			}
		}
		c.order = c.order[:0]
	}
	var joined error
	for index := len(candidate.bound) - 1; index >= 0; index-- {
		bundle := candidate.bundles[candidate.bound[index]]
		joined = errors.Join(joined, bundle.unbind())
		bundle.unbind = nil
	}
	candidate.bound = nil
	if partial != nil {
		joined = errors.Join(joined, closeRestoreProjectionBundle(partial))
	}
	for index := len(candidate.bundles) - 1; index >= 0; index-- {
		joined = errors.Join(joined, closeRestoreProjectionBundle(candidate.bundles[index]))
	}
	return joined
}

func (c *windowController) validRestoreProjectionCandidate(candidate *restoreProjectionCandidate, requirePublished bool) bool {
	return candidate != nil && candidate.owner == c && c.restorePending == candidate && !candidate.aborted && !candidate.committed && candidate.published == requirePublished
}

func closeRestoreProjectionBundle(bundle *nativeProjectionBundle) error {
	if bundle == nil {
		return nil
	}
	if bundle.host != nil {
		bundle.host.MakeContextCurrent()
	}
	return bundle.close()
}
