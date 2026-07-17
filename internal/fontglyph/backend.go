package fontglyph

import (
	"image"
	"image/color"
	"image/draw"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"cervterm/internal/unicodecluster"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/text/unicode/norm"
)

// Spec describes the font input for a glyph backend. It intentionally avoids
// terminal- or renderer-specific concerns so this package can evolve toward a
// reusable GoGPU-compatible color glyph backend.
type Spec struct {
	Family     string
	Size       float64
	DPI        float64
	TextRaster string
}

// RasterizedGlyph is the renderer-neutral output of a font backend.
//
// CervTerm consumes Image/CellSpan/HasColor to upload GL textures, while a
// future GoGPU contribution can use the same shape for CPU/GPU text pipelines.
type RasterizedGlyph struct {
	Image    *image.RGBA
	Width    int
	Height   int
	BearingX int
	BearingY int
	AdvanceX float64
	CellSpan int
	HasColor bool
	Subpixel bool
}

// GlyphInspection reports which font path and raster path CervTerm uses for a cluster.
// It is intended for coverage tooling and tests; rendering should consume RasterizedGlyph directly.
type GlyphInspection struct {
	FaceSource string
	Rasterized bool
	HasVisible bool
	HasColor   bool
	CellSpan   int
	Width      int
	Height     int
}

// Backend rasterizes Unicode codepoints and pre-shaped text clusters into metric-bearing glyph images.
// Backends are owned by the frontend render thread; Rasterize, inspection, and Close must not be called concurrently.
type Backend interface {
	CellMetrics() (width int, height int, baseline int)
	Rasterize(r rune, cellSpan int) (RasterizedGlyph, bool)
	RasterizeCluster(cluster string, cellSpan int) (RasterizedGlyph, bool)
	Close()
}

type glyphRasterizer interface {
	RasterizeGlyph(glyphID uint16, cellW, cellH, baseline, cellSpan int, advancePx float32) (*image.RGBA, bool)
	Close()
}

type OpenTypeBackend struct {
	faces           []loadedFace
	fallbackSpec    Spec // spec for lazily-loaded fallback faces
	fallbacksLoaded bool
	closed          bool // main-thread lifecycle flag
	cellW           int
	cellH           int
	baseline        int
	ppem            uint16
	shaper          Shaper
	dwRaster        glyphRasterizer
	subpixelText    bool
	closeOnce       sync.Once
}

type loadedFace struct {
	face        font.Face
	sfnt        *sfnt.Font
	tables      ColorTables
	sbix        *sbixExtractor
	cbdt        *cbdtExtractor
	colr        *colrParser
	svg         *svgExtractor
	sourcePath  string
	faceIndex   int
	cacheHandle *parsedFontHandle
}

func NewOpenTypeBackend(spec Spec) (*OpenTypeBackend, error) {
	primary, metrics, err := primaryFace(spec)
	if err != nil {
		return nil, err
	}
	// Fallback faces (CJK, and the ~24 MB color-emoji font) are loaded lazily on
	// the first glyph the primary font can't cover — most sessions never render
	// one, so they never pay the memory. See ensureFallbacks.
	faces := []loadedFace{primary}
	// Cell width is the font's natural monospace advance. Padding it (the old
	// +2) letter-spaced every glyph and made text look loose next to other
	// terminals; monospace glyphs are designed to tile at exactly the advance.
	cellW := max(1, font.MeasureString(primary.face, "W").Ceil())
	cellH := max(1, (metrics.Ascent+metrics.Descent).Ceil()+2)
	baseline := 1 + metrics.Ascent.Ceil()
	ppem := uint16(max(1, int(math.Round(spec.Size*spec.DPI/72))))
	backend := &OpenTypeBackend{faces: faces, fallbackSpec: spec, cellW: cellW, cellH: cellH, baseline: baseline, ppem: ppem, shaper: newDefaultShaper(), subpixelText: spec.TextRaster == "subpixel"}
	if !backend.subpixelText {
		backend.dwRaster = newPlatformTextRasterizer(spec, primary)
	}
	return backend, nil
}

func embeddedGoMono(spec Spec) (loadedFace, font.Metrics, error) {
	return loadCachedFaceIndexKnownSize("embedded:gomono", 0, int64(len(gomono.TTF)), spec, func() ([]byte, error) { return gomono.TTF, nil })
}

