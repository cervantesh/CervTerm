package core

import "cervterm/internal/termimage"

type Attr struct {
	FG, BG                                LogicalColor
	Bold, Dim, Italic, Underline, Inverse bool
	Strikethrough, Blink                  bool
}

// HasExplicitBG reports whether the cell carries a non-default logical
// background, including an RGB literal equal to the current physical default.
func (a Attr) HasExplicitBG() bool { return !a.BG.IsDefault() }

// HasExplicitFG reports whether the cell carries a non-default logical
// foreground, including an RGB literal equal to the current physical default.
func (a Attr) HasExplicitFG() bool { return !a.FG.IsDefault() }

type Cell struct {
	// noCompare keeps Cell non-comparable (it always was, via the old []rune
	// field). A pointer combining field would otherwise make Cell comparable and
	// invite future `==` that compares pointer identity, not mark content. Zero
	// size; placed first so it adds no padding (a zero-size field last would).
	noCompare [0]func()
	// combining points to the cell's stacked zero-width marks, or nil (the common
	// case). A pointer (8 B on 64-bit) instead of a slice header (24 B) keeps Cell
	// at 32 B on 64-bit — the shrink that this and the accessor seam existed for.
	// Cells are copied by value constantly (scrollback, snapshots); mutation goes
	// through AppendCombining, which copies-on-write so a copy is never disturbed.
	combining        *[]rune
	Rune             rune
	Attr             Attr
	HyperlinkID      HyperlinkID
	WideContinuation bool
	SemanticKind     SemanticKind
}

// Combining returns the cell's stacked zero-width marks, or nil. The backing
// slice is owned by the cell — callers must not mutate it (append via
// AppendCombining, which copies-on-write).
func (c Cell) Combining() []rune {
	if c.combining == nil {
		return nil
	}
	return *c.combining
}

// HasCombining reports whether the cell carries any zero-width marks.
func (c Cell) HasCombining() bool { return c.combining != nil && len(*c.combining) > 0 }

// AppendCombining stacks r onto the cell's marks with copy-on-write semantics:
// it installs a fresh slice behind a fresh pointer, so a value copy of this cell
// (one already captured into scrollback or a render snapshot, sharing the old
// pointer) is never mutated.
func (c *Cell) AppendCombining(r rune) {
	var cur []rune
	if c.combining != nil {
		cur = *c.combining
	}
	next := make([]rune, len(cur)+1)
	copy(next, cur)
	next[len(cur)] = r
	c.combining = &next
}

// CloneCombining returns an independent copy of the cell's marks (nil stays
// nil), for snapshots that must not alias the live backing slice.
func (c Cell) CloneCombining() []rune {
	if c.combining == nil || len(*c.combining) == 0 {
		return nil
	}
	return append([]rune(nil), *c.combining...)
}

// NewCellWithCombining builds a cell carrying the given marks. The only way to
// set combining marks from outside the core package (the field is unexported so
// the accessor seam is compiler-enforced).
func NewCellWithCombining(r rune, attr Attr, marks ...rune) Cell {
	c := Cell{Rune: r, Attr: attr}
	if len(marks) > 0 {
		m := append([]rune(nil), marks...)
		c.combining = &m
	}
	return c
}

const (
	defaultScrollbackRows = 2000
	maxScrollbackRows     = 10_000
)

type Charset int

const (
	CharsetASCII Charset = iota
	CharsetDECSpecial
)

type screenState struct {
	cols, rows              int
	cells                   []Cell
	rowWrapped              []bool
	scrollback              []Cell
	scrollbackWrapped       []bool
	scrollbackStart         int
	scrollbackRows          int
	scrollbackCapacity      int
	displayOffset           int
	cursorRow               int
	cursorCol               int
	wrapNext                bool
	savedCursorRow          int
	savedCursorCol          int
	savedWrapNext           bool
	hasSavedCursor          bool
	scrollTop               int
	scrollBottom            int
	charsets                [2]Charset
	activeCharset           int
	hyperlinks              hyperlinkState
	semanticKind            SemanticKind
	semanticBoundaryPending bool
}

type Terminal struct {
	cols, rows              int
	cells                   []Cell
	rowWrapped              []bool
	scrollback              []Cell
	scrollbackWrapped       []bool
	scrollbackStart         int
	scrollbackRows          int
	scrollbackCapacity      int
	displayOffset           int
	cursorRow               int
	cursorCol               int
	wrapNext                bool
	savedCursorRow          int
	savedCursorCol          int
	savedWrapNext           bool
	hasSavedCursor          bool
	attr                    Attr
	paletteBase             PaletteBase
	paletteOverrides        PaletteOverrides
	title                   string
	cwd                     string
	cwdSeq                  int
	bellCount               int
	notifications           notificationStore
	hyperlinks              hyperlinkState
	semanticKind            SemanticKind
	semanticBoundaryPending bool
	bracketedPaste          bool
	alternateScreen         bool
	primaryScreen           *screenState
	scrollTop               int
	scrollBottom            int
	cursorVisible           bool
	autoWrap                bool
	applicationCursor       bool
	applicationKeypad       bool
	mouseMode               MouseMode
	originMode              bool
	insertMode              bool
	tabStops                []bool
	charsets                [2]Charset
	activeCharset           int
	cursorStyle             CursorStyle
	focusEvents             bool
	imageStore              *termimage.Store
	imageOwner              *termimage.StoreOwner
	imageSidecars           *imageSidecars
}
