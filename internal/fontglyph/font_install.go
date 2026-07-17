package fontglyph

import (
	"fmt"
	"io"
	"math"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

func faceFromParsed(pf *parsedFontData, spec Spec) (loadedFace, font.Metrics, error) {
	face, err := opentype.NewFace(pf.sfnt, &opentype.FaceOptions{Size: spec.Size, DPI: spec.DPI, Hinting: font.HintingFull})
	if err != nil {
		return loadedFace{}, font.Metrics{}, err
	}
	lf := loadedFace{face: face, sfnt: pf.sfnt, tables: pf.tables, sbix: pf.sbix, cbdt: pf.cbdt, colr: pf.colr, svg: pf.svg}
	return lf, face.Metrics(), nil
}

func cachedParsedFont(key string, index int, load func() ([]byte, error)) (*parsedFontData, *parsedFontHandle, error) {
	return currentFontCache().acquire(key, index, 0, load)
}

func cachedParsedFontKnownSize(key string, index int, knownSize int64, load func() ([]byte, error)) (*parsedFontData, *parsedFontHandle, error) {
	return currentFontCache().acquire(key, index, knownSize, load)
}

func loadCachedFaceIndex(key string, index int, spec Spec, load func() ([]byte, error)) (loadedFace, font.Metrics, error) {
	pf, handle, err := cachedParsedFont(key, index, load)
	if err != nil {
		return loadedFace{}, font.Metrics{}, err
	}
	lf, metrics, err := faceFromParsed(pf, spec)
	if err != nil {
		handle.release()
		return loadedFace{}, font.Metrics{}, err
	}
	lf.faceIndex, lf.cacheHandle = index, handle
	return lf, metrics, nil
}

func loadCachedFaceIndexKnownSize(key string, index int, knownSize int64, spec Spec, load func() ([]byte, error)) (loadedFace, font.Metrics, error) {
	pf, handle, err := cachedParsedFontKnownSize(key, index, knownSize, load)
	if err != nil {
		return loadedFace{}, font.Metrics{}, err
	}
	lf, metrics, err := faceFromParsed(pf, spec)
	if err != nil {
		handle.release()
		return loadedFace{}, font.Metrics{}, err
	}
	lf.faceIndex, lf.cacheHandle = index, handle
	return lf, metrics, nil
}

func loadCachedFileFaceIndex(path string, index int, spec Spec) (loadedFace, font.Metrics, error) {
	file, err := os.Open(path)
	if err != nil {
		return loadedFace{}, font.Metrics{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return loadedFace{}, font.Metrics{}, err
	}
	manager := currentFontCache()
	knownSize := info.Size()
	if knownSize < 0 || knownSize > manager.maxBytes {
		return loadedFace{}, font.Metrics{}, fmt.Errorf("%w: font stat size is %d (limit %d)", errFontCacheCapacity, knownSize, manager.maxBytes)
	}
	pf, handle, err := manager.acquire(path, index, knownSize, func() ([]byte, error) {
		return readFontFileBounded(file, knownSize, manager.maxBytes)
	})
	if err != nil {
		return loadedFace{}, font.Metrics{}, err
	}
	lf, metrics, err := faceFromParsed(pf, spec)
	if err != nil {
		handle.release()
		return loadedFace{}, font.Metrics{}, err
	}
	lf.faceIndex, lf.cacheHandle = index, handle
	return lf, metrics, nil
}

func readFontFileBounded(reader io.Reader, reserved, maxBytes int64) ([]byte, error) {
	if reserved < 0 || maxBytes < 0 || reserved > maxBytes {
		return nil, errFontCacheCapacity
	}
	limit := reserved
	if limit < math.MaxInt64 {
		limit++
	}
	data, err := io.ReadAll(io.LimitReader(reader, limit))
	if err != nil {
		return nil, err
	}
	actual := int64(len(data))
	if actual > maxBytes {
		return nil, fmt.Errorf("%w: font is larger than %d bytes", errFontCacheCapacity, maxBytes)
	}
	if actual > reserved {
		return nil, fmt.Errorf("%w: stat=%d read=%d", errFontFileGrew, reserved, actual)
	}
	return data, nil
}
