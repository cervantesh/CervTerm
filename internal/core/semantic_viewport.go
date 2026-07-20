package core

// ScrollViewportToGlobalRow aligns a snapshot-relative physical row with the
// viewport top, clamped to the scrollable history boundary.
func (t *Terminal) ScrollViewportToGlobalRow(globalRow int) bool {
	if globalRow < 0 {
		globalRow = 0
	}
	if globalRow > t.scrollbackRows {
		globalRow = t.scrollbackRows
	}
	previous := t.displayOffset
	t.displayOffset = t.scrollbackRows - globalRow
	return t.displayOffset != previous
}
