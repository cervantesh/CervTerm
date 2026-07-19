package mux

import (
	"errors"
	"fmt"
)

var (
	ErrPaneNotFound          = errors.New("mux: pane not found")
	ErrSplitNotFound         = errors.New("mux: split not found")
	ErrPaneNotRunning        = errors.New("mux: pane is not running")
	ErrAlreadyBootstrapped   = errors.New("mux: already bootstrapped")
	ErrEmptyModel            = errors.New("mux: model is empty")
	ErrInvalidAxis           = errors.New("mux: invalid split axis")
	ErrInvalidRatio          = errors.New("mux: invalid split ratio")
	ErrInvalidDirection      = errors.New("mux: invalid focus direction")
	ErrInvalidGeometry       = errors.New("mux: invalid geometry")
	ErrSplitTooSmall         = errors.New("mux: split would create a pane below the minimum size")
	ErrNoPaneInDirection     = errors.New("mux: no pane in focus direction")
	ErrInvalidResizeDelta    = errors.New("mux: resize delta must be positive")
	ErrTopologyTooSmall      = errors.New("mux: topology mutation would create a pane below 2x2 cells")
	ErrIDExhausted           = errors.New("mux: identifier space exhausted")
	ErrWindowNotFound        = errors.New("mux: window not found")
	ErrWindowLimitReached    = errors.New("mux: window limit reached")
	ErrWorkspaceNotFound     = errors.New("mux: workspace not found")
	ErrWorkspaceLimitReached = errors.New("mux: workspace limit reached")
	ErrInvalidWorkspaceName  = errors.New("mux: workspace name must be non-empty after trimming")
	ErrWorkspaceNameTooLong  = errors.New("mux: workspace name is too long")
	ErrWorkspaceNameExists   = errors.New("mux: workspace name already exists")
	ErrTabNotFound           = errors.New("mux: tab not found")
	ErrTabLimitReached       = errors.New("mux: tab limit reached")
	ErrInvalidTabPosition    = errors.New("mux: invalid tab position")
	ErrSameTabTransfer       = errors.New("mux: pane transfer requires different tabs")
	ErrSameWindowTransfer    = errors.New("mux: tab transfer requires different windows")
	ErrInvariant             = errors.New("mux: invariant violation")
	ErrShuttingDown          = errors.New("mux: shutting down")
)

func invariantError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvariant, fmt.Sprintf(format, args...))
}
