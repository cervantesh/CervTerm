//go:build !windows

package fontglyph

func newPlatformTextRasterizer(Spec, loadedFace) glyphRasterizer { return nil }
