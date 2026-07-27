package fontglyph

import (
	"fmt"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

func faceFromParsed(pf *parsedFontData, spec Spec) (loadedFace, font.Metrics, error) {
	face, err := opentype.NewFace(pf.sfnt, &opentype.FaceOptions{Size: spec.Size, DPI: spec.DPI, Hinting: font.HintingFull})
	if err != nil {
		return loadedFace{}, font.Metrics{}, err
	}
	lf := loadedFace{
		face: face, sfnt: pf.sfnt, tables: pf.tables, rasterColor: pf.rasterColor,
		sbix: pf.sbix, cbdt: pf.cbdt, colr: pf.colr, svg: pf.svg,
	}
	return lf, face.Metrics(), nil
}

func cachedParsedFont(key string, index int, load func() ([]byte, error)) (*parsedFontData, *parsedFaceLease, error) {
	return acquireParsedFace(currentParsedFaceCache(), key, index, 0, load)
}

func cachedParsedFontKnownSize(key string, index int, knownSize int64, load func() ([]byte, error)) (*parsedFontData, *parsedFaceLease, error) {
	return acquireParsedFace(currentParsedFaceCache(), key, index, knownSize, load)
}

func loadCachedFaceIndex(key string, index int, spec Spec, load func() ([]byte, error)) (loadedFace, font.Metrics, error) {
	pf, handle, err := cachedParsedFont(key, index, load)
	if err != nil {
		return loadedFace{}, font.Metrics{}, err
	}
	lf, metrics, err := faceFromParsed(pf, spec)
	if err != nil {
		handle.Close()
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
		handle.Close()
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
	manager := currentParsedFaceCache()
	knownSize := info.Size()
	maxBytes := manager.MaxBytes()
	if knownSize < 0 || knownSize > maxBytes {
		return loadedFace{}, font.Metrics{}, fmt.Errorf("%w: font stat size is %d (limit %d)", errFontCacheCapacity, knownSize, maxBytes)
	}
	pf, handle, err := acquireParsedFace(manager, path, index, knownSize, func() ([]byte, error) {
		return readParsedFontFileBounded(file, knownSize, maxBytes)
	})
	if err != nil {
		return loadedFace{}, font.Metrics{}, err
	}
	lf, metrics, err := faceFromParsed(pf, spec)
	if err != nil {
		handle.Close()
		return loadedFace{}, font.Metrics{}, err
	}
	lf.faceIndex, lf.cacheHandle = index, handle
	return lf, metrics, nil
}
