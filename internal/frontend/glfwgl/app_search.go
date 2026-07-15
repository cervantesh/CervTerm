//go:build glfw

package glfwgl

import (
	"sync"

	"cervterm/internal/core"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// Interactive scrollback search (Slice 2). The hotkey is a fixed ctrl+shift+f
// chord in v1 (configurable later). While the bar is open the key and char
// callbacks route here and the PTY sees nothing (trap 1); closing restores the
// live input flow exactly by clearing search.active.

// searchController owns the modal scrollback search: its state plus the three
// App services it needs, injected as explicit dependencies (the terminal to
// query, the shared mutex that guards it against the PTY reader, and a redraw
// signal) rather than a back-pointer to the whole App. That keeps search logic
// in one place and testable in isolation with a real core.Terminal.
//
// All fields are main-thread only. While active is true, key and char callbacks
// route to the search bar and nothing reaches the PTY. Match position is stored
// in the global (physical-row) index space; draw() converts it to a viewport row.
type searchController struct {
	active   bool
	query    []rune
	hasMatch bool
	matchRow int // global row (scrollback+live index space)
	matchCol int // start cell column of the match
	matchLen int // match length in runes (highlight cell span, v1)
	viewRow  int // frame-local: match's viewport row, or -1 when off-screen

	term   *core.Terminal
	mu     *sync.Mutex
	redraw func()
}

// init wires the App services the controller depends on. Called once after the
// App is constructed, before the render loop starts.
func (s *searchController) init(term *core.Terminal, mu *sync.Mutex, redraw func()) {
	s.term = term
	s.mu = mu
	s.redraw = redraw
}

// handleKey processes the search hotkey and, while the bar is open, all keyboard
// input. It returns true when it consumed the key so the caller stops before
// script keys, the stats toggle, clipboard, and PTY encoding.
func (s *searchController) handleKey(key glfw.Key, mods glfw.ModifierKey) bool {
	isChord := key == glfw.KeyF && mods&glfw.ModControl != 0 && mods&glfw.ModShift != 0
	if !s.active {
		if isChord {
			s.open()
			return true
		}
		return false
	}
	// Bar open: consume every key so nothing (incl. ctrl+c) reaches the PTY.
	switch key {
	case glfw.KeyEscape:
		s.close()
	case glfw.KeyEnter, glfw.KeyKPEnter:
		s.next()
	case glfw.KeyBackspace:
		s.backspace()
	}
	return true
}

func (s *searchController) open() {
	s.active = true
	s.query = s.query[:0]
	s.hasMatch = false
	s.redraw()
}

// close returns to the live view input flow. It leaves the viewport where the
// last match scrolled it; the user scrolls back to the bottom as usual.
func (s *searchController) close() {
	s.active = false
	s.hasMatch = false
	s.redraw()
}

// appendRune adds a printable rune to the query. Editing is rune-based, so
// multibyte input is never split (trap 4). Control runes are ignored.
func (s *searchController) appendRune(r rune) {
	if r < 0x20 || r == 0x7f {
		return
	}
	s.query = append(s.query, r)
	s.redraw()
}

func (s *searchController) backspace() {
	if len(s.query) > 0 {
		s.query = s.query[:len(s.query)-1]
	}
	s.redraw()
}

// next jumps to the next match upward. The first jump searches from the bottom
// of the live screen; subsequent jumps search strictly above the current match
// (trap: from-row convention matches core.SearchBackward). An empty query is a
// no-op (trap 5).
func (s *searchController) next() {
	if len(s.query) == 0 {
		s.hasMatch = false
		s.redraw()
		return
	}
	s.mu.Lock()
	from := s.term.ScrollbackLines() + s.term.Rows()
	if s.hasMatch {
		from = s.matchRow
	}
	row, col, ok := s.term.SearchBackward(string(s.query), from)
	if ok {
		s.matchRow, s.matchCol = row, col
		s.matchLen = len(s.query)
		s.hasMatch = true
		s.scrollGlobalRowIntoView(row)
	} else {
		s.hasMatch = false
	}
	s.mu.Unlock()
	s.redraw()
}

// scrollGlobalRowIntoView adjusts the display offset so the given global
// (physical-row) index is visible, centering it when a jump is needed. Must be
// called with the terminal mutex held. Uses the same global-row/DisplayOffset
// convention as core.Resize and CopyView (trap 2): the viewport shows global
// rows [scrollbackRows-displayOffset, +rows-1].
func (s *searchController) scrollGlobalRowIntoView(g int) {
	if _, ok := s.term.GlobalRowToViewport(g); ok {
		return // already visible
	}
	scrollbackRows := s.term.ScrollbackLines()
	curOffset := s.term.DisplayOffset()
	targetTop := g - s.term.Rows()/2
	if targetTop < 0 {
		targetTop = 0
	}
	// ScrollViewport moves relative to the current offset and clamps to
	// [0, scrollbackRows]; a positive delta scrolls back into history.
	s.term.ScrollViewport((scrollbackRows - targetTop) - curOffset)
}
