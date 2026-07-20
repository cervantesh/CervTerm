package quickselect

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"cervterm/internal/mux"
)

const (
	MaxCandidates = 512
	MaxTextBytes  = 4 * 1024
	MaxPattern    = 4 * 1024
	MaxRules      = 32
	MaxIDBytes    = 64
)

var (
	errRuleID  = errors.New("quick select rule ID is empty or exceeds 64 bytes")
	errPattern = errors.New("quick select pattern is empty or exceeds 4 KiB")
	httpRE     = regexp.MustCompile(`https?://[^\s<>"']+`)
)

type Action string

const (
	ActionOpen Action = "open"
	ActionCopy Action = "copy"
)

type PreparedRule struct {
	ID       string
	Priority int
	Action   Action
	re       *regexp.Regexp
}

func PrepareRule(id, pattern string, priority int) (PreparedRule, error) {
	return PrepareRuleWithAction(id, pattern, ActionCopy, priority)
}

func PrepareRuleWithAction(id, pattern string, action Action, priority int) (PreparedRule, error) {
	if id == "" || len(id) > MaxIDBytes {
		return PreparedRule{}, errRuleID
	}
	if pattern == "" || len(pattern) > MaxPattern {
		return PreparedRule{}, errPattern
	}
	if action != ActionOpen && action != ActionCopy {
		return PreparedRule{}, fmt.Errorf("quick select action %q must be open or copy", action)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return PreparedRule{}, err
	}
	return PreparedRule{ID: id, Priority: priority, Action: action, re: re}, nil
}

type Point struct {
	GlobalRow int
	Cell      int
}

type Candidate struct {
	RuleID   string
	Priority int
	Action   Action
	Text     string
	Start    Point
	End      Point // exclusive cell coordinate
	Label    string
}

type token struct {
	byteStart, byteEnd int
	start, end         Point
}

type logicalLine struct {
	text   string
	tokens []token
}

type rawCandidate struct {
	Candidate
	startByte int
	endByte   int
}

// Find scans only the detached bounded snapshot. HTTP(S) is always enabled;
// prepared regular expressions add internal rule sources.
func Find(snapshot mux.QuickSelectSnapshot, rules []PreparedRule) []Candidate {
	var found []rawCandidate
	for _, line := range snapshotLines(snapshot) {
		found = appendMatches(found, line, "builtin:http", 0, ActionOpen, httpRE, true)
		for _, rule := range rules {
			if rule.re != nil {
				found = appendMatches(found, line, rule.ID, rule.Priority, rule.Action, rule.re, false)
			}
		}
	}
	sort.SliceStable(found, func(i, j int) bool { return better(found[i], found[j]) })
	accepted := make([]rawCandidate, 0, min(len(found), MaxCandidates))
	for _, candidate := range found {
		if overlapsAny(candidate, accepted) {
			continue
		}
		accepted = append(accepted, candidate)
		if len(accepted) == MaxCandidates {
			break
		}
	}
	sort.SliceStable(accepted, func(i, j int) bool {
		if accepted[i].Start != accepted[j].Start {
			return pointLess(accepted[i].Start, accepted[j].Start)
		}
		return better(accepted[i], accepted[j])
	})
	labels := Labels(len(accepted))
	out := make([]Candidate, len(accepted))
	for i := range accepted {
		out[i] = accepted[i].Candidate
		out[i].Label = labels[i]
	}
	return out
}

func snapshotLines(snapshot mux.QuickSelectSnapshot) []logicalLine {
	lines := make([]logicalLine, 0, snapshot.Rows)
	current := logicalLine{}
	for row := 0; row < snapshot.Rows; row++ {
		for col := 0; col < snapshot.Cols; col++ {
			cell := snapshot.Cells[row*snapshot.Cols+col]
			if cell.WideContinuation {
				continue
			}
			text := cellText(cell.Rune, cell.Combining())
			if text == "" {
				text = " "
			}
			start := len(current.text)
			current.text += text
			width := 1
			if col+1 < snapshot.Cols && snapshot.Cells[row*snapshot.Cols+col+1].WideContinuation {
				width = 2
			}
			current.tokens = append(current.tokens, token{
				byteStart: start, byteEnd: len(current.text),
				start: Point{snapshot.GlobalRowOrigin + row, col},
				end:   Point{snapshot.GlobalRowOrigin + row, col + width},
			})
		}
		if row >= len(snapshot.Wrapped) || !snapshot.Wrapped[row] {
			lines = append(lines, current)
			current = logicalLine{}
		}
	}
	if current.text != "" {
		lines = append(lines, current)
	}
	return lines
}

func appendMatches(dst []rawCandidate, line logicalLine, id string, priority int, action Action, re *regexp.Regexp, trimHTTP bool) []rawCandidate {
	for _, match := range re.FindAllStringIndex(line.text, -1) {
		start, end := match[0], match[1]
		if trimHTTP {
			end = trimURL(line.text, start, end)
		}
		first, last, ok := coveringTokens(line.tokens, start, end)
		if !ok {
			continue
		}
		start, end = first.byteStart, last.byteEnd
		text := line.text[start:end]
		if text == "" || len(text) > MaxTextBytes || !utf8.ValidString(text) {
			continue
		}
		dst = append(dst, rawCandidate{Candidate: Candidate{
			RuleID: id, Priority: priority, Action: action, Text: text, Start: first.start, End: last.end,
		}, startByte: start, endByte: end})
	}
	return dst
}

func coveringTokens(tokens []token, start, end int) (token, token, bool) {
	if start >= end {
		return token{}, token{}, false
	}
	first, last := -1, -1
	for i, tok := range tokens {
		if tok.byteEnd > start && tok.byteStart < end {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return token{}, token{}, false
	}
	return tokens[first], tokens[last], true
}

func better(a, b rawCandidate) bool {
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	if len(a.Text) != len(b.Text) {
		return len(a.Text) > len(b.Text)
	}
	if a.Start != b.Start {
		return pointLess(a.Start, b.Start)
	}
	if a.RuleID != b.RuleID {
		return a.RuleID < b.RuleID
	}
	return a.Text < b.Text
}

func overlapsAny(candidate rawCandidate, accepted []rawCandidate) bool {
	for _, other := range accepted {
		if candidate.Start == other.Start && candidate.End == other.End && candidate.Text == other.Text {
			return true
		}
		if pointLess(candidate.Start, other.End) && pointLess(other.Start, candidate.End) {
			return true
		}
	}
	return false
}

func pointLess(a, b Point) bool {
	return a.GlobalRow < b.GlobalRow || (a.GlobalRow == b.GlobalRow && a.Cell < b.Cell)
}

func cellText(base rune, combining []rune) string {
	var b strings.Builder
	if base != 0 {
		b.WriteRune(base)
	}
	for _, r := range combining {
		b.WriteRune(r)
	}
	return b.String()
}

func trimURL(text string, start, end int) int {
	for end > start && strings.ContainsRune(".,;:!?", rune(text[end-1])) {
		end--
	}
	return end
}
