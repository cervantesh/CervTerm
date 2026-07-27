//go:build windows

package fontglyph

import (
	platformpkg "cervterm/internal/fontglyph/platform"
	shapepkg "cervterm/internal/fontglyph/shape"
)

func newDefaultShaper() Shaper {
	return rootShaperFromShape(shapepkg.Default(func(fallback shapepkg.Shaper) shapepkg.Shaper {
		if !platformpkg.Available() {
			return nil
		}
		return rootToShapeShaper{root: DirectWriteShaper{Fallback: rootShaperFromShape(fallback)}}
	}))
}