func primaryFace(spec Spec) (loadedFace, font.Metrics, error) {
	if isEmbeddedFamily(spec.Family) {
		return embeddedGoMono(spec)
	}
	resolution := ResolveSystemFont(spec.Family)
	if !resolution.Found {
		log.Printf("configured font family %q not found; falling back to Go Mono", spec.Family)
		return embeddedGoMono(spec)
	}
	log.Printf("configured font family %q resolved to %s face %d", spec.Family, resolution.Regular, resolution.FaceIndex)
	face, metrics, err := loadCachedFileFaceIndex(resolution.Regular, resolution.FaceIndex, spec)
	if err != nil {
		log.Printf("configured font family %q could not be loaded/parsed from %s: %v; falling back to Go Mono", spec.Family, resolution.Regular, err)
		return embeddedGoMono(spec)
	}
	face.sourcePath = resolution.Regular
	return face, metrics, nil
}

func loadOpenTypeFace(data []byte, spec Spec) (loadedFace, font.Metrics, error) {
	return loadOpenTypeFaceIndex(data, 0, spec)
}

// loadOpenTypeFaceIndex parses data and builds a face without caching. Used by
// direct callers (and tests) that already hold the font bytes; the cached path
// used for zoom lives in loadCachedFaceIndex.
func loadOpenTypeFaceIndex(data []byte, index int, spec Spec) (loadedFace, font.Metrics, error) {
	pf, err := parseFontData(data, index)
	if err != nil {
		return loadedFace{}, font.Metrics{}, err
	}
	lf, metrics, err := faceFromParsed(pf, spec)
	if err != nil {
		return loadedFace{}, font.Metrics{}, err
	}
	lf.faceIndex = index
	return lf, metrics, nil
}

func (b *OpenTypeBackend) CellMetrics() (width int, height int, baseline int) {
	return b.cellW, b.cellH, b.baseline
}

func (b *OpenTypeBackend) Close() {
	if b == nil {
		return
	}
	b.closeOnce.Do(func() {
		b.closed = true
		for i := range b.faces {
			if closer, ok := b.faces[i].face.(interface{ Close() error }); ok {
				_ = closer.Close()
			}
			b.faces[i].face = nil
			b.faces[i].cacheHandle.release()
			b.faces[i].cacheHandle = nil
		}
		if b.dwRaster != nil {
			b.dwRaster.Close()
			b.dwRaster = nil
		}
	})
}

func (b *OpenTypeBackend) TextRasterEngine() string {
	if b != nil && b.subpixelText {
		return "subpixel"
	}
	if b != nil && b.dwRaster != nil {
		return "directwrite"
	}
	return "go"
}

