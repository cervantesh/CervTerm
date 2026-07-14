package script

import (
	"strings"
	"testing"

	"cervterm/internal/config"
)

// loadOverlayRuntime loads a config and returns the runtime plus a fake host for
// dispatching keybindings that mutate overlays.
func loadOverlayRuntime(t *testing.T, body string) (*Runtime, *fakeHost) {
	t.Helper()
	path := writeScriptConfig(t, body)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	t.Cleanup(rt.Close)
	return rt, &fakeHost{}
}

func TestOverlayCommitAtomicity(t *testing.T) {
	rt, host := loadOverlayRuntime(t, `local cervterm = require("cervterm")
local ov = cervterm.overlay("p")
return { keys = {
  { key = "a", action = function() ov:rect(1, 1, 4, 2, "#112233") end },
  { key = "b", action = function() ov:commit() end },
} }`)

	// Building without committing leaves the committed scene empty and does not
	// bump seq: nothing shows half-built.
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch build: %v", err)
	}
	scenes := rt.Overlays()
	if len(scenes) != 1 {
		t.Fatalf("Overlays() = %d scenes, want 1", len(scenes))
	}
	if got := len(scenes[0].Prims); got != 0 {
		t.Fatalf("uncommitted scene has %d prims, want 0", got)
	}
	if seq := rt.OverlaySeq(); seq != 0 {
		t.Fatalf("building mutation bumped seq to %d, want 0", seq)
	}

	// commit atomically publishes the building list.
	if err := rt.Dispatch(1, host); err != nil {
		t.Fatalf("Dispatch commit: %v", err)
	}
	scenes = rt.Overlays()
	if got := len(scenes[0].Prims); got != 1 {
		t.Fatalf("committed scene has %d prims, want 1", got)
	}
	p := scenes[0].Prims[0]
	if p.Kind != OverlayRect || p.Col != 1 || p.Row != 1 || p.W != 4 || p.H != 2 {
		t.Fatalf("prim = %+v, unexpected", p)
	}
	if p.R != 0x11 || p.G != 0x22 || p.B != 0x33 || p.A != 0xFF {
		t.Fatalf("color = %v,%v,%v,%v want 11,22,33,FF", p.R, p.G, p.B, p.A)
	}
	if seq := rt.OverlaySeq(); seq != 1 {
		t.Fatalf("commit seq = %d, want 1", seq)
	}
}

func TestOverlaySeqBumpRules(t *testing.T) {
	rt, host := loadOverlayRuntime(t, `local cervterm = require("cervterm")
local ov = cervterm.overlay("p")
return { keys = {
  { key = "a", action = function() ov:rect(1, 1, 1, 1, "#ffffff") end },
  { key = "b", action = function() ov:commit() end },
  { key = "c", action = function() ov:hide() end },
  { key = "d", action = function() ov:hide() end },
  { key = "e", action = function() ov:show() end },
  { key = "f", action = function() ov:show() end },
  { key = "g", action = function() ov:destroy() end },
  { key = "h", action = function() ov:destroy() end },
} }`)

	steps := []struct {
		name    string
		binding int
		wantSeq int
	}{
		{"build no bump", 0, 0},
		{"commit bumps", 1, 1},
		{"hide bumps", 2, 2},
		{"hide again no bump", 3, 2},
		{"show bumps", 4, 3},
		{"show again no bump", 5, 3},
		{"destroy bumps", 6, 4},
		{"destroy again no bump", 7, 4},
	}
	for _, step := range steps {
		if err := rt.Dispatch(step.binding, host); err != nil {
			t.Fatalf("%s: Dispatch: %v", step.name, err)
		}
		if seq := rt.OverlaySeq(); seq != step.wantSeq {
			t.Fatalf("%s: seq = %d, want %d", step.name, seq, step.wantSeq)
		}
	}
}

