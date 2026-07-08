package core

import "testing"

func TestRuneWidth(t *testing.T) {
	tests := []struct {
		r    rune
		want int
	}{
		{r: 'a', want: 1},
		{r: '好', want: 2},
		{r: '\u0301', want: 0},
		{r: '😀', want: 2},
	}
	for _, tt := range tests {
		if got := RuneWidth(tt.r); got != tt.want {
			t.Fatalf("RuneWidth(%q) = %d, want %d", tt.r, got, tt.want)
		}
	}
}
