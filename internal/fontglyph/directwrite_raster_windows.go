//go:build windows

package fontglyph

import (
	"fmt"
	"image"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/image/font/gofont/gomono"
)

const (
	dwriteRenderingModeNaturalSymmetric = 5
	dwriteMeasuringModeNatural          = 0
	dwriteTextureClearType3x1           = 1
)

type dwriteRasterizer struct {
	factory  *iWriteFactory
	fontFace *iUnknown
	emSize   float32
}

type dwriteFontFile struct{ lpVtbl *dwriteFontFileVtbl }
type dwriteFontFileVtbl struct {
	queryInterface, addRef, release     uintptr
	getReferenceKey, getLoader, analyze uintptr
}
type dwriteGlyphRunAnalysis struct{ lpVtbl *dwriteGlyphRunAnalysisVtbl }
type dwriteGlyphRunAnalysisVtbl struct {
	queryInterface, addRef, release                                uintptr
	getAlphaTextureBounds, createAlphaTexture, getAlphaBlendParams uintptr
}
type dwriteGlyphRun struct {
	FontFace      *iUnknown
	FontEmSize    float32
	GlyphCount    uint32
	GlyphIndices  *uint16
	GlyphAdvances *float32
	GlyphOffsets  *dwriteGlyphOffset
	IsSideways    int32
	BidiLevel     uint32
}
type dwriteRect struct{ Left, Top, Right, Bottom int32 }

var embeddedFontLogOnce sync.Once

func newPlatformTextRasterizer(spec Spec, primary loadedFace) glyphRasterizer {
	if spec.TextRaster != "auto" {
		return nil
	}
	path := primary.sourcePath
	if path == "" {
		var err error
		path, err = cachedGoMonoPath()
		if err != nil {
			embeddedFontLogOnce.Do(func() { log.Printf("DirectWrite raster disabled: cache embedded Go Mono: %v", err) })
			return nil
		}
	}
	raster, err := newDWriteRasterizer(path, primary.faceIndex, spec.Size, spec.DPI)
	if err != nil {
		log.Printf("DirectWrite raster unavailable for %s: %v", path, err)
		return nil
	}
	return raster
}

func cachedGoMonoPath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cacheDir, "cervterm", "fonts")
	path := filepath.Join(dir, "gomono.ttf")
	if info, err := os.Stat(path); err == nil && info.Size() == int64(len(gomono.TTF)) {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "gomono-*.tmp")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err = tmp.Chmod(0o644); err == nil {
		_, err = tmp.Write(gomono.TTF)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	// Windows rename does not replace an existing destination. Remove only a
	// known stale/wrong-sized cache entry; concurrent writers still converge
	// because a losing rename accepts the correctly-sized winner below.
	if info, statErr := os.Stat(path); statErr == nil && info.Size() != int64(len(gomono.TTF)) {
		if removeErr := os.Remove(path); removeErr != nil {
			return "", removeErr
		}
	}
	if err = os.Rename(tmpPath, path); err != nil {
		if info, statErr := os.Stat(path); statErr == nil && info.Size() == int64(len(gomono.TTF)) {
			return path, nil
		}
		return "", err
	}
	return path, nil
}

func newDWriteRasterizer(fontPath string, faceIndex int, sizePt, dpi float64) (*dwriteRasterizer, error) {
	// Isolated factory: the shared factory's font-file cache would keep
	// fontPath locked for the process lifetime even after Close.
	factory, err := newDirectWriteFactoryOfType(dwriteFactoryTypeIsolated)
	if err != nil {
		return nil, err
	}
	fontFile, faceType, err := factory.openFontFile(fontPath)
	if err != nil {
		factory.release()
		return nil, err
	}
	fontFace, err := factory.createAnalyzedFontFace(fontFile, faceType, faceIndex)
	fontFile.release()
	if err != nil {
		factory.release()
		return nil, err
	}
	return &dwriteRasterizer{factory: factory, fontFace: fontFace, emSize: float32(sizePt * dpi / 72)}, nil
}

func (f *iWriteFactory) openFontFile(path string) (*dwriteFontFile, uint32, error) {
	path16, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, 0, err
	}
	var file *dwriteFontFile
	hr, _, callErr := syscall.SyscallN(f.lpVtbl.createFontFileReference, uintptr(unsafe.Pointer(f)), uintptr(unsafe.Pointer(path16)), 0, uintptr(unsafe.Pointer(&file)))
	if failedHRESULT(hr) || file == nil {
		if file != nil {
			file.release()
		}
		return nil, 0, fmt.Errorf("IDWriteFactory::CreateFontFileReference(%s): HRESULT 0x%08x (%v)", path, uint32(hr), callErr)
	}
	var supported int32
	var fileType, faceType, faceCount uint32
	hr, _, callErr = syscall.SyscallN(file.lpVtbl.analyze, uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(&supported)), uintptr(unsafe.Pointer(&fileType)), uintptr(unsafe.Pointer(&faceType)), uintptr(unsafe.Pointer(&faceCount)))
	if failedHRESULT(hr) || supported == 0 || faceCount == 0 {
		file.release()
		return nil, 0, fmt.Errorf("IDWriteFontFile::Analyze(%s): HRESULT 0x%08x, supported=%t, faces=%d (%v)", path, uint32(hr), supported != 0, faceCount, callErr)
	}
	return file, faceType, nil
}

