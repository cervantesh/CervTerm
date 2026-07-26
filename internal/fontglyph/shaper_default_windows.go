//go:build windows

package fontglyph

import shapepkg "cervterm/internal/fontglyph/shape"

func newDefaultShaper() Shaper {
	return rootShaperFromShape(shapepkg.Default(func(fallback shapepkg.Shaper) shapepkg.Shaper {
		if !directWriteTextAnalyzerAvailable() {
			return nil
		}
		return rootToShapeShaper{root: DirectWriteShaper{Fallback: rootShaperFromShape(fallback)}}
	}))
}
