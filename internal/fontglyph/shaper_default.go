//go:build !windows

package fontglyph

func newDefaultShaper() Shaper {
	return SimpleShaper{}
}
