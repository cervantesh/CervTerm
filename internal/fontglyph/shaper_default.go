//go:build !windows

package fontglyph

import shapepkg "cervterm/internal/fontglyph/shape"

func newDefaultShaper() Shaper {
	return rootShaperFromShape(shapepkg.Default(nil))
}
