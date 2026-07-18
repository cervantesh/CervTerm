package mux

import (
	"errors"
	"fmt"
)

var (
	ErrPaneNotFound        = errors.New("mux: pane not found")
	ErrSplitNotFound       = errors.New("mux: split not found")
	ErrPaneNotRunning      = errors.New("mux: pane is not running")
	ErrAlreadyBootstrapped = errors.New("mux: already bootstrapped")
	ErrEmptyModel          = errors.New("mux: model is empty")
	ErrInvalidAxis         = errors.New("mux: invalid split axis")
	ErrInvalidRatio        = errors.New("mux: invalid split ratio")
	ErrInvalidDirection    = errors.New("mux: invalid focus direction")
	ErrInvalidGeometry     = errors.New("mux: invalid geometry")
	ErrSplitTooSmall       = errors.New("mux: split would create a pane below the minimum size")
	ErrNoPaneInDirection   = errors.New("mux: no pane in focus direction")
	ErrInvalidResizeDelta  = errors.New("mux: resize delta must be positive")
	ErrTopologyTooSmall    = errors.New("mux: topology mutation would create a pane below 2x2 cells")
	ErrIDExhausted         = errors.New("mux: identifier space exhausted")
	ErrInvariant           = errors.New("mux: invariant violation")
)

func invariantError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvariant, fmt.Sprintf(format, args...))
}