func (b *OpenTypeBackend) Rasterize(r rune, cellSpan int) (RasterizedGlyph, bool) {
	if b == nil || b.closed || r == 0 || r < 32 {
		return RasterizedGlyph{}, false
	}
	lf, bounds, advance, ok := b.faceForRune(r)
	if !ok {
		return RasterizedGlyph{}, false
	}
	cellSpan = max(1, cellSpan)
	if glyph, ok := b.rasterizeBitmapColorGlyph(lf, r, cellSpan, advance); ok {
		return glyph, true
	}
	if glyph, ok := b.rasterizeCOLRGlyph(lf, r, cellSpan, bounds, advance); ok {
		return glyph, true
	}
	if glyph, ok := b.rasterizeSVGColorGlyph(lf, r, cellSpan, advance); ok {
		return glyph, true
	}
	var sfntBuf sfnt.Buffer
	glyphID, glyphIndexErr := lf.sfnt.GlyphIndex(&sfntBuf, r)
	if b.subpixelText && glyphIndexErr == nil && glyphID != 0 {
		if img, ok := b.rasterizeSubpixel(lf, glyphID, bounds, cellSpan); ok {
			return RasterizedGlyph{
				Image: img, Width: (bounds.Max.X - bounds.Min.X).Ceil(), Height: (bounds.Max.Y - bounds.Min.Y).Ceil(),
				BearingX: bounds.Min.X.Ceil(), BearingY: -bounds.Min.Y.Ceil(), AdvanceX: float64(advance) / 64.0,
				CellSpan: cellSpan, HasColor: false, Subpixel: true,
			}, true
		}
	}
	if b.dwRaster != nil && lf.sfnt == b.faces[0].sfnt {
		if glyphIndexErr == nil && glyphID != 0 {
			if img, ok := b.dwRaster.RasterizeGlyph(uint16(glyphID), b.cellW, b.cellH, b.baseline, cellSpan, float32(advance)/64); ok {
				return RasterizedGlyph{
					Image: img, Width: (bounds.Max.X - bounds.Min.X).Ceil(), Height: (bounds.Max.Y - bounds.Min.Y).Ceil(),
					BearingX: bounds.Min.X.Ceil(), BearingY: -bounds.Min.Y.Ceil(), AdvanceX: float64(advance) / 64.0,
					CellSpan: cellSpan, HasColor: false,
				}, true
			}
		}
	}

	img := image.NewRGBA(image.Rect(0, 0, b.cellW*cellSpan, b.cellH))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)

	// Center the glyph's advance box within the cell instead of jamming its ink
	// to the left edge. cellW is the font's monospace advance, so for narrow,
	// font-centered glyphs (period, colon, comma, quotes) this restores their
	// designed position; the old left-align (fixed.I(1) - bounds.Min.X) left a
	// near-full-cell gap after them that read as an extra space.
	dotX := (fixed.I(b.cellW*cellSpan) - advance) / 2
	dotY := fixed.I(b.baseline)
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.RGBA{255, 255, 255, 255}),
		Face: lf.face,
		Dot:  fixed.Point26_6{X: dotX, Y: dotY},
	}
	d.DrawString(string(r))

	return RasterizedGlyph{
		Image:    img,
		Width:    (bounds.Max.X - bounds.Min.X).Ceil(),
		Height:   (bounds.Max.Y - bounds.Min.Y).Ceil(),
		BearingX: bounds.Min.X.Ceil(),
		BearingY: -bounds.Min.Y.Ceil(),
		AdvanceX: float64(advance) / 64.0,
		CellSpan: cellSpan,
		HasColor: false,
	}, true
}

func (b *OpenTypeBackend) RasterizeCluster(cluster string, cellSpan int) (RasterizedGlyph, bool) {
	if b == nil || b.closed {
		return RasterizedGlyph{}, false
	}
	glyph, _, ok := b.rasterizeCluster(cluster, cellSpan)
	return glyph, ok
}

func (b *OpenTypeBackend) rasterizeCluster(cluster string, cellSpan int) (RasterizedGlyph, loadedFace, bool) {
	if b == nil || b.closed || cluster == "" {
		return RasterizedGlyph{}, loadedFace{}, false
	}
	if composed, ok := normalizeClusterToSingleRune(cluster); ok {
		if glyph, ok := b.Rasterize(composed, max(1, cellSpan)); ok {
			face, _, _, _ := b.faceForRune(composed)
			return glyph, face, true
		}
	}

	candidates := b.clusterFaceCandidates(cluster)
	var visibleFallback RasterizedGlyph
	var visibleFallbackFace loadedFace
	visibleFallbackOK := false
	for _, lf := range candidates {
		glyph, ok := b.rasterizeClusterWithFace(cluster, cellSpan, lf)
		if !ok {
			continue
		}
		if !unicodecluster.IsEmojiString(cluster) || glyph.HasColor {
			return glyph, lf, true
		}
		if !visibleFallbackOK && glyph.Image != nil && hasVisibleRGBA(glyph.Image) {
			visibleFallback = glyph
			visibleFallbackFace = lf
			visibleFallbackOK = true
		}
	}
	if visibleFallbackOK {
		return visibleFallback, visibleFallbackFace, true
	}
	return RasterizedGlyph{}, loadedFace{}, false
}

func (b *OpenTypeBackend) rasterizeClusterWithFace(cluster string, cellSpan int, lf loadedFace) (RasterizedGlyph, bool) {
	if b.shaper != nil && lf.sfnt != nil {
		if shaped, ok := b.shaper.Shape(cluster, lf, b.ppem); ok {
			if glyph, ok := b.rasterizeShapedCluster(lf, shaped, max(1, cellSpan)); ok {
				return glyph, true
			}
		}
	}
	cellSpan = max(1, cellSpan)
	img := image.NewRGBA(image.Rect(0, 0, b.cellW*cellSpan, b.cellH))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)

	dot := fixed.Point26_6{X: fixed.I(1), Y: fixed.I(b.baseline)}
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.RGBA{255, 255, 255, 255}),
		Face: lf.face,
		Dot:  dot,
	}
	d.DrawString(cluster)
	advance := d.MeasureString(cluster)

	return RasterizedGlyph{
		Image:    img,
		Width:    max(1, advance.Ceil()),
		Height:   b.cellH,
		BearingX: 1,
		BearingY: b.baseline,
		AdvanceX: float64(advance) / 64.0,
		CellSpan: cellSpan,
		HasColor: false,
	}, true
}

