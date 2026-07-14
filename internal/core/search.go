package core

import (
	"strings"
	"unicode"
)

// SearchBackward finds the most recent (bottom-most) case-insensitive plain
// substring match for query at or above fromGlobalRow, searching upward.
//
// Rows are addressed in the same global (physical-row) index space Resize uses:
// index 0 is the oldest scrollback row and scrollbackRows+rows-1 is the last
// live row (see physicalRows). The returned globalRow/col identify the start
// cell of the match in that space; col is a cell column (wide-glyph aware).
//
// Semantics:
//   - The search is strictly above fromGlobalRow: only rows with a global index
//     < fromGlobalRow are considered. Pass scrollbackRows+rows the first time to
//     include every row (bottom-most match first); pass the previous match's
//     global row to step to the next match upward.
//   - Case folding uses simple per-rune lowercasing (unicode.ToLower on the
//     content, strings.ToLower on the query). This is a v1 simplification: it
//     does not perform full Unicode case folding of multi-rune mappings, but
//     handles the common ASCII/Latin cases the search UI targets.
//   - An empty query never matches (ok == false), so an empty search bar is a
//     no-op rather than a whole-buffer match storm.
//
// v1 limitation: matches are found within a single physical row only. A query
// that straddles a wrapped-line boundary (spanning two physical rows of one
// logical line) is not matched. This is documented and left for a later slice.
func (t *Terminal) SearchBackward(query string, fromGlobalRow int) (globalRow, col int, ok bool) {
	needle := []rune(strings.ToLower(query))
	if len(needle) == 0 {
		return 0, 0, false
	}
	rows, _ := t.physicalRows()
	start := fromGlobalRow - 1
	if start > len(rows)-1 {
		start = len(rows) - 1
	}
	for g := start; g >= 0; g-- {
		runes, cols := rowSearchRunes(rows[g])
		if idx := indexRunes(runes, needle); idx >= 0 {
			return g, cols[idx], true
		}
	}
	return 0, 0, false
}

// rowSearchRunes flattens a physical row into its lowercased visible runes plus
// a parallel slice giving the source cell column of each rune. Wide-glyph
// continuation cells are skipped (they carry no rune), and blank cells collapse
// to a space so a query with embedded spaces still matches. Combining marks are
// dropped from the searchable text for v1 simplicity.
func rowSearchRunes(row []Cell) (runes []rune, cols []int) {
	runes = make([]rune, 0, len(row))
	cols = make([]int, 0, len(row))
	for c := 0; c < len(row); c++ {
		cell := row[c]
		if cell.WideContinuation {
			continue
		}
		r := cell.Rune
		if r == 0 {
			r = ' '
		}
		runes = append(runes, unicode.ToLower(r))
		cols = append(cols, c)
	}
	return runes, cols
}

// indexRunes returns the index of the first occurrence of needle in hay, or -1.
// Rune-based (never byte-slices UTF-8) so multibyte content maps cleanly back to
// cell columns via the caller's parallel column slice.
func indexRunes(hay, needle []rune) int {
	if len(needle) == 0 || len(needle) > len(hay) {
		return -1
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		match := true
		for j := range needle {
			if hay[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
