//go:build glfw

package glfwgl

import (
	"testing"

	"cervterm/internal/core"
	termsel "cervterm/internal/selection"
)

func cellsFromRows(rows ...string) ([]core.Cell, int, int) {
	width := 0
	for _, r := range rows {
		if n := len([]rune(r)); n > width {
			width = n
		}
	}
	cells := make([]core.Cell, width*len(rows))
	for ri, r := range rows {
		for ci, ru := range []rune(r) {
			cells[ri*width+ci] = core.Cell{Rune: ru}
		}
	}
	return cells, width, len(rows)
}

func TestDetectLinks(t *testing.T) {
	cells, cols, rows := cellsFromRows("visita http://example.com hoy")
	links := detectLinks(cells, cols, rows)
	if len(links) != 1 {
		t.Fatalf("want 1 link, got %d: %+v", len(links), links)
	}
	l := links[0]
	if l.url != "http://example.com" {
		t.Fatalf("url = %q", l.url)
	}
	if l.row != 0 || l.startCol != 7 {
		t.Fatalf("region = row %d col %d..%d", l.row, l.startCol, l.endCol)
	}
	// startCol points at the 'h' of http.
	if cells[l.startCol].Rune != 'h' || cells[l.endCol].Rune != 'm' {
		t.Fatalf("region does not cover the URL: %q..%q", cells[l.startCol].Rune, cells[l.endCol].Rune)
	}
}

func TestDetectLinksTrimsTrailingPunct(t *testing.T) {
	cells, cols, rows := cellsFromRows("ver https://a.b/path.")
	links := detectLinks(cells, cols, rows)
	if len(links) != 1 || links[0].url != "https://a.b/path" {
		t.Fatalf("trailing punctuation not trimmed: %+v", links)
	}
}

func TestDetectLinksIgnoresBareScheme(t *testing.T) {
	cells, cols, rows := cellsFromRows("nada aqui http:// fin")
	if links := detectLinks(cells, cols, rows); len(links) != 0 {
		t.Fatalf("bare scheme should not be a link: %+v", links)
	}
}

func TestDetectLinksMultiplePerRow(t *testing.T) {
	cells, cols, rows := cellsFromRows("http://a.com y http://b.com")
	links := detectLinks(cells, cols, rows)
	if len(links) != 2 {
		t.Fatalf("want 2 links, got %d: %+v", len(links), links)
	}
	if links[0].url != "http://a.com" || links[1].url != "http://b.com" {
		t.Fatalf("urls = %q, %q", links[0].url, links[1].url)
	}
}

func TestLinkAt(t *testing.T) {
	cells, cols, rows := cellsFromRows("go http://x.io end")
	links := detectLinks(cells, cols, rows)
	if len(links) != 1 {
		t.Fatalf("setup: want 1 link")
	}
	l := links[0]
	if _, ok := linkAt(links, termsel.Point{Row: 0, Col: l.startCol}); !ok {
		t.Fatalf("start of link should hit")
	}
	if _, ok := linkAt(links, termsel.Point{Row: 0, Col: l.endCol}); !ok {
		t.Fatalf("end of link should hit")
	}
	if _, ok := linkAt(links, termsel.Point{Row: 0, Col: l.startCol - 1}); ok {
		t.Fatalf("just before the link should not hit")
	}
	if _, ok := linkAt(links, termsel.Point{Row: 1, Col: l.startCol}); ok {
		t.Fatalf("different row should not hit")
	}
}
