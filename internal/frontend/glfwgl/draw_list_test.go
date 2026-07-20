package glfwgl

import (
	"image/color"
	"reflect"
	"testing"
)

var (
	testChrome = color.RGBA{0x10, 0x20, 0x30, 0xEE}
	testAccent = color.RGBA{0x33, 0x99, 0xFF, 0xFF}
	testMuted  = color.RGBA{0x66, 0x66, 0x66, 0xFF}
)

const (
	tCellW = 10
	tCellH = 20
)

// --- hudLayout ---

func TestHUDLayoutEmpty(t *testing.T) {
	if got := hudLayout(nil, nil, tCellW, tCellH, 1, testChrome, testAccent); got != nil {
		t.Fatalf("expected nil for empty lines, got %v", got)
	}
	if got := hudLayout([]string{}, []color.RGBA{}, tCellW, tCellH, 1, testChrome, testAccent); got != nil {
		t.Fatalf("expected nil for zero-length lines, got %v", got)
	}
}

func TestHUDLayoutNoticeOnly(t *testing.T) {
	lines := []string{"hi"}
	colors := []color.RGBA{testAccent}
	pad := float32(6)
	got := hudLayout(lines, colors, tCellW, tCellH, 1, testChrome, testAccent)
	want := []drawCmd{
		{kind: cmdRect, x: pad, y: pad, w: 2*tCellW + 2*pad, h: tCellH + 2*pad, col: testChrome},
		{kind: cmdRect, x: pad, y: pad, w: 2*tCellW + 2*pad, h: 1, col: testAccent},
		{kind: cmdText, x: pad + pad, y: pad + pad, text: "hi", col: testAccent},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 cmds, got %d", len(got))
	}
	if got[0].h != tCellH+2*pad {
		t.Fatalf("bh = %v, want %v", got[0].h, tCellH+2*pad)
	}
}

func TestHUDLayoutStatsOnly(t *testing.T) {
	lines := []string{"short", "a wider line"}
	colors := []color.RGBA{testMuted, testMuted}
	pad := float32(6)
	got := hudLayout(lines, colors, tCellW, tCellH, 1, testChrome, testAccent)
	if len(got) != 4 {
		t.Fatalf("expected 4 cmds, got %d", len(got))
	}
	if got[0].col != testChrome {
		t.Fatalf("stats background = %v, want custom chrome %v", got[0].col, testChrome)
	}
	widest := len([]rune("a wider line")) // 12
	wantBW := float32(widest)*tCellW + 2*pad
	if got[0].w != wantBW {
		t.Fatalf("bw = %v, want %v", got[0].w, wantBW)
	}
	// text rows: y of line i = pad + pad + i*cellH == 2*pad + i*cellH
	for i := 0; i < 2; i++ {
		wantY := 2*pad + float32(i)*tCellH
		if got[2+i].y != wantY {
			t.Fatalf("line %d y = %v, want %v", i, got[2+i].y, wantY)
		}
	}
}

func TestHUDLayoutStatsPlusNotice(t *testing.T) {
	lines := []string{"one", "two", "three"}
	colors := []color.RGBA{testMuted, testMuted, testAccent}
	pad := float32(6)
	got := hudLayout(lines, colors, tCellW, tCellH, 1, testChrome, testAccent)
	if len(got) != 5 {
		t.Fatalf("expected 5 cmds, got %d", len(got))
	}
	wantBH := 3*float32(tCellH) + 2*pad
	if got[0].h != wantBH {
		t.Fatalf("bh = %v, want %v", got[0].h, wantBH)
	}
}

func TestHUDLayoutRuneWidth(t *testing.T) {
	lines := []string{"ñoño"} // 4 runes, more bytes
	colors := []color.RGBA{testAccent}
	pad := float32(6)
	got := hudLayout(lines, colors, tCellW, tCellH, 1, testChrome, testAccent)
	wantBW := float32(4)*tCellW + 2*pad
	if got[0].w != wantBW {
		t.Fatalf("bw = %v, want %v (rune count, not bytes)", got[0].w, wantBW)
	}
}