func TestOverlayBudgetCap(t *testing.T) {
	rt, host := loadOverlayRuntime(t, `local cervterm = require("cervterm")
local ov = cervterm.overlay("p")
return { keys = {
  { key = "a", action = function()
      for i = 1, 600 do ov:rect(1, 1, 1, 1, "#ffffff") end
      ov:commit()
  end },
} }`)

	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	scenes := rt.Overlays()
	if got := len(scenes[0].Prims); got != overlayPrimBudget {
		t.Fatalf("committed prims = %d, want %d (budget cap)", got, overlayPrimBudget)
	}
	notices := rt.DrainOverlayNotices()
	if len(notices) != 1 || !strings.Contains(notices[0], "budget") {
		t.Fatalf("notices = %#v, want one budget notice", notices)
	}
	// Deduped: draining again yields nothing.
	if again := rt.DrainOverlayNotices(); again != nil {
		t.Fatalf("second drain = %#v, want nil", again)
	}
}

func TestOverlayInvalidDropsWithNotice(t *testing.T) {
	rt, host := loadOverlayRuntime(t, `local cervterm = require("cervterm")
local ov = cervterm.overlay("p")
return { keys = {
  { key = "a", action = function()
      ov:rect(1, 1, 1, 1, "not-a-color")
      ov:rect(0, 1, 1, 1, "#ffffff")
      ov:rect(2, 2, 3, 3, "#00ff00")
      ov:commit()
  end },
} }`)

	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	// Only the valid primitive survives; invalid color and invalid coord dropped.
	scenes := rt.Overlays()
	if got := len(scenes[0].Prims); got != 1 {
		t.Fatalf("committed prims = %d, want 1", got)
	}
	notices := rt.DrainOverlayNotices()
	if len(notices) != 2 {
		t.Fatalf("notices = %#v, want 2 (color + coords)", notices)
	}
}

func TestOverlayDestroyRemoves(t *testing.T) {
	rt, host := loadOverlayRuntime(t, `local cervterm = require("cervterm")
return { keys = {
  { key = "a", action = function()
      cervterm.overlay("a"):rect(1, 1, 1, 1, "#ffffff")
      cervterm.overlay("a"):commit()
      cervterm.overlay("b"):rect(1, 1, 1, 1, "#ffffff")
      cervterm.overlay("b"):commit()
  end },
  { key = "b", action = function() cervterm.overlay("a"):destroy() end },
} }`)

	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch build: %v", err)
	}
	if got := len(rt.Overlays()); got != 2 {
		t.Fatalf("Overlays() = %d, want 2", got)
	}
	if err := rt.Dispatch(1, host); err != nil {
		t.Fatalf("Dispatch destroy: %v", err)
	}
	scenes := rt.Overlays()
	if len(scenes) != 1 || scenes[0].ID != "b" {
		t.Fatalf("after destroy scenes = %#v, want only b", scenes)
	}
}

func TestOverlayPerIDIdentity(t *testing.T) {
	// Separate cervterm.overlay("p") calls must resolve to the same underlying
	// display lists: one call builds, a later call commits and publishes it.
	rt, host := loadOverlayRuntime(t, `local cervterm = require("cervterm")
return { keys = {
  { key = "a", action = function() cervterm.overlay("p"):text(2, 1, "hi", "#abcdef") end },
  { key = "b", action = function() cervterm.overlay("p"):commit() end },
} }`)

	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch build: %v", err)
	}
	if err := rt.Dispatch(1, host); err != nil {
		t.Fatalf("Dispatch commit: %v", err)
	}
	scenes := rt.Overlays()
	if len(scenes) != 1 || len(scenes[0].Prims) != 1 {
		t.Fatalf("scenes = %#v, want one overlay with one prim", scenes)
	}
	p := scenes[0].Prims[0]
	if p.Kind != OverlayText || p.Text != "hi" || p.Col != 2 || p.Row != 1 {
		t.Fatalf("prim = %+v, unexpected", p)
	}
}