func (f *iWriteFactory) createAnalyzedFontFace(file *dwriteFontFile, faceType uint32, faceIndex int) (*iUnknown, error) {
	files := []*dwriteFontFile{file}
	var face *iUnknown
	hr, _, callErr := syscall.SyscallN(f.lpVtbl.createFontFace, uintptr(unsafe.Pointer(f)), uintptr(faceType), 1, uintptr(unsafe.Pointer(&files[0])), uintptr(faceIndex), 0, uintptr(unsafe.Pointer(&face)))
	if failedHRESULT(hr) || face == nil {
		if face != nil {
			face.release()
		}
		return nil, fmt.Errorf("IDWriteFactory::CreateFontFace: HRESULT 0x%08x (%v)", uint32(hr), callErr)
	}
	return face, nil
}

func (d *dwriteRasterizer) RasterizeGlyph(glyphID uint16, cellW, cellH, baseline, cellSpan int, advancePx float32) (*image.RGBA, bool) {
	canvas := image.NewRGBA(image.Rect(0, 0, cellW*max(1, cellSpan), cellH))
	if d == nil || d.factory == nil || d.fontFace == nil || glyphID == 0 || d.emSize <= 0 {
		return canvas, false
	}
	originX := (float32(canvas.Bounds().Dx()) - advancePx) / 2
	runAdvance := advancePx
	offset := dwriteGlyphOffset{}
	run := dwriteGlyphRun{FontFace: d.fontFace, FontEmSize: d.emSize, GlyphCount: 1, GlyphIndices: &glyphID, GlyphAdvances: &runAdvance, GlyphOffsets: &offset}
	var analysis *dwriteGlyphRunAnalysis
	hr, _, _ := syscall.SyscallN(d.factory.lpVtbl.createGlyphRunAnalysis, uintptr(unsafe.Pointer(d.factory)), uintptr(unsafe.Pointer(&run)), uintptr(math.Float32bits(1)), 0, dwriteRenderingModeNaturalSymmetric, dwriteMeasuringModeNatural, uintptr(math.Float32bits(originX)), uintptr(math.Float32bits(float32(baseline))), uintptr(unsafe.Pointer(&analysis)))
	if failedHRESULT(hr) || analysis == nil {
		if analysis != nil {
			analysis.release()
		}
		return canvas, false
	}
	defer analysis.release()
	var bounds dwriteRect
	hr, _, _ = syscall.SyscallN(analysis.lpVtbl.getAlphaTextureBounds, uintptr(unsafe.Pointer(analysis)), dwriteTextureClearType3x1, uintptr(unsafe.Pointer(&bounds)))
	if failedHRESULT(hr) {
		return canvas, false
	}
	w, h := int64(bounds.Right-bounds.Left), int64(bounds.Bottom-bounds.Top)
	if w == 0 || h == 0 {
		return canvas, true
	}
	if w < 0 || h < 0 || w > int64(^uint32(0))/3 || h > int64(^uint32(0))/(w*3) {
		return canvas, false
	}
	texture := make([]byte, int(w*h*3))
	hr, _, _ = syscall.SyscallN(analysis.lpVtbl.createAlphaTexture, uintptr(unsafe.Pointer(analysis)), dwriteTextureClearType3x1, uintptr(unsafe.Pointer(&bounds)), uintptr(unsafe.Pointer(&texture[0])), uintptr(len(texture)))
	if failedHRESULT(hr) {
		return canvas, false
	}
	// Match the Go raster path: center the font's natural advance box in the cell.
	// DirectWrite reports alpha-texture bounds in canvas coordinates after applying
	// originX, so retain bounds.Left instead of hard-left-aligning every glyph.
	for y := 0; y < int(h); y++ {
		for x := 0; x < int(w); x++ {
			i := (y*int(w) + x) * 3
			a := uint8((uint16(texture[i]) + uint16(texture[i+1]) + uint16(texture[i+2]) + 1) / 3)
			dx, dy := int(bounds.Left)+x, int(bounds.Top)+y
			if dx >= 0 && dx < canvas.Bounds().Dx() && dy >= 0 && dy < canvas.Bounds().Dy() {
				p := canvas.PixOffset(dx, dy)
				canvas.Pix[p], canvas.Pix[p+1], canvas.Pix[p+2], canvas.Pix[p+3] = a, a, a, a
			}
		}
	}
	return canvas, true
}

func (d *dwriteRasterizer) Close() {
	if d == nil {
		return
	}
	if d.fontFace != nil {
		d.fontFace.release()
		d.fontFace = nil
	}
	if d.factory != nil {
		d.factory.release()
		d.factory = nil
	}
}

func (f *dwriteFontFile) release() {
	if f != nil && f.lpVtbl != nil && f.lpVtbl.release != 0 {
		syscall.SyscallN(f.lpVtbl.release, uintptr(unsafe.Pointer(f)))
	}
}
func (a *dwriteGlyphRunAnalysis) release() {
	if a != nil && a.lpVtbl != nil && a.lpVtbl.release != 0 {
		syscall.SyscallN(a.lpVtbl.release, uintptr(unsafe.Pointer(a)))
	}
}