func (b *OpenTypeBackend) InspectClusterGlyph(cluster string, cellSpan int) GlyphInspection {
	info := GlyphInspection{}
	if b == nil || b.closed {
		return info
	}
	glyph, face, ok := b.rasterizeCluster(cluster, cellSpan)
	if !ok {
		return info
	}
	info.FaceSource = face.sourcePath
	info.Rasterized = true
	info.HasVisible = glyph.Image != nil && hasVisibleRGBA(glyph.Image)
	info.HasColor = glyph.HasColor
	info.CellSpan = glyph.CellSpan
	info.Width = glyph.Width
	info.Height = glyph.Height
	return info
}

func normalizeClusterToSingleRune(cluster string) (rune, bool) {
	normalized := norm.NFC.String(cluster)
	var out rune
	count := 0
	for _, r := range normalized {
		out = r
		count++
		if count > 1 {
			return 0, false
		}
	}
	if count != 1 {
		return 0, false
	}
	return out, normalized != cluster
}

func (b *OpenTypeBackend) rasterizeBitmapColorGlyph(lf loadedFace, r rune, cellSpan int, advance fixed.Int26_6) (RasterizedGlyph, bool) {
	if lf.sfnt == nil {
		return RasterizedGlyph{}, false
	}
	var buf sfnt.Buffer
	glyphID, err := lf.sfnt.GlyphIndex(&buf, r)
	if err != nil || glyphID == 0 {
		return RasterizedGlyph{}, false
	}
	bitmap, ok := bitmapColorGlyph(lf, uint16(glyphID), b.ppem)
	if !ok {
		return RasterizedGlyph{}, false
	}
	canvasW := b.cellW * cellSpan
	canvasH := b.cellH
	img := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)

	srcBounds := bitmap.Image.Bounds()
	if srcBounds.Dx() <= 0 || srcBounds.Dy() <= 0 {
		return RasterizedGlyph{}, false
	}
	scale := math.Min(float64(canvasW)/float64(srcBounds.Dx()), float64(canvasH)/float64(srcBounds.Dy()))
	if scale <= 0 {
		return RasterizedGlyph{}, false
	}
	dstW := max(1, int(math.Round(float64(srcBounds.Dx())*scale)))
	dstH := max(1, int(math.Round(float64(srcBounds.Dy())*scale)))
	dstX := (canvasW - dstW) / 2
	dstY := b.baseline - int(math.Round(float64(bitmap.OriginOffsetY)*scale))
	if dstY < 0 || dstY+dstH > canvasH {
		dstY = (canvasH - dstH) / 2
	}
	dst := image.Rect(dstX, dstY, dstX+dstW, dstY+dstH)
	xdraw.CatmullRom.Scale(img, dst, bitmap.Image, srcBounds, xdraw.Over, nil)

	return RasterizedGlyph{
		Image:    img,
		Width:    dstW,
		Height:   dstH,
		BearingX: dstX,
		BearingY: canvasH - dstY,
		AdvanceX: float64(advance) / 64.0,
		CellSpan: cellSpan,
		HasColor: true,
	}, true
}

func (b *OpenTypeBackend) faceForRune(r rune) (loadedFace, fixed.Rectangle26_6, fixed.Int26_6, bool) {
	if b == nil || b.closed {
		return loadedFace{}, fixed.Rectangle26_6{}, 0, false
	}
	if face, bounds, advance, ok := b.searchRune(r); ok {
		return face, bounds, advance, true
	}
	// The primary (and any already-loaded fallback) couldn't cover r; load the
	// fallback fonts now and retry once.
	if !b.fallbacksLoaded {
		b.ensureFallbacks()
		return b.searchRune(r)
	}
	return loadedFace{}, fixed.Rectangle26_6{}, 0, false
}

