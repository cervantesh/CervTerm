package config

import (
	"math"
	"strconv"
)

// BackgroundAlpha returns the configured background alpha. Invalid colors are
// treated as opaque; Validate reports their syntax separately.
func (c Config) BackgroundAlpha() uint8 {
	if len(c.Colors.Background) != 9 {
		return 0xff
	}
	n, err := strconv.ParseUint(c.Colors.Background[7:9], 16, 8)
	if err != nil {
		return 0xff
	}
	return uint8(n)
}

// EffectiveBackgroundAlpha applies the terminal-background opacity multiplier
// once to the configured solid background alpha.
func (c Config) EffectiveBackgroundAlpha() uint8 {
	return uint8(math.Round(float64(c.BackgroundAlpha()) * c.Window.BackgroundOpacity))
}
