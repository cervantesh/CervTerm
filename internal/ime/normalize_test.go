package ime

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestNormalizeUpdateSnapsCursorAndTargetToClusters(t *testing.T) {
	units := utf16.Encode([]rune("e\u0301x"))
	normalized, err := normalizeUpdate(NativeUpdate{
		UTF16:       units,
		CursorUTF16: 1,
		Attributes:  []byte{AttributeInput, AttributeTargetNotConverted, AttributeInput},
	})
	if err != nil {
		t.Fatal(err)
	}
	if normalized.text != "e\u0301x" || normalized.cursorRune != 0 || normalized.target != (Span{Start: 0, End: 2}) {
		t.Fatalf("normalized=%#v", normalized)
	}
}

func TestNormalizeUpdateRejectsDisjointTargetRuns(t *testing.T) {
	_, err := normalizeUpdate(NativeUpdate{
		UTF16:       []uint16{'a', 'b', 'c', 'd', 'e'},
		CursorUTF16: 4,
		Attributes: []byte{
			AttributeTargetConverted, AttributeInput,
			AttributeInput, AttributeTargetNotConverted, AttributeTargetNotConverted,
		},
	})
	if !errors.Is(err, ErrInvalidAttributes) {
		t.Fatalf("err=%v", err)
	}
}

func TestNormalizeUpdateRejectsSplitSurrogateAttributes(t *testing.T) {
	units := utf16.Encode([]rune("A😀B"))
	_, err := normalizeUpdate(NativeUpdate{
		UTF16:       units,
		CursorUTF16: 3,
		Attributes:  []byte{AttributeInput, AttributeInput, AttributeTargetConverted, AttributeInput},
	})
	if !errors.Is(err, ErrInvalidAttributes) {
		t.Fatalf("err=%v", err)
	}
}

func TestNormalizeRejectsMalformedUTF16CursorAndAttributes(t *testing.T) {
	tests := []struct {
		name   string
		update NativeUpdate
		want   error
	}{
		{name: "high surrogate", update: NativeUpdate{UTF16: []uint16{0xD800}}, want: ErrInvalidUTF16},
		{name: "low surrogate", update: NativeUpdate{UTF16: []uint16{0xDC00}}, want: ErrInvalidUTF16},
		{name: "wrong pair", update: NativeUpdate{UTF16: []uint16{0xD800, 'x'}}, want: ErrInvalidUTF16},
		{name: "negative cursor", update: NativeUpdate{UTF16: []uint16{'x'}, CursorUTF16: -1}, want: ErrInvalidCursor},
		{name: "past cursor", update: NativeUpdate{UTF16: []uint16{'x'}, CursorUTF16: 2}, want: ErrInvalidCursor},
		{name: "inside surrogate", update: NativeUpdate{UTF16: utf16.Encode([]rune("😀")), CursorUTF16: 1}, want: ErrInvalidCursor},
		{name: "short attrs", update: NativeUpdate{UTF16: []uint16{'x', 'y'}, CursorUTF16: 1, Attributes: []byte{AttributeInput}}, want: ErrInvalidAttributes},
		{name: "long attrs", update: NativeUpdate{UTF16: []uint16{'x'}, CursorUTF16: 1, Attributes: []byte{AttributeInput, AttributeInput}}, want: ErrInvalidAttributes},
		{name: "unknown attr", update: NativeUpdate{UTF16: []uint16{'x'}, CursorUTF16: 1, Attributes: []byte{0xff}}, want: ErrInvalidAttributes},
		{name: "split surrogate attrs", update: NativeUpdate{UTF16: utf16.Encode([]rune("😀")), CursorUTF16: 2, Attributes: []byte{AttributeInput, AttributeTargetConverted}}, want: ErrInvalidAttributes},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := normalizeUpdate(test.update); !errors.Is(err, test.want) {
				t.Fatalf("err=%v want=%v", err, test.want)
			}
		})
	}
}

func TestNormalizeEnforcesPredecodeAndDecodedBounds(t *testing.T) {
	if _, err := normalizeUpdate(NativeUpdate{UTF16: make([]uint16, MaxPreeditUTF16Units+1)}); !errors.Is(err, ErrPreeditLimit) {
		t.Fatalf("preedit unit bound err=%v", err)
	}
	if _, err := normalizeUpdate(NativeUpdate{UTF16: utf16.Encode([]rune(strings.Repeat("x", MaxPreeditRunes+1)))}); !errors.Is(err, ErrPreeditLimit) {
		t.Fatalf("preedit rune bound err=%v", err)
	}
	if _, err := normalizeUpdate(NativeUpdate{UTF16: utf16.Encode([]rune(strings.Repeat("x", MaxPreeditRunes))), CursorUTF16: MaxPreeditRunes}); err != nil {
		t.Fatalf("exact preedit bound err=%v", err)
	}
	if _, _, err := normalizeCommit(nil); !errors.Is(err, ErrEmptyCommit) {
		t.Fatalf("empty commit err=%v", err)
	}
	if _, _, err := normalizeCommit(make([]uint16, MaxCommitUTF16Units+1)); !errors.Is(err, ErrCommitLimit) {
		t.Fatalf("commit unit bound err=%v", err)
	}
	if _, _, err := normalizeCommit(utf16.Encode([]rune(strings.Repeat("x", MaxCommitBytes)))); err != nil {
		t.Fatalf("exact commit bound err=%v", err)
	}
}

func TestGraphemeBoundariesCoverIMESequences(t *testing.T) {
	for name, text := range map[string]string{
		"hangul jamo": "가",
		"crlf":        "\r\n",
		"indic":       "क्ष",
		"zwj emoji":   "👨‍👩‍👧‍👦",
		"flag":        "🇯🇵",
	} {
		t.Run(name, func(t *testing.T) {
			want := []int{0, len([]rune(text))}
			if got := graphemeBoundaries(text); !reflect.DeepEqual(got, want) {
				t.Fatalf("boundaries=%v want=%v", got, want)
			}
		})
	}
}

func TestDecodeUTF16StrictBoundaryTable(t *testing.T) {
	text, runes, boundaries, err := decodeUTF16Strict(utf16.Encode([]rune("A😀B")))
	if err != nil {
		t.Fatal(err)
	}
	if text != "A😀B" || string(runes) != text {
		t.Fatalf("text=%q runes=%q", text, string(runes))
	}
	want := []int{0, 1, -1, 2, 3}
	if len(boundaries) != len(want) {
		t.Fatalf("boundaries=%v", boundaries)
	}
	for index := range want {
		if boundaries[index] != want[index] {
			t.Fatalf("boundaries=%v want=%v", boundaries, want)
		}
	}
}
