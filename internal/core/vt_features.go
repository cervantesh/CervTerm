package core

func (t *Terminal) Index() {
	t.NewLine()
}

func (t *Terminal) NextLine() {
	t.CarriageReturn()
	t.Index()
}

func (t *Terminal) ReverseIndex() {
	t.wrapNext = false
	if t.cursorRow == t.scrollTop {
		t.scrollDownRegion(t.scrollTop, t.scrollBottom, 1)
		return
	}
	if t.cursorRow > 0 {
		t.cursorRow--
	}
}

func (t *Terminal) DesignateCharset(slot int, cs Charset) {
	if slot < 0 || slot >= len(t.charsets) {
		return
	}
	t.charsets[slot] = cs
}

func (t *Terminal) SelectCharset(slot int) {
	if slot < 0 || slot >= len(t.charsets) {
		return
	}
	t.activeCharset = slot
}

func (t *Terminal) translateCharset(r rune) rune {
	if t.charsets[t.activeCharset] != CharsetDECSpecial || r < '`' || r > '~' {
		return r
	}
	if r == '_' {
		return ' '
	}
	if mapped, ok := decSpecialGraphics[r]; ok {
		return mapped
	}
	return r
}

var decSpecialGraphics = map[rune]rune{
	'`': '◆', 'a': '▒', 'b': '␉', 'c': '␌', 'd': '␍', 'e': '␊',
	'f': '°', 'g': '±', 'h': '␤', 'i': '␋', 'j': '┘', 'k': '┐',
	'l': '┌', 'm': '└', 'n': '┼', 'o': '⎺', 'p': '⎻', 'q': '─',
	'r': '⎼', 's': '⎽', 't': '├', 'u': '┤', 'v': '┴', 'w': '┬',
	'x': '│', 'y': '≤', 'z': '≥', '{': 'π', '|': '≠', '}': '£',
	'~': '·',
}

func (t *Terminal) SetOriginMode(v bool) {
	t.originMode = v
	t.SetCursor(0, 0)
}

func (t *Terminal) OriginMode() bool { return t.originMode }

func (t *Terminal) SetInsertMode(v bool) { t.insertMode = v }
func (t *Terminal) InsertMode() bool     { return t.insertMode }

func (t *Terminal) CursorReport() (int, int) {
	row := t.cursorRow
	if t.originMode {
		row -= t.scrollTop
	}
	return row + 1, t.cursorCol + 1
}

func (t *Terminal) SetTabStop() {
	if t.cursorCol >= 0 && t.cursorCol < len(t.tabStops) {
		t.tabStops[t.cursorCol] = true
	}
}

func (t *Terminal) ClearTabStop() {
	if t.cursorCol >= 0 && t.cursorCol < len(t.tabStops) {
		t.tabStops[t.cursorCol] = false
	}
}

func (t *Terminal) ClearAllTabStops() {
	for i := range t.tabStops {
		t.tabStops[i] = false
	}
}

func (t *Terminal) CursorForwardTabs(n int) {
	if n <= 0 {
		n = 1
	}
	for ; n > 0; n-- {
		t.Tab()
	}
}

func (t *Terminal) CursorBackwardTabs(n int) {
	if n <= 0 {
		n = 1
	}
	for ; n > 0; n-- {
		prev := 0
		for col := t.cursorCol - 1; col >= 0; col-- {
			if col < len(t.tabStops) && t.tabStops[col] {
				prev = col
				break
			}
		}
		t.cursorCol = prev
	}
	t.wrapNext = false
}

func (t *Terminal) resetTabStops() {
	t.tabStops = make([]bool, t.cols)
	for i := 8; i < t.cols; i += 8 {
		t.tabStops[i] = true
	}
}

func (t *Terminal) resizeTabStops(oldCols, newCols int) {
	old := t.tabStops
	t.tabStops = make([]bool, newCols)
	copy(t.tabStops, old)
	if newCols > oldCols {
		for i := ((oldCols / 8) + 1) * 8; i < newCols; i += 8 {
			t.tabStops[i] = true
		}
	}
}

func (t *Terminal) SetCursorStyle(style CursorStyle) { t.cursorStyle = style }
func (t *Terminal) CursorStyle() CursorStyle         { return t.cursorStyle }

func (t *Terminal) FocusEventsMode() bool { return t.focusEvents }

func (t *Terminal) SetFocusEventsMode(v bool) {
	t.focusEvents = v
}
