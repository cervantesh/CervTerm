package core

type Attr struct {
	FG, BG                                RGB
	Bold, Dim, Italic, Underline, Inverse bool
	Strikethrough, Blink                  bool
}

type Cell struct {
	Rune             rune
	combining        []rune // zero-width marks stacked on Rune; nil for the common case
	Attr             Attr
	WideContinuation bool
}

// Combining returns the cell's stacked zero-width marks, or nil. The backing
// slice is owned by the cell — callers must not mutate it (append via
// AppendCombining, which copies-on-write).
func (c Cell) Combining() []rune { return c.combining }

// HasCombining reports whether the cell carries any zero-width marks.
func (c Cell) HasCombining() bool { return len(c.combining) > 0 }

// AppendCombining stacks r onto the cell's marks with copy-on-write semantics:
// it never grows a shared backing array, so a value copy of this cell (e.g. one
// already captured into scrollback or a render snapshot) is never mutated.
func (c *Cell) AppendCombining(r rune) {
	next := make([]rune, len(c.combining)+1)
	copy(next, c.combining)
	next[len(c.combining)] = r
	c.combining = next
}

// CloneCombining returns an independent copy of the cell's marks (nil stays
// nil), for snapshots that must not alias the live backing slice.
func (c Cell) CloneCombining() []rune {
	if len(c.combining) == 0 {
		return nil
	}
	return append([]rune(nil), c.combining...)
}

// NewCellWithCombining builds a cell carrying the given marks. The only way to
// set combining marks from outside the core package (the field is unexported so
// the accessor seam is compiler-enforced).
func NewCellWithCombining(r rune, attr Attr, marks ...rune) Cell {
	c := Cell{Rune: r, Attr: attr}
	if len(marks) > 0 {
		c.combining = append([]rune(nil), marks...)
	}
	return c
}

const maxScrollbackRows = 2000

type Charset int

const (
	CharsetASCII Charset = iota
	CharsetDECSpecial
)

type screenState struct {
	cols, rows        int
	cells             []Cell
	rowWrapped        []bool
	scrollback        []Cell
	scrollbackWrapped []bool
	scrollbackStart   int
	scrollbackRows    int
	displayOffset     int
	cursorRow         int
	cursorCol         int
	wrapNext          bool
	savedCursorRow    int
	savedCursorCol    int
	savedWrapNext     bool
	hasSavedCursor    bool
	scrollTop         int
	scrollBottom      int
	charsets          [2]Charset
	activeCharset     int
}

type Terminal struct {
	cols, rows        int
	cells             []Cell
	rowWrapped        []bool
	scrollback        []Cell
	scrollbackWrapped []bool
	scrollbackStart   int
	scrollbackRows    int
	displayOffset     int
	cursorRow         int
	cursorCol         int
	wrapNext          bool
	savedCursorRow    int
	savedCursorCol    int
	savedWrapNext     bool
	hasSavedCursor    bool
	attr              Attr
	title             string
	cwd               string
	cwdSeq            int
	bellCount         int
	bracketedPaste    bool
	alternateScreen   bool
	primaryScreen     *screenState
	scrollTop         int
	scrollBottom      int
	cursorVisible     bool
	autoWrap          bool
	applicationCursor bool
	applicationKeypad bool
	mouseMode         MouseMode
	originMode        bool
	insertMode        bool
	tabStops          []bool
	charsets          [2]Charset
	activeCharset     int
	cursorStyle       int
	focusEvents       bool
}
