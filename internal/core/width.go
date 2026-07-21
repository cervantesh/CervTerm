package core

import "cervterm/internal/unicodeprops"

func RuneWidth(r rune) int {
	if r > 0 && r < 0x7f {
		return 1
	}
	return unicodeprops.DisplayWidthRune(r)
}
