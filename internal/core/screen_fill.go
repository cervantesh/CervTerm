package core

func (t *Terminal) fillBlank(cells []Cell) {
	if len(cells) == 0 {
		return
	}
	cells[0] = t.blank()
	for filled := 1; filled < len(cells); filled *= 2 {
		copy(cells[filled:], cells[:min(filled, len(cells)-filled)])
	}
}
