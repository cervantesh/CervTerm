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

type recordingURLLauncher struct {
	opened []string
	err    error
}

func (r *recordingURLLauncher) Launch(uri string) error {
	r.opened = append(r.opened, uri)
	return r.err
}

func TestDetectSnapshotLinksUsesOSC8IdentityAndPrecedence(t *testing.T) {
	cells, cols, rows := cellsFromRows("https://plain.test")
	for index := range cells {
		cells[index].HyperlinkID = 7
	}
	links := detectSnapshotLinks(cells, []core.Hyperlink{{ID: 7, URI: "https://target.test/path"}}, cols, rows)
	if len(links) != 1 || !links[0].explicit || links[0].url != "https://target.test/path" || links[0].startCol != 0 || links[0].endCol != cols-1 {
		t.Fatalf("links=%#v", links)
	}
}

func TestOSC8ActivationRequiresFreshClickAndSafeScheme(t *testing.T) {
	a := newMuxTestApp(t, 80, 24)
	launcher := &recordingURLLauncher{}
	a.linkLauncher = launcher
	feedTestPane(t, a, []byte("\x1b]8;;HTTPS://EXAMPLE.TEST/path?secret=x\x1b\\go\x1b]8;;\x1b\\"))
	a.syncFocusedProjection()
	a.refreshLinks()
	if len(launcher.opened) != 0 {
		t.Fatal("terminal output opened link automatically")
	}
	a.captureLinkPress(termsel.Point{Row: 0, Col: 0})
	if !a.handleLinkClick(termsel.Point{Row: 0, Col: 0}) || len(launcher.opened) != 1 || launcher.opened[0] != "https://example.test/path?secret=x" {
		t.Fatalf("opened=%#v", launcher.opened)
	}
	feedTestPane(t, a, []byte("\r\x1b[2K\x1b]8;;file:///etc/passwd\x1b\\x\x1b]8;;\x1b\\"))
	a.syncFocusedProjection()
	a.refreshLinks()
	a.captureLinkPress(termsel.Point{Row: 0, Col: 0})
	if !a.handleLinkClick(termsel.Point{Row: 0, Col: 0}) || len(launcher.opened) != 1 {
		t.Fatalf("unsafe opened=%#v", launcher.opened)
	}
}

func TestOSC8ActivationRejectsStaleRegion(t *testing.T) {
	a := newMuxTestApp(t, 80, 24)
	launcher := &recordingURLLauncher{}
	a.linkLauncher = launcher
	feedTestPane(t, a, []byte("\x1b]8;;https://old.test\x1b\\old\x1b]8;;\x1b\\"))
	a.syncFocusedProjection()
	a.refreshLinks()
	a.captureLinkPress(termsel.Point{Row: 0, Col: 0})
	feedTestPane(t, a, []byte("\r\x1b[2Knew"))
	a.syncFocusedProjection()
	if !a.handleLinkClick(termsel.Point{Row: 0, Col: 0}) || len(launcher.opened) != 0 {
		t.Fatalf("stale opened=%#v", launcher.opened)
	}
}

func TestOSC8ActivationRejectsSameURIIdentitySwapAfterPress(t *testing.T) {
	a := newMuxTestApp(t, 80, 24)
	launcher := &recordingURLLauncher{}
	a.linkLauncher = launcher
	point := termsel.Point{Row: 0, Col: 0}
	feedTestPane(t, a, []byte("\x1b]8;id=one;https://same.test\x1b\\same\x1b]8;;\x1b\\"))
	a.syncFocusedProjection()
	a.refreshLinks()
	a.captureLinkPress(point)
	feedTestPane(t, a, []byte("\r\x1b[2K\x1b]8;id=two;https://same.test\x1b\\same\x1b]8;;\x1b\\"))
	a.syncFocusedProjection()
	a.refreshLinks()
	if !a.handleLinkClick(point) || len(launcher.opened) != 0 {
		t.Fatalf("opened=%#v", launcher.opened)
	}
}
