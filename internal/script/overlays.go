package script

import (
	"fmt"
	"strconv"

	lua "github.com/yuin/gopher-lua"
)

// OverlayPrimKind enumerates the retained display-list primitive types a script
// can build into an overlay scene.
type OverlayPrimKind int

const (
	OverlayRect OverlayPrimKind = iota
	OverlayText
	OverlayHLine
	OverlayVLine
)

// overlayPrimBudget caps primitives per overlay. Enforced at build time so the
// render pass never allocates or checks a limit per frame (trap 5).
const overlayPrimBudget = 512

// OverlayPrim is one cell-addressed primitive in a committed overlay scene.
// Col/Row are 1-based cell coordinates as supplied by Lua; the frontend clips
// them to the live grid. Colors are pre-parsed to RGBA at build time so the
// render pass performs no per-frame string parsing (trap 5).
type OverlayPrim struct {
	Kind       OverlayPrimKind
	Col, Row   int
	W, H       int
	Text       string
	R, G, B, A uint8
}

// OverlayScene is a committed overlay's display list plus visibility, exposed to
// the frontend in creation order.
type OverlayScene struct {
	ID      string
	Visible bool
	Prims   []OverlayPrim
}

// overlay holds one id's two display lists: building (mutated by rect/text/...)
// and committed (the atomically-swapped snapshot the frontend renders). A
// freshly created overlay is visible so the common build+commit path shows
// without an explicit show(); only committed prims ever render, so nothing shows
// half-built.
type overlay struct {
	id        string
	committed []OverlayPrim
	building  []OverlayPrim
	visible   bool
	destroyed bool
	handle    *lua.LTable
}

// overlayStore is the runtime's main-thread-only overlay store, keyed by id and
// ordered by creation. seq bumps only on commit/show/hide/destroy — never on
// building-list mutations — so the frontend re-syncs and repaints only on a real
// scene change. notices collects deduped build-time diagnostics (invalid
// color/coords, budget) that the frontend drains and surfaces (trap 4). No
// locking: every mutation happens on the loop thread.
type overlayStore struct {
	order   []string
	byID    map[string]*overlay
	seq     int
	notices []string
	noted   map[string]bool
}

func (s *overlayStore) get(id string) *overlay {
	if s.byID == nil {
		s.byID = make(map[string]*overlay)
	}
	if ov, ok := s.byID[id]; ok {
		return ov
	}
	ov := &overlay{id: id, visible: true}
	s.byID[id] = ov
	s.order = append(s.order, id)
	return ov
}

// notify queues a build-time diagnostic once per unique message so a repeatedly
// failing primitive shows a single notice, not one per call or per frame.
func (s *overlayStore) notify(msg string) {
	if s.noted == nil {
		s.noted = make(map[string]bool)
	}
	if s.noted[msg] {
		return
	}
	s.noted[msg] = true
	s.notices = append(s.notices, msg)
}

func (s *overlayStore) drainNotices() []string {
	if len(s.notices) == 0 {
		return nil
	}
	out := s.notices
	s.notices = nil
	return out
}

// addPrim validates coordinates and color, enforces the budget, and appends to
// the building list. An invalid primitive is dropped with a deduped notice and
// never breaks the scene (trap 4).
func (s *overlayStore) addPrim(ov *overlay, kind OverlayPrimKind, col, row, w, h int, text, colorStr string) {
	if ov.destroyed {
		return
	}
	r, g, b, a, ok := parseOverlayColor(colorStr)
	if !ok {
		s.notify(fmt.Sprintf("overlay %q: invalid color %q", ov.id, colorStr))
		return
	}
	if col < 1 || row < 1 || w < 1 || h < 1 {
		s.notify(fmt.Sprintf("overlay %q: invalid coordinates", ov.id))
		return
	}
	if len(ov.building) >= overlayPrimBudget {
		s.notify(fmt.Sprintf("overlay %q: primitive budget (%d) exceeded", ov.id, overlayPrimBudget))
		return
	}
	ov.building = append(ov.building, OverlayPrim{Kind: kind, Col: col, Row: row, W: w, H: h, Text: text, R: r, G: g, B: b, A: a})
}

func (s *overlayStore) clear(ov *overlay) {
	if ov.destroyed {
		return
	}
	ov.building = ov.building[:0]
}

// commit atomically swaps the building list into committed. It copies into
// committed's own backing array so later building mutations never alias the
// rendered snapshot. Each commit is a new scene, so seq always bumps.
func (s *overlayStore) commit(ov *overlay) {
	if ov.destroyed {
		return
	}
	ov.committed = append(ov.committed[:0], ov.building...)
	s.seq++
}

func (s *overlayStore) setVisible(ov *overlay, visible bool) {
	if ov.destroyed || ov.visible == visible {
		return
	}
	ov.visible = visible
	s.seq++
}

