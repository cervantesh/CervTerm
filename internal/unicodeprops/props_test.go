package unicodeprops

import "testing"

func TestEmojiPropertiesRepresentativeCodepoints(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		fn   func(rune) bool
	}{
		{name: "astral emoji", r: '😀', fn: IsEmoji},
		{name: "BMP emoji", r: '✍', fn: IsEmoji},
		{name: "emoji presentation", r: '☕', fn: IsEmojiPresentation},
		{name: "extended pictographic", r: '❤', fn: IsExtendedPictographic},
		{name: "modifier", r: '\U0001F3FD', fn: IsEmojiModifier},
		{name: "regional indicator", r: '🇦', fn: IsRegionalIndicator},
		{name: "variation selector", r: VariationSelectorEmoji, fn: IsVariationSelector},
		{name: "keycap mark", r: CombiningEnclosingKeycap, fn: IsCombiningEnclosingKeycap},
		{name: "tag letter", r: '\U000E0067', fn: IsEmojiTag},
		{name: "tag cancel", r: TagCancel, fn: IsEmojiTag},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.fn(tt.r) {
				t.Fatalf("property check failed for %U", tt.r)
			}
		})
	}
}

func TestDisplayWidthRuneRepresentativeCodepoints(t *testing.T) {
	tests := []struct {
		r    rune
		want int
	}{
		{r: 'a', want: 1},
		{r: '好', want: 2},
		{r: '\u0301', want: 0},
		{r: '😀', want: 2},
		{r: '✍', want: 2},
		{r: '☕', want: 2},
		{r: '✈', want: 2},
		{r: '⚽', want: 2},
		{r: '⭐', want: 2},
		{r: '❤', want: 2},
		{r: '\U0001F3FD', want: 0},
		{r: '🇦', want: 1},
		{r: VariationSelectorEmoji, want: 0},
		{r: '\U000E0067', want: 0},
		{r: TagCancel, want: 0},
	}
	for _, tt := range tests {
		if got := DisplayWidthRune(tt.r); got != tt.want {
			t.Fatalf("DisplayWidthRune(%U) = %d, want %d", tt.r, got, tt.want)
		}
	}
}
