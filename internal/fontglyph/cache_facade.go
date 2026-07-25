package fontglyph

import (
	"io"
	"sync"

	"cervterm/internal/fontdesc"
	"cervterm/internal/fontglyph/cache"
	"cervterm/internal/fontglyph/internal/face"
)

type parsedFaceCache = cache.Manager[*parsedFontData]
type parsedFaceLease = cache.Lease[*parsedFontData]

func newParsedFaceCache(maxFaces int, maxBytes int64) *parsedFaceCache {
	return cache.New(maxFaces, maxBytes, func(data []byte, index int) (*face.Owner[*parsedFontData], error) {
		parsed, err := parseFontData(data, index)
		if err != nil {
			return nil, err
		}
		return face.NewOwner(parsed, nil), nil
	})
}

func acquireParsedFace(manager *parsedFaceCache, source string, index int, knownSize int64, load func() ([]byte, error)) (*parsedFontData, *parsedFaceLease, error) {
	owner, lease, err := manager.Acquire(source, index, knownSize, load)
	if err != nil {
		return nil, nil, err
	}
	return owner.Value(), lease, nil
}

var (
	parsedFaceCacheMu sync.Mutex
	parsedCache       = newParsedFaceCache(fontdesc.MaxParsedFaces, fontdesc.MaxParsedBytes)
)

func currentParsedFaceCache() *parsedFaceCache {
	parsedFaceCacheMu.Lock()
	defer parsedFaceCacheMu.Unlock()
	return parsedCache
}

func resetParsedFaceCacheForTest(manager *parsedFaceCache) func() {
	parsedFaceCacheMu.Lock()
	previous := parsedCache
	parsedCache = manager
	parsedFaceCacheMu.Unlock()
	return func() {
		parsedFaceCacheMu.Lock()
		parsedCache = previous
		parsedFaceCacheMu.Unlock()
	}
}

func canonicalFontCacheSource(source string) string { return cache.CanonicalSource(source) }
func fontCacheKey(source string, index int) string  { return cache.Key(source, index) }
func readParsedFontFileBounded(reader io.Reader, reserved, maxBytes int64) ([]byte, error) {
	return cache.ReadFileBounded(reader, reserved, maxBytes)
}
