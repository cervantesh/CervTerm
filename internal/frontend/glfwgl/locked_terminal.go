package glfwgl

import (
	"sync"

	"cervterm/internal/core"
)

// searchTerminal is the narrow port the search controller needs from the
// terminal: one atomic "find the next match and reveal it" operation. The
// locking is the adapter's concern, so the controller never handles a mutex.
type searchTerminal interface {
	// SearchUpward finds the next match for query at or above the current match
	// (prevRow when hasPrev is true) or from the live bottom otherwise, scrolls it
	// into view on a hit, and returns the match — all under the terminal lock.
	SearchUpward(query string, hasPrev bool, prevRow int) (row, col int, ok bool)
}

// lockedTerminal adapts a *core.Terminal plus the mutex guarding it against the
// PTY reader goroutine into serialized, coarse-grained operations. It is the one
// place that knows the terminal must be locked; callers depend on the port, not
// on the mutex. The mutex is shared with App (it is &App.mu), so operations here
// are mutually exclusive with the parser advance and every other App term access.
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
