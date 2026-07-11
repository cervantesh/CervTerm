package core

type Attr struct {
	FG, BG                                RGB
	Bold, Dim, Italic, Underline, Inverse bool
	Strikethrough, Blink                  bool
}

type Cell struct {
	Rune             rune
	Combining        []rune
	Attr             Attr
	WideContinuation bool
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
	workingDirectory  string
}