func TestParseOverlayColor(t *testing.T) {
	cases := []struct {
		in         string
		r, g, b, a uint8
		ok         bool
	}{
		{"#000000", 0, 0, 0, 0xFF, true},
		{"#FFFFFF", 0xFF, 0xFF, 0xFF, 0xFF, true},
		{"#10141CF0", 0x10, 0x14, 0x1C, 0xF0, true},
		{"#abcdef", 0xAB, 0xCD, 0xEF, 0xFF, true},
		{"#AbCdEf12", 0xAB, 0xCD, 0xEF, 0x12, true},
		{"", 0, 0, 0, 0, false},
		{"10141C", 0, 0, 0, 0, false},
		{"#123", 0, 0, 0, 0, false},
		{"#12345", 0, 0, 0, 0, false},
		{"#gggggg", 0, 0, 0, 0, false},
		{"#10141CF0AA", 0, 0, 0, 0, false},
	}
	for _, c := range cases {
		r, g, b, a, ok := parseOverlayColor(c.in)
		if ok != c.ok || (ok && (r != c.r || g != c.g || b != c.b || a != c.a)) {
			t.Fatalf("parseOverlayColor(%q) = %v,%v,%v,%v,%v want %v,%v,%v,%v,%v",
				c.in, r, g, b, a, ok, c.r, c.g, c.b, c.a, c.ok)
		}
	}
}

func TestClipCellRect(t *testing.T) {
	cases := []struct {
		name           string
		col, row, w, h int
		cols, rows     int
		x0, y0, x1, y1 int
		ok             bool
	}{
		{"fully inside", 2, 3, 4, 2, 80, 24, 1, 2, 4, 3, true},
		{"top-left origin", 1, 1, 1, 1, 80, 24, 0, 0, 0, 0, true},
		{"clip right/bottom", 79, 23, 10, 10, 80, 24, 78, 22, 79, 23, true},
		{"off right edge", 81, 1, 3, 1, 80, 24, 0, 0, 0, 0, false},
		{"off bottom edge", 1, 25, 1, 3, 80, 24, 0, 0, 0, 0, false},
		{"negative start clipped", 1, 1, 3, 3, 2, 2, 0, 0, 1, 1, true},
		{"empty grid", 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, false},
	}
	for _, c := range cases {
		x0, y0, x1, y1, ok := ClipCellRect(c.col, c.row, c.w, c.h, c.cols, c.rows)
		if ok != c.ok || (ok && (x0 != c.x0 || y0 != c.y0 || x1 != c.x1 || y1 != c.y1)) {
			t.Fatalf("%s: ClipCellRect = %d,%d,%d,%d,%v want %d,%d,%d,%d,%v",
				c.name, x0, y0, x1, y1, ok, c.x0, c.y0, c.x1, c.y1, c.ok)
		}
	}
}

func TestCoveredRows(t *testing.T) {
	prims := []OverlayPrim{
		{Kind: OverlayRect, Col: 1, Row: 3, W: 2, H: 4}, // rows 2..5 (0-based)
		{Kind: OverlayText, Col: 1, Row: 1},             // row 0
		{Kind: OverlayVLine, Col: 1, Row: 10, H: 100},   // rows 9..23 clipped
	}
	first, last, any := CoveredRows(prims, 24)
	if !any || first != 0 || last != 23 {
		t.Fatalf("CoveredRows = %d,%d,%v want 0,23,true", first, last, any)
	}

	// Rect fully below the grid contributes nothing.
	below := []OverlayPrim{{Kind: OverlayRect, Col: 1, Row: 100, W: 1, H: 1}}
	if _, _, any := CoveredRows(below, 24); any {
		t.Fatalf("off-grid prim reported covered rows")
	}
	if _, _, any := CoveredRows(nil, 24); any {
		t.Fatalf("empty scene reported covered rows")
	}
}
