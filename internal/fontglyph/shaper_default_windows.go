//go:build windows

package fontglyph

func newDefaultShaper() Shaper {
	if directWriteTextAnalyzerAvailable() {
		return DirectWriteShaper{Fallback: SimpleShaper{}}
	}
	return SimpleShaper{}
}
