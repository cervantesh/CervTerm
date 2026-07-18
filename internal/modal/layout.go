package modal

import (
	"unicode"

	"cervterm/internal/unicodeprops"
)

const MaxDrawCommands = MaxEntries + 3

type RowKind uint8

const (
	RowPrompt RowKind = iota
	RowEntry
	RowHelp
	RowError
)

type DrawCommand struct {
	Kind     RowKind
	Row      int
	Text     string
	Selected bool
	Entry    int
}

type LayoutGeometry struct {
	Columns     int
	Rows        int
	VisibleRows int
}

type Layout struct {
	Commands []DrawCommand
	Scroll   int
}

func ListLayout(state State, geometry LayoutGeometry) Layout {
	if !state.Mode.Valid() || geometry.Columns <= 0 || geometry.Rows <= 0 {
		return Layout{}
	}
	visible := geometry.VisibleRows
	if visible <= 0 || visible > geometry.Rows-2 {
		visible = geometry.Rows - 2
	}
	if visible < 0 {
		visible = 0
	}
	maxScroll := len(state.Filtered) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := clamp(state.Scroll, 0, maxScroll)
	if state.Selection < scroll {
		scroll = state.Selection
	} else if visible > 0 && state.Selection >= scroll+visible {
		scroll = state.Selection - visible + 1
	}
	commands := make([]DrawCommand, 0, min(MaxDrawCommands, visible+2))
	commands = append(commands, DrawCommand{Kind: RowPrompt, Text: ClipCells("> "+string(state.Query), geometry.Columns)})
	end := min(len(state.Filtered), scroll+visible)
	for row, filtered := 1, scroll; filtered < end && len(commands) < MaxDrawCommands-1; row, filtered = row+1, filtered+1 {
		index := state.Filtered[filtered]
		commands = append(commands, DrawCommand{Kind: RowEntry, Row: row, Text: ClipCells(state.Entries[index].Label, geometry.Columns), Selected: filtered == state.Selection, Entry: index})
	}
	footer, kind := "Esc close  Enter select", RowHelp
	if state.Error != "" {
		footer, kind = state.Error, RowError
	}
	if geometry.Rows > 1 && len(commands) < MaxDrawCommands {
		commands = append(commands, DrawCommand{Kind: kind, Row: geometry.Rows - 1, Text: ClipCells(footer, geometry.Columns)})
	}
	return Layout{Commands: commands, Scroll: scroll}
}

func ClipCells(text string, columns int) string {
	if columns <= 0 {
		return ""
	}
	runes := []rune(text)
	used, end := 0, 0
	for start := 0; start < len(runes); {
		next := clusterEnd(runes, start)
		width := clusterWidth(runes[start:next])
		if used+width > columns {
			break
		}
		used += width
		end = next
		start = next
	}
	return string(runes[:end])
}

func CellWidth(text string) int {
	runes, width := []rune(text), 0
	for start := 0; start < len(runes); {
		next := clusterEnd(runes, start)
		width += clusterWidth(runes[start:next])
		start = next
	}
	return width
}

func clusterEnd(runes []rune, start int) int {
	end := start + 1
	regional := unicodeprops.IsRegionalIndicator(runes[start])
	for end < len(runes) {
		r := runes[end]
		if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicodeprops.IsEmojiControl(r) {
			end++
			if r == unicodeprops.ZeroWidthJoiner && end < len(runes) {
				end++
			}
			continue
		}
		if regional && end == start+1 && unicodeprops.IsRegionalIndicator(r) {
			end++
		}
		break
	}
	return end
}

func clusterWidth(cluster []rune) int {
	width := 0
	for _, r := range cluster {
		if w := unicodeprops.DisplayWidthRune(r); w > width {
			width = w
		}
	}
	return width
}

type DamageSnapshot struct {
	Mode     Mode
	Revision uint64
	Geometry LayoutGeometry
}

func SnapshotDamage(state State, geometry LayoutGeometry) DamageSnapshot {
	return DamageSnapshot{Mode: state.Mode, Revision: state.Revision, Geometry: geometry}
}

func (d DamageSnapshot) Changed(next DamageSnapshot) bool { return d != next }
