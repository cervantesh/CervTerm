//go:build glfw

package glfwgl

import (
	"sync"

	"cervterm/internal/core"
)

// lockedTerminal is retained only as a test adapter for searchController's
// legacy core.Terminal fixtures; production search uses muxSearchTerminal.
type lockedTerminal struct {
	term *core.Terminal
	mu   *sync.Mutex
}

func newLockedTerminal(term *core.Terminal, mu *sync.Mutex) *lockedTerminal {
	return &lockedTerminal{term: term, mu: mu}
}

// SearchUpward implements searchTerminal. Computing the from-row, searching, and
// scrolling the hit into view happen in one critical section so the terminal
// cannot change between the steps.
func (l *lockedTerminal) SearchUpward(query string, hasPrev bool, prevRow int) (row, col int, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	from := l.term.ScrollbackLines() + l.term.Rows()
	if hasPrev {
		from = prevRow
	}
	row, col, ok = l.term.SearchBackward(query, from)
	if ok {
		scrollGlobalRowIntoView(l.term, row)
	}
	return row, col, ok
}

// scrollGlobalRowIntoView adjusts the display offset so the given global
// (physical-row) index is visible, centering it when a jump is needed. It must
// be called with the terminal lock held (SearchUpward does). Uses the same
// global-row/DisplayOffset convention as core.Resize and CopyView (trap 2): the
// viewport shows global rows [scrollbackRows-displayOffset, +rows-1].
func scrollGlobalRowIntoView(t *core.Terminal, g int) {
	if _, ok := t.GlobalRowToViewport(g); ok {
		return // already visible
	}
	scrollbackRows := t.ScrollbackLines()
	curOffset := t.DisplayOffset()
	targetTop := g - t.Rows()/2
	if targetTop < 0 {
		targetTop = 0
	}
	// ScrollViewport moves relative to the current offset and clamps to
	// [0, scrollbackRows]; a positive delta scrolls back into history.
	t.ScrollViewport((scrollbackRows - targetTop) - curOffset)
}
