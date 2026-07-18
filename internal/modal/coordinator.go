package modal

import (
	"strings"
	"unicode"
)

const (
	MaxEntries    = 512
	MaxQueryRunes = 1024
	MaxErrorRunes = 4096
)

type Mode uint8

const (
	ModeNone Mode = iota
	ModeSearch
	ModeCommandPalette
	ModeQuickSelect
	ModeLaunchMenu
	ModeTabSwitcher
	ModeTabCloseConfirmation
)

func (m Mode) Valid() bool { return m >= ModeSearch && m <= ModeTabCloseConfirmation }

type PaneIdentity uint64
type FocusIdentity uint64

type Entry struct {
	ID       string
	Label    string
	Detail   string
	Category string
}

type State struct {
	Mode         Mode
	OpeningPane  PaneIdentity
	OpeningFocus FocusIdentity
	Query        []rune
	Entries      []Entry
	Filtered     []int
	Selection    int
	Scroll       int
	Error        string
	Revision     uint64
}

type IntentKind uint8

const (
	IntentNone IntentKind = iota
	IntentClose
	IntentAccept
	IntentRestoreFocus
)

type Intent struct {
	Kind  IntentKind
	Pane  PaneIdentity
	Focus FocusIdentity
	Entry Entry
}

type Coordinator struct{ state State }

func (c *Coordinator) Active() bool     { return c.state.Mode.Valid() }
func (c *Coordinator) Mode() Mode       { return c.state.Mode }
func (c *Coordinator) Revision() uint64 { return c.state.Revision }

func (c *Coordinator) Snapshot() State {
	s := c.state
	s.Query = append([]rune(nil), s.Query...)
	s.Entries = append([]Entry(nil), s.Entries...)
	s.Filtered = append([]int(nil), s.Filtered...)
	return s
}

func (c *Coordinator) Open(mode Mode, pane PaneIdentity, focus FocusIdentity, entries []Entry) bool {
	if !mode.Valid() || len(entries) == 0 || len(entries) > MaxEntries {
		return false
	}
	copied := append([]Entry(nil), entries...)
	revision := c.state.Revision + 1
	c.state = State{Mode: mode, OpeningPane: pane, OpeningFocus: focus, Entries: copied, Revision: revision}
	c.refilter()
	return true
}

func (c *Coordinator) Close() []Intent {
	if !c.Active() {
		return nil
	}
	pane, focus, revision := c.state.OpeningPane, c.state.OpeningFocus, c.state.Revision+1
	c.state = State{Revision: revision}
	return []Intent{{Kind: IntentClose}, {Kind: IntentRestoreFocus, Pane: pane, Focus: focus}}
}

func (c *Coordinator) Replace(mode Mode, entries []Entry) bool {
	if !c.Active() {
		return false
	}
	pane, focus := c.state.OpeningPane, c.state.OpeningFocus
	return c.Open(mode, pane, focus, entries)
}

func (c *Coordinator) AppendRune(r rune) bool {
	if !c.Active() || unicode.IsControl(r) || len(c.state.Query) >= MaxQueryRunes {
		return c.Active()
	}
	c.state.Query = append(c.state.Query, r)
	c.refilter()
	c.bump()
	return true
}

func (c *Coordinator) Backspace() bool {
	if !c.Active() {
		return false
	}
	if len(c.state.Query) != 0 {
		c.state.Query = c.state.Query[:len(c.state.Query)-1]
		c.refilter()
		c.bump()
	}
	return true
}

func (c *Coordinator) Move(delta int) bool {
	if !c.Active() {
		return false
	}
	old := c.state.Selection
	last := len(c.state.Filtered) - 1
	if last < 0 {
		c.state.Selection = 0
	} else {
		c.state.Selection = clamp(c.state.Selection+delta, 0, last)
	}
	if c.state.Selection != old {
		c.bump()
	}
	return true
}

func (c *Coordinator) Page(delta, visibleRows int) bool {
	if visibleRows < 1 {
		visibleRows = 1
	}
	return c.Move(delta * visibleRows)
}

func (c *Coordinator) Scroll(delta, visibleRows int) bool {
	if !c.Active() {
		return false
	}
	max := len(c.state.Filtered) - visibleRows
	if max < 0 {
		max = 0
	}
	old := c.state.Scroll
	c.state.Scroll = clamp(c.state.Scroll+delta, 0, max)
	if c.state.Scroll != old {
		c.bump()
	}
	return true
}

func (c *Coordinator) SetError(message string) bool {
	if !c.Active() {
		return false
	}
	r := []rune(message)
	if len(r) > MaxErrorRunes {
		r = r[:MaxErrorRunes]
	}
	message = string(r)
	if c.state.Error != message {
		c.state.Error = message
		c.bump()
	}
	return true
}

func (c *Coordinator) Accept() []Intent {
	if !c.Active() || len(c.state.Filtered) == 0 {
		return nil
	}
	entry := c.state.Entries[c.state.Filtered[c.state.Selection]]
	return []Intent{{Kind: IntentAccept, Pane: c.state.OpeningPane, Focus: c.state.OpeningFocus, Entry: entry}}
}

func (c *Coordinator) bump() { c.state.Revision++ }

func (c *Coordinator) refilter() {
	c.state.Filtered = c.state.Filtered[:0]
	query := strings.ToLower(string(c.state.Query))
	for i := range c.state.Entries {
		entry := c.state.Entries[i]
		if query == "" || strings.Contains(strings.ToLower(entry.Label+"\n"+entry.Detail+"\n"+entry.Category), query) {
			c.state.Filtered = append(c.state.Filtered, i)
		}
	}
	if len(c.state.Filtered) == 0 {
		c.state.Selection, c.state.Scroll = 0, 0
		return
	}
	c.state.Selection = clamp(c.state.Selection, 0, len(c.state.Filtered)-1)
	c.state.Scroll = clamp(c.state.Scroll, 0, c.state.Selection)
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
