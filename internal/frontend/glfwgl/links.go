//go:build glfw

package glfwgl

import (
	"cervterm/internal/core"
	termsel "cervterm/internal/selection"
)

// linkRegion is a detected http(s) URL occupying one visual row, columns
// startCol..endCol inclusive.
type linkRegion struct {
	row      int
	startCol int
	endCol   int
	url      string
}

// detectLinks scans the visible grid for http:// and https:// URLs, one visual
// row at a time (a URL that wraps across rows is detected only up to the row
// boundary — documented v1 limitation, matching scrollback search). Pure so it
// is unit-testable without an App or GL context.
func detectLinks(cells []core.Cell, cols, rows int) []linkRegion {
	if cols <= 0 || rows <= 0 || len(cells) < cols*rows {
		return nil
	}
	var links []linkRegion
	for row := 0; row < rows; row++ {
		runes := make([]rune, 0, cols)
		colOf := make([]int, 0, cols)
		for col := 0; col < cols; col++ {
			cell := cells[row*cols+col]
			if cell.WideContinuation {
				continue
			}
			r := cell.Rune
			if r == 0 {
				r = ' '
			}
			runes = append(runes, r)
			colOf = append(colOf, col)
		}
		links = appendRowLinks(links, row, runes, colOf)
	}
	return links
}

func appendRowLinks(links []linkRegion, row int, runes []rune, colOf []int) []linkRegion {
	i := 0
	for i < len(runes) {
		if !matchesScheme(runes, i) {
			i++
			continue
		}
		j := i
		for j < len(runes) && isURLRune(runes[j]) {
			j++
		}
		end := j
		for end > i && isTrailingPunct(runes[end-1]) {
			end--
		}
		// Require something after the scheme's "://" so a bare "http://" is skipped.
		if end-i > len("https://") {
			links = append(links, linkRegion{
				row:      row,
				startCol: colOf[i],
				endCol:   colOf[end-1],
				url:      string(runes[i:end]),
			})
		}
		i = j
	}
	return links
}

func matchesScheme(runes []rune, i int) bool {
	return hasPrefixAt(runes, i, "http://") || hasPrefixAt(runes, i, "https://")
}

func hasPrefixAt(runes []rune, i int, prefix string) bool {
	p := []rune(prefix)
	if i+len(p) > len(runes) {
		return false
	}
	for k, r := range p {
		if runes[i+k] != r {
			return false
		}
	}
	return true
}

// isURLRune accepts printable ASCII that commonly appears in URLs, stopping at
// whitespace and delimiters that would not be part of a link in terminal text.
func isURLRune(r rune) bool {
	if r <= ' ' || r >= 0x7f {
		return false
	}
	switch r {
	case '"', '<', '>', '`', '{', '}', '|', '\\', '^':
		return false
	}
	return true
}

// isTrailingPunct trims sentence punctuation that tends to follow a URL rather
// than belong to it.
func isTrailingPunct(r rune) bool {
	switch r {
	case '.', ',', ';', ':', '!', '?', ')', ']', '}', '\'', '"':
		return true
	}
	return false
}

// linkAt returns the link under the given grid point, if any.
func linkAt(links []linkRegion, p termsel.Point) (linkRegion, bool) {
	for _, l := range links {
		if l.row == p.Row && p.Col >= l.startCol && p.Col <= l.endCol {
			return l, true
		}
	}
	return linkRegion{}, false
}
