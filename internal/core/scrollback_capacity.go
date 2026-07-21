package core

// SetScrollbackCapacity resizes this terminal's history ring, retaining the newest
// rows and clamping the viewport. It also updates the saved primary screen when
// called while the alternate screen is active.
func (t *Terminal) SetScrollbackCapacity(capacity int) {
	if capacity < 0 {
		capacity = 0
	} else if capacity > maxScrollbackRows {
		capacity = maxScrollbackRows
	}
	oldRows := t.scrollbackRows
	if t.primaryScreen != nil {
		oldRows = t.primaryScreen.scrollbackRows
	}
	if t.imageSidecars != nil {
		t.imagesSetScrollbackCapacity(oldRows, min(oldRows, capacity))
	}
	if t.primaryScreen != nil {
		p := t.primaryScreen
		tmp := &Terminal{cols: p.cols, scrollback: p.scrollback, scrollbackWrapped: p.scrollbackWrapped, scrollbackStart: p.scrollbackStart, scrollbackRows: p.scrollbackRows, scrollbackCapacity: p.scrollbackCapacity, displayOffset: p.displayOffset}
		tmp.setScrollbackCapacity(capacity)
		p.scrollback, p.scrollbackWrapped = tmp.scrollback, tmp.scrollbackWrapped
		p.scrollbackStart, p.scrollbackRows = tmp.scrollbackStart, tmp.scrollbackRows
		p.scrollbackCapacity, p.displayOffset = tmp.scrollbackCapacity, tmp.displayOffset
	}
	if t.alternateScreen {
		// Alternate screens never own history; the saved primary was resized above.
		t.scrollback, t.scrollbackWrapped = nil, nil
		t.scrollbackStart, t.scrollbackRows, t.scrollbackCapacity, t.displayOffset = 0, 0, 0, 0
		return
	}
	t.setScrollbackCapacity(capacity)
}

func (t *Terminal) setScrollbackCapacity(capacity int) {
	if capacity == t.scrollbackCapacity {
		return
	}
	keep := min(t.scrollbackRows, capacity)
	var cells []Cell
	var wrapped []bool
	if capacity > 0 {
		cells = make([]Cell, capacity*t.cols)
		wrapped = make([]bool, capacity)
		first := t.scrollbackRows - keep
		for i := 0; i < keep; i++ {
			source := (t.scrollbackStart + first + i) % max(1, t.scrollbackCapacity)
			copy(cells[i*t.cols:(i+1)*t.cols], t.scrollback[source*t.cols:(source+1)*t.cols])
			if source < len(t.scrollbackWrapped) {
				wrapped[i] = t.scrollbackWrapped[source]
			}
		}
	}
	t.scrollback, t.scrollbackWrapped = cells, wrapped
	t.scrollbackStart, t.scrollbackRows, t.scrollbackCapacity = 0, keep, capacity
	t.displayOffset = min(t.displayOffset, keep)
}