func TestHUDLayoutUIScale(t *testing.T) {
	lines := []string{"x"}
	colors := []color.RGBA{testAccent}

	// uiScale = 2 → pad = 12, accent thickness = 2
	got := hudLayout(lines, colors, tCellW, tCellH, 2, testChrome, testAccent)
	if got[0].x != 12 || got[0].y != 12 {
		t.Fatalf("uiScale=2 box origin = (%v,%v), want (12,12)", got[0].x, got[0].y)
	}
	if got[1].h != 2 {
		t.Fatalf("uiScale=2 accent thickness = %v, want 2", got[1].h)
	}
	if got[2].x != 24 || got[2].y != 24 {
		t.Fatalf("uiScale=2 text origin = (%v,%v), want (24,24)", got[2].x, got[2].y)
	}

	// uiScale = 0.5 → accent thickness clamps to 1
	got = hudLayout(lines, colors, tCellW, tCellH, 0.5, testChrome, testAccent)
	if got[1].h != 1 {
		t.Fatalf("uiScale=0.5 accent thickness = %v, want 1 (clamped)", got[1].h)
	}
}

// --- searchBarLayout ---

func TestSearchBarInactive(t *testing.T) {
	if got := searchBarLayout(false, "foo", true, 800, 600, tCellH, 1, testChrome, testAccent, testMuted); got != nil {
		t.Fatalf("expected nil when inactive, got %v", got)
	}
}

func TestSearchBarEmptyQuery(t *testing.T) {
	winW, winH := 800, 600
	pad := float32(6)
	got := searchBarLayout(true, "", true, winW, winH, tCellH, 1, testChrome, testAccent, testMuted)
	if len(got) != 3 {
		t.Fatalf("expected 3 cmds, got %d", len(got))
	}
	bh := float32(tCellH) + 2*pad
	wantBy := float32(winH) - bh
	if got[0].y != wantBy {
		t.Fatalf("box y = %v, want %v", got[0].y, wantBy)
	}
	if got[0].w != float32(winW) {
		t.Fatalf("box w = %v, want %v", got[0].w, float32(winW))
	}
	if got[0].col != testChrome {
		t.Fatalf("search background = %v, want custom chrome %v", got[0].col, testChrome)
	}
	if got[2].text != "buscar: " {
		t.Fatalf("text = %q, want %q", got[2].text, "buscar: ")
	}
	if got[2].col != testAccent {
		t.Fatalf("text color = %v, want accent", got[2].col)
	}
}

func TestSearchBarQueryHasMatch(t *testing.T) {
	got := searchBarLayout(true, "foo", true, 800, 600, tCellH, 1, testChrome, testAccent, testMuted)
	if got[2].text != "buscar: foo  [enter: siguiente]" {
		t.Fatalf("text = %q", got[2].text)
	}
	if got[2].col != testAccent {
		t.Fatalf("text color = %v, want accent", got[2].col)
	}
}

func TestSearchBarQueryNoMatch(t *testing.T) {
	got := searchBarLayout(true, "foo", false, 800, 600, tCellH, 1, testChrome, testAccent, testMuted)
	if got[2].text != "buscar: foo  sin resultados" {
		t.Fatalf("text = %q", got[2].text)
	}
	if got[2].col != testMuted {
		t.Fatalf("text color = %v, want muted", got[2].col)
	}
}

// --- statusBandLayout ---

func TestStatusBandEmpty(t *testing.T) {
	if got := statusBandLayout("", 100, 800, 4, tCellH, 1, testChrome, testAccent); got != nil {
		t.Fatalf("expected nil for empty display, got %v", got)
	}
}

func TestStatusBandNormal(t *testing.T) {
	winW := 800
	bandWidth := float32(120)
	paddingY := float32(4)
	pad := float32(6)
	got := statusBandLayout("status", bandWidth, winW, paddingY, tCellH, 1, testChrome, testAccent)
	if len(got) != 3 {
		t.Fatalf("expected 3 cmds, got %d", len(got))
	}
	wantBx := float32(winW) - bandWidth
	if got[0].x != wantBx {
		t.Fatalf("bx = %v, want %v", got[0].x, wantBx)
	}
	if got[0].h != tCellH {
		t.Fatalf("box h = %v, want cellH %v", got[0].h, float32(tCellH))
	}
	if got[0].col != testChrome {
		t.Fatalf("status background = %v, want custom chrome %v", got[0].col, testChrome)
	}
	// text at (bx+pad, paddingY) — y WITHOUT pad
	if got[2].x != wantBx+pad {
		t.Fatalf("text x = %v, want %v", got[2].x, wantBx+pad)
	}
	if got[2].y != paddingY {
		t.Fatalf("text y = %v, want %v (no pad offset)", got[2].y, paddingY)
	}
}

func TestStatusBandFullWidth(t *testing.T) {
	winW := 800
	bandWidth := float32(winW)
	got := statusBandLayout("full", bandWidth, winW, 4, tCellH, 1, testChrome, testAccent)
	if got[0].x != 0 {
		t.Fatalf("bx = %v, want 0 when bandWidth == winW", got[0].x)
	}
}
