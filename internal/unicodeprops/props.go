package unicodeprops

import "unicode"

const (
	ZeroWidthJoiner          = '\u200d'
	VariationSelectorText    = '\ufe0e'
	VariationSelectorEmoji   = '\ufe0f'
	CombiningEnclosingKeycap = '\u20e3'
	TagCancel                = '\U000E007F'
)

func IsEmoji(r rune) bool { return inRanges(r, emojiRanges) }

func IsEmojiPresentation(r rune) bool { return inRanges(r, emojiPresentationRanges) }

func IsExtendedPictographic(r rune) bool { return inRanges(r, extendedPictographicRanges) }

func IsEmojiModifier(r rune) bool { return inRanges(r, emojiModifierRanges) }

func IsEmojiModifierBase(r rune) bool { return inRanges(r, emojiModifierBaseRanges) }

func IsRegionalIndicator(r rune) bool { return inRanges(r, regionalIndicatorRanges) }

func IsVariationSelector(r rune) bool { return inRanges(r, variationSelectorRanges) }

func IsEmojiVariationSelector(r rune) bool { return r == VariationSelectorEmoji }

func IsTextVariationSelector(r rune) bool { return r == VariationSelectorText }

func IsCombiningEnclosingKeycap(r rune) bool { return r == CombiningEnclosingKeycap }

func IsZeroWidthJoiner(r rune) bool { return r == ZeroWidthJoiner }

func IsTagSpecChar(r rune) bool { return r >= 0xE0020 && r <= 0xE007E }

func IsTagCancel(r rune) bool { return r == TagCancel }

func IsEmojiTag(r rune) bool { return IsTagSpecChar(r) || IsTagCancel(r) }

func IsEmojiControl(r rune) bool {
	return IsZeroWidthJoiner(r) || IsVariationSelector(r) || IsCombiningEnclosingKeycap(r) || IsEmojiModifier(r) || IsEmojiTag(r)
}

func IsEmojiCandidate(r rune) bool {
	return IsEmoji(r) || IsEmojiPresentation(r) || IsExtendedPictographic(r) || IsRegionalIndicator(r)
}

func IsEmojiClusterRune(r rune) bool {
	return IsEmojiCandidate(r) || IsEmojiControl(r)
}

func IsEastAsianWide(r rune) bool {
	return inRanges(r, eastAsianWideRanges) && r != 0x303F
}

func IsMarkOrFormat(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r)
}

func DisplayWidthRune(r rune) int {
	if r == 0 {
		return 0
	}
	if IsMarkOrFormat(r) || IsEmojiModifier(r) || IsVariationSelector(r) || IsCombiningEnclosingKeycap(r) || IsEmojiTag(r) {
		return 0
	}
	if IsRegionalIndicator(r) {
		return 1
	}
	if IsEmojiPresentation(r) || IsExtendedPictographic(r) {
		return 2
	}
	if IsEastAsianWide(r) {
		return 2
	}
	return 1
}

func inRanges(r rune, ranges []runeRange) bool {
	lo, hi := 0, len(ranges)
	for lo < hi {
		mid := lo + (hi-lo)/2
		entry := ranges[mid]
		switch {
		case r < entry.lo:
			hi = mid
		case r > entry.hi:
			lo = mid + 1
		default:
			return true
		}
	}
	return false
}