func (s *overlayStore) destroy(ov *overlay) {
	if ov.destroyed {
		return
	}
	ov.destroyed = true
	delete(s.byID, ov.id)
	for i, id := range s.order {
		if id == ov.id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	s.seq++
}

// scenes returns the committed display lists in creation order, each with a
// private copy of its primitives so the frontend can hold the slice across
// frames without aliasing store state. Called only on a seq change, not per
// frame.
func (s *overlayStore) scenes() []OverlayScene {
	out := make([]OverlayScene, 0, len(s.order))
	for _, id := range s.order {
		ov := s.byID[id]
		prims := make([]OverlayPrim, len(ov.committed))
		copy(prims, ov.committed)
		out = append(out, OverlayScene{ID: id, Visible: ov.visible, Prims: prims})
	}
	return out
}

// newOverlayHandle builds the Lua handle table for an overlay. The method
// closures capture the store and the specific overlay, so every
// cervterm.overlay(id) for the same id returns a handle over the same underlying
// display lists (per-id identity). Methods use the : call convention, so arg 1
// is the handle table.
func newOverlayHandle(l *lua.LState, store *overlayStore, ov *overlay) *lua.LTable {
	tbl := l.NewTable()
	reg := func(name string, fn lua.LGFunction) {
		tbl.RawSetString(name, l.NewFunction(fn))
	}
	reg("clear", func(l *lua.LState) int {
		l.CheckTypes(1, lua.LTTable)
		store.clear(ov)
		return 0
	})
	reg("rect", func(l *lua.LState) int {
		l.CheckTypes(1, lua.LTTable)
		store.addPrim(ov, OverlayRect, l.CheckInt(2), l.CheckInt(3), l.CheckInt(4), l.CheckInt(5), "", l.CheckString(6))
		return 0
	})
	reg("text", func(l *lua.LState) int {
		l.CheckTypes(1, lua.LTTable)
		store.addPrim(ov, OverlayText, l.CheckInt(2), l.CheckInt(3), 1, 1, l.CheckString(4), l.CheckString(5))
		return 0
	})
	reg("hline", func(l *lua.LState) int {
		l.CheckTypes(1, lua.LTTable)
		store.addPrim(ov, OverlayHLine, l.CheckInt(2), l.CheckInt(3), l.CheckInt(4), 1, "", l.CheckString(5))
		return 0
	})
	reg("vline", func(l *lua.LState) int {
		l.CheckTypes(1, lua.LTTable)
		store.addPrim(ov, OverlayVLine, l.CheckInt(2), l.CheckInt(3), 1, l.CheckInt(4), "", l.CheckString(5))
		return 0
	})
	reg("commit", func(l *lua.LState) int {
		l.CheckTypes(1, lua.LTTable)
		store.commit(ov)
		return 0
	})
	reg("show", func(l *lua.LState) int {
		l.CheckTypes(1, lua.LTTable)
		store.setVisible(ov, true)
		return 0
	})
	reg("hide", func(l *lua.LState) int {
		l.CheckTypes(1, lua.LTTable)
		store.setVisible(ov, false)
		return 0
	})
	reg("destroy", func(l *lua.LState) int {
		l.CheckTypes(1, lua.LTTable)
		store.destroy(ov)
		return 0
	})
	return tbl
}

// OverlaySeq returns the monotonic overlay mutation counter. It bumps on
// commit/show/hide/destroy only, so the frontend re-syncs scenes and forces a
// one-frame full redraw exactly when the visible scene set changes.
func (r *Runtime) OverlaySeq() int {
	if r.overlays == nil {
		return 0
	}
	return r.overlays.seq
}

// Overlays returns the committed scenes in creation order.
func (r *Runtime) Overlays() []OverlayScene {
	if r.overlays == nil {
		return nil
	}
	return r.overlays.scenes()
}

// DrainOverlayNotices returns and clears the pending deduped build-time notices
// (invalid color/coords, budget). The frontend surfaces each once per loop pass.
func (r *Runtime) DrainOverlayNotices() []string {
	if r.overlays == nil {
		return nil
	}
	return r.overlays.drainNotices()
}

// parseOverlayColor parses #RRGGBB or #RRGGBBAA (case-insensitive). Alpha
// defaults to opaque when omitted. ok is false for any malformed input, which
// the caller turns into a deduped notice + dropped primitive.
func parseOverlayColor(s string) (r, g, b, a uint8, ok bool) {
	if len(s) == 0 || s[0] != '#' {
		return 0, 0, 0, 0, false
	}
	hex := s[1:]
	if len(hex) != 6 && len(hex) != 8 {
		return 0, 0, 0, 0, false
	}
	var vals [4]uint8
	vals[3] = 0xFF
	for i := 0; i < len(hex)/2; i++ {
		v, err := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
		if err != nil {
			return 0, 0, 0, 0, false
		}
		vals[i] = uint8(v)
	}
	return vals[0], vals[1], vals[2], vals[3], true
}

// ClipCellRect clips a 1-based cell rectangle to the 0-based grid, returning the
// visible inclusive cell bounds. ok is false when the rectangle lies entirely
// off the grid (clipped away silently, never an error).
func ClipCellRect(col, row, w, h, cols, rows int) (x0, y0, x1, y1 int, ok bool) {
	if cols <= 0 || rows <= 0 || w < 1 || h < 1 {
		return 0, 0, 0, 0, false
	}
	x0, y0 = col-1, row-1
	x1, y1 = col-1+w-1, row-1+h-1
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > cols-1 {
		x1 = cols - 1
	}
	if y1 > rows-1 {
		y1 = rows - 1
	}
	if x0 > x1 || y0 > y1 {
		return 0, 0, 0, 0, false
	}
	return x0, y0, x1, y1, true
}

// CoveredRows returns the union of grid rows (0-based, inclusive) a scene's
// primitives touch, clipped to the grid. The frontend marks these rows damaged
// every frame a visible overlay renders, so a translucent overlay recomposites
// over a freshly painted terminal row instead of blending onto itself (trap 1).
// any is false when no primitive is on-grid vertically.
func CoveredRows(prims []OverlayPrim, rows int) (first, last int, any bool) {
	first, last = rows, -1
	for _, p := range prims {
		h := p.H
		if p.Kind == OverlayText || p.Kind == OverlayHLine {
			h = 1
		}
		top, bot := p.Row-1, p.Row-1+h-1
		if top < 0 {
			top = 0
		}
		if bot > rows-1 {
			bot = rows - 1
		}
		if top > bot {
			continue
		}
		if top < first {
			first = top
		}
		if bot > last {
			last = bot
		}
		any = true
	}
	if !any {
		return 0, 0, false
	}
	return first, last, true
}