func (b *OpenTypeBackend) searchRune(r rune) (loadedFace, fixed.Rectangle26_6, fixed.Int26_6, bool) {
	for _, face := range b.faces {
		bounds, advance, ok := face.face.GlyphBounds(r)
		if ok {
			return face, bounds, advance, true
		}
		if b.faceHasColorGlyph(face, r) {
			return face, fixed.Rectangle26_6{
				Min: fixed.Point26_6{X: 0, Y: -fixed.I(b.cellH)},
				Max: fixed.Point26_6{X: fixed.I(b.cellW * 2), Y: 0},
			}, fixed.I(b.cellW * 2), true
		}
	}
	return loadedFace{}, fixed.Rectangle26_6{}, 0, false
}

// ensureFallbacks appends the fallback faces to b.faces on first use. Idempotent
// and main-thread only (glyph lookup runs during draw prep on the loop thread).
func (b *OpenTypeBackend) ensureFallbacks() {
	if b == nil || b.closed || b.fallbacksLoaded {
		return
	}
	b.fallbacksLoaded = true
	b.faces = append(b.faces, loadFallbackFaces(b.fallbackSpec)...)
}

func (b *OpenTypeBackend) faceHasColorGlyph(face loadedFace, r rune) bool {
	if face.sfnt == nil || !face.tables.HasAnyColor() {
		return false
	}
	var buf sfnt.Buffer
	glyphID, err := face.sfnt.GlyphIndex(&buf, r)
	if err != nil || glyphID == 0 {
		return false
	}
	if _, ok := bitmapColorGlyph(face, uint16(glyphID), b.ppem); ok {
		return true
	}
	return face.colr != nil || face.svg != nil
}
func (b *OpenTypeBackend) faceForCluster(cluster string) (loadedFace, bool) {
	if b == nil || b.closed {
		return loadedFace{}, false
	}
	for _, face := range b.clusterFaceCandidates(cluster) {
		return face, true
	}
	for _, r := range cluster {
		if r == 0 || r < 32 || unicodecluster.IsZeroWidthClusterRune(r) {
			continue
		}
		lf, _, _, ok := b.faceForRune(r)
		return lf, ok
	}
	return loadedFace{}, false
}

func (b *OpenTypeBackend) clusterFaceCandidates(cluster string) []loadedFace {
	if b == nil || b.closed {
		return nil
	}
	var out []loadedFace
	appendFace := func(face loadedFace, ok bool) {
		if !ok {
			return
		}
		key := fontCacheKey(face.sourcePath, face.faceIndex)
		for _, existing := range out {
			if fontCacheKey(existing.sourcePath, existing.faceIndex) == key {
				return
			}
		}
		out = append(out, face)
	}
	if unicodecluster.IsEmojiString(cluster) {
		if unicodecluster.IsFlagString(cluster) {
			appendFace(b.notoColorEmojiFace())
			appendFace(b.segoeEmojiFace())
		} else {
			appendFace(b.segoeEmojiFace())
			appendFace(b.notoColorEmojiFace())
		}
	}
	if len(out) == 0 {
		if face, ok := firstTextFaceForCluster(cluster, b.faceForRune); ok {
			out = append(out, face)
		}
	}
	return out
}

func firstTextFaceForCluster(cluster string, faceForRune func(rune) (loadedFace, fixed.Rectangle26_6, fixed.Int26_6, bool)) (loadedFace, bool) {
	for _, r := range cluster {
		if r == 0 || r < 32 || unicodecluster.IsZeroWidthClusterRune(r) {
			continue
		}
		lf, _, _, ok := faceForRune(r)
		return lf, ok
	}
	return loadedFace{}, false
}

func (b *OpenTypeBackend) emojiFaceForCluster(cluster string) (loadedFace, bool) {
	if unicodecluster.IsFlagString(cluster) {
		if face, ok := b.notoColorEmojiFace(); ok {
			return face, true
		}
		return b.segoeEmojiFace()
	}
	if face, ok := b.segoeEmojiFace(); ok {
		return face, true
	}
	return b.notoColorEmojiFace()
}

func (b *OpenTypeBackend) preferredEmojiFace() (loadedFace, bool) {
	return b.emojiFaceForCluster("😀")
}

func (b *OpenTypeBackend) notoColorEmojiFace() (loadedFace, bool) {
	b.ensureFallbacks() // emoji lives in a fallback font
	for _, face := range b.faces {
		if isNotoColorEmojiFace(face) {
			return face, true
		}
	}
	return loadedFace{}, false
}

