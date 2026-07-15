//go:build glfw

package glfwgl

import (
	"github.com/go-gl/glfw/v3.3/glfw"
)

// Interactive scrollback search (Slice 2). The hotkey is a fixed ctrl+shift+f
// chord in v1 (configurable later). While the bar is open the key and char
// callbacks route here and the PTY sees nothing (trap 1); closing restores the
// live input flow exactly by clearing a.search.active.

// searchState holds the modal scrollback search state. All fields are
// main-thread only. While active is true, key and char callbacks route to the
// search bar and nothing reaches the PTY (app_search.go). Match position is
// stored in the global (physical-row) index space; draw() converts it to a
// viewport row.
type searchState struct {
	active   bool
	query    []rune
	hasMatch bool
	matchRow int // global row (scrollback+live index space)
	matchCol int // start cell column of the match
	matchLen int // match length in runes (highlight cell span, v1)
	viewRow  int // frame-local: match's viewport row, or -1 when off-screen
}

// handleSearchKey processes the search hotkey and, while the bar is open, all
// keyboard input. It returns true when it consumed the key so the caller stops
// before script keys, the stats toggle, clipboard, and PTY encoding.
func (a *App) handleSearchKey(key glfw.Key, mods glfw.ModifierKey) bool {
	isChord := key == glfw.KeyF && mods&glfw.ModControl != 0 && mods&glfw.ModShift != 0
	if !a.search.active {
		if isChord {
			a.openSearch()
			return true
		}
		return false
	}
	// Bar open: consume every key so nothing (incl. ctrl+c) reaches the PTY.
	switch key {
	case glfw.KeyEscape:
		a.closeSearch()
	case glfw.KeyEnter, glfw.KeyKPEnter:
		a.searchNext()
	case glfw.KeyBackspace:
		a.searchBackspace()
	}
	return true
}

func (a *App) openSearch() {
	a.search.active = true
	a.search.query = a.search.query[:0]
	a.search.hasMatch = false
	a.requestRedraw()
}

// closeSearch returns to the live view input flow. It leaves the viewport where
// the last match scrolled it; the user scrolls back to the bottom as usual.
func (a *App) closeSearch() {
	a.search.active = false
	a.search.hasMatch = false
	a.requestRedraw()
}

// searchAppendRune adds a printable rune to the query. Editing is rune-based, so
// multibyte input is never split (trap 4). Control runes are ignored.
func (a *App) searchAppendRune(r rune) {
	if r < 0x20 || r == 0x7f {
		return
	}
	a.search.query = append(a.search.query, r)
	a.requestRedraw()
}

func (a *App) searchBackspace() {
	if len(a.search.query) > 0 {
		a.search.query = a.search.query[:len(a.search.query)-1]
	}
	a.requestRedraw()
}

// searchNext jumps to the next match upward. The first jump searches from the
// bottom of the live screen; subsequent jumps search strictly above the current
// match (trap: from-row convention matches core.SearchBackward). An empty query
// is a no-op (trap 5).
func (a *App) searchNext() {
	if len(a.search.query) == 0 {
		a.search.hasMatch = false
		a.requestRedraw()
		return
	}
	a.mu.Lock()
	from := a.term.ScrollbackLines() + a.term.Rows()
	if a.search.hasMatch {
		from = a.search.matchRow
	}
	row, col, ok := a.term.SearchBackward(string(a.search.query), from)
	if ok {
		a.search.matchRow, a.search.matchCol = row, col
		a.search.matchLen = len(a.search.query)
		a.search.hasMatch = true
		a.scrollGlobalRowIntoView(row)
	} else {
		a.search.hasMatch = false
	}
	a.mu.Unlock()
	a.requestRedraw()
}

// scrollGlobalRowIntoView adjusts the display offset so the given global
// (physical-row) index is visible, centering it when a jump is needed. Must be
// called with a.mu held. Uses the same global-row/DisplayOffset convention as
// core.Resize and CopyView (trap 2): the viewport shows global rows
// [scrollbackRows-displayOffset, +rows-1].
func (a *App) scrollGlobalRowIntoView(g int) {
	if _, ok := a.term.GlobalRowToViewport(g); ok {
		return // already visible
	}
	scrollbackRows := a.term.ScrollbackLines()
	curOffset := a.term.DisplayOffset()
	targetTop := g - a.term.Rows()/2
	if targetTop < 0 {
		targetTop = 0
	}
	// ScrollViewport moves relative to the current offset and clamps to
	// [0, scrollbackRows]; a positive delta scrolls back into history.
	a.term.ScrollViewport((scrollbackRows - targetTop) - curOffset)
}
