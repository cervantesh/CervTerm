//go:build windows

package fontglyph

import (
	"image"

	platformpkg "cervterm/internal/fontglyph/platform"
)

func newPlatformTextRasterizer(spec Spec, primary loadedFace) glyphRasterizer {
	return platformpkg.NewTextRasterizer(platformpkg.TextRasterSpec{
		Mode: spec.TextRaster,
		Source: platformpkg.FaceSource{
			Path:  primary.sourcePath,
			Index: primary.faceIndex,
		},
		SizePoints: spec.Size,
		DPI:        spec.DPI,
	})
}

func cachedGoMonoPath() (string, error) { return platformpkg.CachedGoMonoPath() }

type dwriteRasterizer struct {
	inner platformpkg.GlyphRasterizer
}

func newDWriteRasterizer(fontPath string, faceIndex int, sizePt, dpi float64) (*dwriteRasterizer, error) {
	inner, err := platformpkg.NewDirectWriteRasterizer(platformpkg.FaceSource{Path: fontPath, Index: faceIndex}, sizePt, dpi)
	if err != nil {
		return nil, err
	}
	return &dwriteRasterizer{inner: inner}, nil
}

func (d *dwriteRasterizer) RasterizeGlyph(glyphID uint16, cellW, cellH, baseline, cellSpan int, advancePx float32) (*image.RGBA, bool) {
	if d == nil || d.inner == nil {
		return image.NewRGBA(image.Rect(0, 0, cellW*max(1, cellSpan), cellH)), false
	}
	return d.inner.RasterizeGlyph(glyphID, cellW, cellH, baseline, cellSpan, advancePx)
}

func (d *dwriteRasterizer) Close() {
	if d == nil || d.inner == nil {
		return
	}
	d.inner.Close()
	d.inner = nil
}