func (b *OpenTypeBackend) segoeEmojiFace() (loadedFace, bool) {
	b.ensureFallbacks()
	for _, face := range b.faces {
		if isSegoeEmojiFace(face) {
			return face, true
		}
	}
	return loadedFace{}, false
}

func isSegoeEmojiFace(face loadedFace) bool {
	return strings.EqualFold(filepath.Base(face.sourcePath), "seguiemj.ttf")
}

func isNotoColorEmojiFace(face loadedFace) bool {
	base := strings.ToLower(filepath.Base(face.sourcePath))
	return base == "notocoloremoji.ttf" || base == "noto-color-emoji.ttf"
}

type EmojiFontDiagnostics struct {
	NotoColorEmojiPath string
	SegoeEmojiPath     string
	Warnings           []string
}

func DiagnoseEmojiFonts() EmojiFontDiagnostics {
	diag := EmojiFontDiagnostics{}
	for _, path := range fallbackFontPaths() {
		baseFace := loadedFace{sourcePath: path}
		if diag.NotoColorEmojiPath == "" && isNotoColorEmojiFace(baseFace) {
			if _, err := os.Stat(path); err == nil {
				diag.NotoColorEmojiPath = path
			}
		}
		if diag.SegoeEmojiPath == "" && isSegoeEmojiFace(baseFace) {
			if _, err := os.Stat(path); err == nil {
				diag.SegoeEmojiPath = path
			}
		}
	}
	if diag.NotoColorEmojiPath == "" {
		diag.Warnings = append(diag.Warnings, "NotoColorEmoji.ttf not found; country and subdivision flags may render as regional letters or fallback glyphs")
	}
	if diag.SegoeEmojiPath == "" {
		diag.Warnings = append(diag.Warnings, "Segoe UI Emoji not found; Windows emoji ZWJ/person sequences may have degraded rendering")
	}
	return diag
}

func loadFallbackFaces(spec Spec) []loadedFace {
	var faces []loadedFace
	for _, path := range fallbackFontPaths() {
		face, _, err := loadCachedFileFaceIndex(path, 0, spec)
		if err != nil {
			continue
		}
		face.sourcePath = path
		faces = append(faces, face)
	}
	return faces
}

func bitmapColorGlyph(lf loadedFace, glyphID uint16, ppem uint16) (bitmapGlyph, bool) {
	if lf.sbix != nil {
		if glyph, ok := lf.sbix.glyph(glyphID, ppem); ok {
			return glyph, true
		}
	}
	if lf.cbdt != nil {
		if glyph, ok := lf.cbdt.glyph(glyphID, ppem); ok {
			return glyph, true
		}
	}
	return bitmapGlyph{}, false
}

func fallbackFontPaths() []string {
	fontDir := filepath.Join(os.Getenv("SystemRoot"), "Fonts")
	if fontDir == "Fonts" {
		fontDir = `C:\Windows\Fonts`
	}
	paths := []string{
		filepath.Join("dist", "font-sources", "NotoColorEmoji.ttf"),
		filepath.Join("font-sources", "NotoColorEmoji.ttf"),
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		paths = append(paths,
			filepath.Join(exeDir, "font-sources", "NotoColorEmoji.ttf"),
			filepath.Join(exeDir, "..", "font-sources", "NotoColorEmoji.ttf"),
		)
	}
	paths = append(paths,
		filepath.Join(fontDir, "NotoColorEmoji.ttf"),
		filepath.Join(fontDir, "seguiemj.ttf"),
		filepath.Join(fontDir, "seguisym.ttf"),
		filepath.Join(fontDir, "consola.ttf"),
		filepath.Join(fontDir, "malgun.ttf"),
		filepath.Join(fontDir, "simsunb.ttf"),
		filepath.Join(fontDir, "SimsunExtG.ttf"),
		filepath.Join(fontDir, "arial.ttf"),
		filepath.Join(fontDir, "segoeui.ttf"),
		filepath.Join(fontDir, "LeelawUI.ttf"),
		filepath.Join(fontDir, "NotoSans-Regular.ttf"),
		"/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf",
		"/usr/share/fonts/google-noto-emoji/NotoColorEmoji.ttf",
		"/System/Library/Fonts/Apple Color Emoji.ttc",
	)
	return uniqueExistingFontCandidates(paths)
}

func uniqueExistingFontCandidates(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		cleaned := filepath.Clean(path)
		key := strings.ToLower(cleaned)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cleaned)
	}
	return out
}
