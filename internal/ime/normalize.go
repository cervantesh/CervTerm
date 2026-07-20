package ime

import (
	"unicode/utf16"
	"unicode/utf8"

	"github.com/clipperhouse/uax29/v2/graphemes"
)

type normalizedUpdate struct {
	text       string
	runes      []rune
	cursorRune int
	target     Span
}

func normalizeUpdate(update NativeUpdate) (normalizedUpdate, error) {
	if len(update.UTF16) > MaxPreeditUTF16Units {
		return normalizedUpdate{}, ErrPreeditLimit
	}
	if update.CursorUTF16 < 0 || update.CursorUTF16 > len(update.UTF16) {
		return normalizedUpdate{}, ErrInvalidCursor
	}
	if err := validateAttributes(update.UTF16, update.Attributes); err != nil {
		return normalizedUpdate{}, err
	}
	text, runes, boundaries, err := decodeUTF16Strict(update.UTF16)
	if err != nil {
		return normalizedUpdate{}, err
	}
	if len(text) > MaxPreeditBytes || len(runes) > MaxPreeditRunes {
		return normalizedUpdate{}, ErrPreeditLimit
	}
	if boundaries[update.CursorUTF16] < 0 {
		return normalizedUpdate{}, ErrInvalidCursor
	}
	targetStartUnit, targetEndUnit, err := targetUnits(update.Attributes, update.CursorUTF16)
	if err != nil {
		return normalizedUpdate{}, err
	}

	cursorRune := boundaries[update.CursorUTF16]
	targetStartRune, targetEndRune := boundaries[targetStartUnit], boundaries[targetEndUnit]
	if targetStartRune < 0 || targetEndRune < 0 {
		return normalizedUpdate{}, ErrInvalidAttributes
	}
	clusterBoundaries := graphemeBoundaries(text)
	cursorRune = floorBoundary(clusterBoundaries, cursorRune)
	if targetStartUnit == targetEndUnit {
		targetStartRune, targetEndRune = cursorRune, cursorRune
	} else {
		targetStartRune = floorBoundary(clusterBoundaries, targetStartRune)
		targetEndRune = ceilBoundary(clusterBoundaries, targetEndRune)
	}
	return normalizedUpdate{
		text:       text,
		runes:      runes,
		cursorRune: cursorRune,
		target:     Span{Start: targetStartRune, End: targetEndRune},
	}, nil
}

func normalizeCommit(units []uint16) (string, []rune, error) {
	if len(units) > MaxCommitUTF16Units {
		return "", nil, ErrCommitLimit
	}
	text, runes, _, err := decodeUTF16Strict(units)
	if err != nil {
		return "", nil, err
	}
	if text == "" {
		return "", nil, ErrEmptyCommit
	}
	if len(text) > MaxCommitBytes || len(runes) > MaxCommitRunes {
		return "", nil, ErrCommitLimit
	}
	return text, runes, nil
}

// decodeUTF16Strict returns a boundary table indexed by UTF-16 code-unit
// offsets. Values inside a surrogate pair are -1; every valid caret boundary
// maps to its rune offset.
func decodeUTF16Strict(units []uint16) (string, []rune, []int, error) {
	runes := make([]rune, 0, len(units))
	boundaries := make([]int, len(units)+1)
	for index := range boundaries {
		boundaries[index] = -1
	}
	boundaries[0] = 0
	for index := 0; index < len(units); {
		first := rune(units[index])
		switch {
		case first >= 0xD800 && first <= 0xDBFF:
			if index+1 >= len(units) {
				return "", nil, nil, ErrInvalidUTF16
			}
			second := rune(units[index+1])
			if second < 0xDC00 || second > 0xDFFF {
				return "", nil, nil, ErrInvalidUTF16
			}
			runes = append(runes, utf16.DecodeRune(first, second))
			index += 2
			boundaries[index] = len(runes)
		case first >= 0xDC00 && first <= 0xDFFF:
			return "", nil, nil, ErrInvalidUTF16
		default:
			runes = append(runes, first)
			index++
			boundaries[index] = len(runes)
		}
	}
	text := string(runes)
	if !utf8.ValidString(text) {
		return "", nil, nil, ErrInvalidUTF16
	}
	return text, runes, boundaries, nil
}

func validateAttributes(units []uint16, attributes []byte) error {
	if len(attributes) != 0 && len(attributes) != len(units) {
		return ErrInvalidAttributes
	}
	for index, attribute := range attributes {
		if attribute > AttributeFixedConverted {
			return ErrInvalidAttributes
		}
		if index+1 < len(units) && units[index] >= 0xD800 && units[index] <= 0xDBFF && attributes[index+1] != attribute {
			return ErrInvalidAttributes
		}
	}
	return nil
}

func targetUnits(attributes []byte, cursor int) (int, int, error) {
	if len(attributes) == 0 {
		return cursor, cursor, nil
	}
	isTarget := func(attribute byte) bool {
		return attribute == AttributeTargetConverted || attribute == AttributeTargetNotConverted
	}
	start, end, runs := cursor, cursor, 0
	for index := 0; index < len(attributes); {
		if !isTarget(attributes[index]) {
			index++
			continue
		}
		runs++
		if runs > 1 {
			return 0, 0, ErrInvalidAttributes
		}
		start = index
		for index < len(attributes) && isTarget(attributes[index]) {
			index++
		}
		end = index
	}
	return start, end, nil
}

func graphemeBoundaries(text string) []int {
	iterator := graphemes.FromString(text)
	boundaries := []int{0}
	runeOffset := 0
	for iterator.Next() {
		runeOffset += utf8.RuneCountInString(iterator.Value())
		boundaries = append(boundaries, runeOffset)
	}
	return boundaries
}

func floorBoundary(boundaries []int, value int) int {
	result := 0
	for _, boundary := range boundaries {
		if boundary > value {
			break
		}
		result = boundary
	}
	return result
}

func ceilBoundary(boundaries []int, value int) int {
	for _, boundary := range boundaries {
		if boundary >= value {
			return boundary
		}
	}
	return boundaries[len(boundaries)-1]
}
