package unicodecluster

import (
	"strings"
	"unicode"

	"cervterm/internal/unicodeprops"
)

type Cluster struct {
	Text    string
	Runes   []rune
	Width   int
	IsEmoji bool
}

func First(s string) (Cluster, bool) {
	runes := []rune(s)
	if len(runes) == 0 {
		return Cluster{}, false
	}
	end := clusterEnd(runes, 0)
	return New(runes[:end]), true
}

func Segment(s string) []Cluster {
	runes := []rune(s)
	clusters := make([]Cluster, 0, len(runes))
	for i := 0; i < len(runes); {
		end := clusterEnd(runes, i)
		clusters = append(clusters, New(runes[i:end]))
		i = end
	}
	return clusters
}

func New(runes []rune) Cluster {
	copied := append([]rune(nil), runes...)
	text := string(copied)
	emoji := IsEmojiRunes(copied)
	return Cluster{Text: text, Runes: copied, Width: DisplayWidthRunes(copied), IsEmoji: emoji}
}

func IsEmojiString(s string) bool { return IsEmojiRunes([]rune(s)) }

func IsEmojiRunes(runes []rune) bool {
	for _, r := range runes {
		if unicodeprops.IsEmojiPresentation(r) || unicodeprops.IsExtendedPictographic(r) || unicodeprops.IsRegionalIndicator(r) || unicodeprops.IsEmojiVariationSelector(r) || unicodeprops.IsEmojiModifier(r) || unicodeprops.IsCombiningEnclosingKeycap(r) || unicodeprops.IsEmojiTag(r) {
			return true
		}
	}
	return false
}

func DisplayWidthString(s string) int { return DisplayWidthRunes([]rune(s)) }

func DisplayWidthRunes(runes []rune) int {
	if len(runes) == 0 {
		return 0
	}
	if IsEmojiRunes(runes) {
		return 2
	}
	width := 0
	for _, r := range runes {
		width += unicodeprops.DisplayWidthRune(r)
	}
	if width < 0 {
		return 0
	}
	return width
}

func ShouldShapeRune(r rune) bool {
	return unicodeprops.IsEmojiCandidate(r) || unicodeprops.IsEmojiControl(r)
}

func ContainsZeroWidthJoiner(runes []rune) bool {
	for _, r := range runes {
		if unicodeprops.IsZeroWidthJoiner(r) {
			return true
		}
	}
	return false
}

func ContainsEmojiVariationSelector(runes []rune) bool {
	for _, r := range runes {
		if unicodeprops.IsEmojiVariationSelector(r) {
			return true
		}
	}
	return false
}

func IsRegionalIndicator(r rune) bool { return unicodeprops.IsRegionalIndicator(r) }

func IsFlagString(s string) bool {
	runes := []rune(s)
	if len(runes) == 2 && unicodeprops.IsRegionalIndicator(runes[0]) && unicodeprops.IsRegionalIndicator(runes[1]) {
		return true
	}
	if len(runes) >= 3 && runes[0] == '\U0001F3F4' && unicodeprops.IsTagCancel(runes[len(runes)-1]) {
		for _, r := range runes[1 : len(runes)-1] {
			if !unicodeprops.IsTagSpecChar(r) {
				return false
			}
		}
		return true
	}
	return false
}

func IsZeroWidthClusterRune(r rune) bool {
	return unicodeprops.IsMarkOrFormat(r) || unicodeprops.IsEmojiModifier(r) || unicodeprops.IsVariationSelector(r) || unicodeprops.IsCombiningEnclosingKeycap(r) || unicodeprops.IsEmojiTag(r)
}

func HasTrailingJoiner(s string) bool {
	return strings.HasSuffix(s, string(unicodeprops.ZeroWidthJoiner))
}

func clusterEnd(runes []rune, start int) int {
	end := start + 1
	if unicodeprops.IsRegionalIndicator(runes[start]) && end < len(runes) && unicodeprops.IsRegionalIndicator(runes[end]) {
		return end + 1
	}
	for end < len(runes) {
		prev := runes[end-1]
		cur := runes[end]
		switch {
		case unicode.Is(unicode.Mn, cur) || unicode.Is(unicode.Me, cur) || unicode.Is(unicode.Mc, cur):
			end++
		case unicodeprops.IsVariationSelector(cur) || unicodeprops.IsEmojiModifier(cur) || unicodeprops.IsCombiningEnclosingKeycap(cur) || unicodeprops.IsEmojiTag(cur):
			end++
		case unicodeprops.IsZeroWidthJoiner(prev):
			end++
		case unicodeprops.IsZeroWidthJoiner(cur):
			end++
		default:
			return end
		}
	}
	return end
}
