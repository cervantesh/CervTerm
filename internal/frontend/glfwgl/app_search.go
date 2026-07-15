//go:build glfw

package glfwgl

import (
	"github.com/go-gl/glfw/v3.3/glfw"
)

// Interactive scrollback search (Slice 2). The hotkey is a fixed ctrl+shift+f
// chord in v1 (configurable later). While the bar is open the key and char
// callbacks route here and the PTY sees nothing (trap 1); closing restores the
// live input flow exactly by clearing a.searching.

// handleSearchKey processes the search hotkey and, while the bar is open, all
// keyboard input. It returns true when it consumed the key so the caller stops
// before script keys, the stats toggle, clipboard, and PTY encoding.
func (a *App) handleSearchKey(key glfw.Key, mods glfw.ModifierKey) bool {
	isChord := key == glfw.KeyF && mods&glfw.ModControl != 0 && mods&glfw.ModShift != 0
	if !a.searching {
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
	a.searching = true
	a.searchQuery = a.searchQuery[:0]
	a.searchHasMatch = false
	a.requestRedraw()
}

// closeSearch returns to the live view input flow. It leaves the viewport where
// the last match scrolled it; the user scrolls back to the bottom as usual.
func (a *App) closeSearch() {
	a.searching = false
	a.searchHasMatch = false
	a.requestRedraw()
}

// searchAppendRune adds a printable rune to the query. Editing is rune-based, so
// multibyte input is never split (trap 4). Control runes are ignored.
func (a *App) searchAppendRune(r rune) {
	if r < 0x20 || r == 0x7f {
		return
	}
	a.searchQuery = append(a.searchQuery, r)
	a.requestRedraw()
}

func (a *App) searchBackspace() {
	if len(a.searchQuery) > 0 {
		a.searchQuery = a.searchQuery[:len(a.searchQuery)-1]
	}
	a.requestRedraw()
}

// searchNext jumps to the next match upward. The first jump searches from the
// bottom of the live screen; subsequent jumps search strictly above the current
// match (trap: from-row convention matches core.SearchBackward). An empty query
// is a no-op (trap 5).
func (a *App) searchNext() {
	if len(a.searchQuery) == 0 {
		a.searchHasMatch = false
		a.requestRedraw()
		return
	}
	a.mu.Lock()
	from := a.term.ScrollbackLines() + a.term.Rows()
	if a.searchHasMatch {
		from = a.searchMatchRow
	}
	row, col, ok := a.term.SearchBackward(string(a.searchQuery), from)
	if ok {
		a.searchMatchRow, a.searchMatchCol = row, col
		a.searchMatchLen = len(a.searchQuery)
		a.searchHasMatch = true
		a.scrollGlobalRowIntoView(row)
	} else {
		a.searchHasMatch = false
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
