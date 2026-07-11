package core

import "cervterm/internal/unicodeprops"

func RuneWidth(r rune) int {
	return unicodeprops.DisplayWidthRune(r)
}
