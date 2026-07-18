package background

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	ErrSourceClosed = errors.New("background source is closed")
	ErrSourceOwned  = errors.New("background source is owned by a cache")
)

// Source is an immutable decoded image. Close releases its retained pixels;
// callers transfer ownership to Cache when inserting it.
type Source struct {
	rgba          *image.RGBA
	digest        [sha256.Size]byte
	canonicalPath string
	format        string
	cpuBytes      uint64
	encodedBytes  uint64
	closed        bool
	owner         *Cache
}

func (s *Source) EncodedBytes() uint64 {
	if s == nil || s.closed {
		return 0
	}
	return s.encodedBytes
}

func (s *Source) CPUBytes() uint64 {
	if s == nil || s.closed {
		return 0
	}
	return s.cpuBytes
}

func (s *Source) Digest() [sha256.Size]byte {
	if s == nil {
		return [sha256.Size]byte{}
	}
	return s.digest
}

func (s *Source) Format() string {
	if s == nil {
		return ""
	}
	return s.format
}

func (s *Source) CanonicalPath() string {
	if s == nil {
		return ""
	}
	return s.canonicalPath
}

func (s *Source) Bounds() image.Rectangle {
	if s == nil || s.closed || s.rgba == nil {
		return image.Rectangle{}
	}
	return s.rgba.Bounds()
}

func (s *Source) ColorModel() color.Model {
	return color.RGBAModel
}

func (s *Source) At(x, y int) color.Color {
	if s == nil || s.closed || s.rgba == nil {
		return color.RGBA{}
	}
	return s.rgba.At(x, y)
}

func (s *Source) Close() error {
	if s == nil || s.closed {
		return ErrSourceClosed
	}
	if s.owner != nil {
		return ErrSourceOwned
	}
	return s.closeOwned()
}

func (s *Source) closeOwned() error {
	if s == nil || s.closed {
		return ErrSourceClosed
	}
	s.closed = true
	s.owner = nil
	s.rgba = nil
	s.cpuBytes = 0
	s.encodedBytes = 0
	return nil
}

// FileDigest returns a bounded canonical content identity without decoding pixels.
func FileDigest(imageIndex int, path string) (string, [sha256.Size]byte, error) {
	canonical, err := canonicalizePath(path)
	if err != nil {
		return "", [sha256.Size]byte{}, fmt.Errorf("image %d path: canonicalization failed", imageIndex)
	}
	file, err := os.Open(canonical)
	if err != nil {
		return "", [sha256.Size]byte{}, fmt.Errorf("image %d file: open failed", imageIndex)
	}
	defer file.Close()
	encoded, err := io.ReadAll(io.LimitReader(file, int64(MaxEncodedBytesPerImage)+1))
	if err != nil {
		return "", [sha256.Size]byte{}, fmt.Errorf("image %d encoded input: read failed", imageIndex)
	}
	if uint64(len(encoded)) > MaxEncodedBytesPerImage {
		return "", [sha256.Size]byte{}, fmt.Errorf("image %d encoded budget: exceeds per-image limit", imageIndex)
	}
	return canonical, sha256.Sum256(encoded), nil
}

// DecodeFile decodes a trusted local file and redacts its path from all
// returned error text.
func DecodeFile(imageIndex int, path string, budget *Budget) (*Source, error) {
	canonical, err := canonicalizePath(path)
	if err != nil {
		return nil, fmt.Errorf("image %d path: canonicalization failed", imageIndex)
	}
	file, err := os.Open(canonical)
	if err != nil {
		return nil, fmt.Errorf("image %d file: open failed", imageIndex)
	}
	defer file.Close()
	return decodeReader(imageIndex, file, canonical, budget)
}

// DecodeBytes decodes trusted local image bytes without assigning a cacheable
// path identity.
func DecodeBytes(imageIndex int, encoded []byte, budget *Budget) (*Source, error) {
	return decodeReader(imageIndex, bytes.NewReader(encoded), "", budget)
}

func decodeReader(imageIndex int, reader io.Reader, canonicalPath string, budget *Budget) (*Source, error) {
	limited := io.LimitReader(reader, int64(MaxEncodedBytesPerImage)+1)
	encoded, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("image %d encoded input: read failed", imageIndex)
	}
	if err := budget.reserveEncoded(imageIndex, uint64(len(encoded))); err != nil {
		return nil, err
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("image %d format: unsupported or malformed", imageIndex)
	}
	if format != "png" && format != "jpeg" && format != "gif" {
		return nil, fmt.Errorf("image %d format %q: unsupported", imageIndex, format)
	}
	cpuBytes, err := validateDecodedDimensions(imageIndex, config.Width, config.Height)
	if err != nil {
		return nil, err
	}
	if format == "gif" {
		frames, err := countGIFFrames(encoded)
		if err != nil {
			return nil, fmt.Errorf("image %d format gif: malformed", imageIndex)
		}
		if frames != 1 {
			return nil, fmt.Errorf("image %d format gif: animated or multi-frame images are unsupported", imageIndex)
		}
	}

	var decoded image.Image
	if format == "gif" {
		decoded, err = gif.Decode(bytes.NewReader(encoded))
	} else {
		decoded, _, err = image.Decode(bytes.NewReader(encoded))
	}
	if err != nil {
		return nil, fmt.Errorf("image %d format %s: decode failed", imageIndex, format)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != config.Width || bounds.Dy() != config.Height {
		return nil, fmt.Errorf("image %d format %s: inconsistent dimensions", imageIndex, format)
	}

	rgba := image.NewRGBA(image.Rect(0, 0, config.Width, config.Height))
	draw.Draw(rgba, rgba.Bounds(), decoded, bounds.Min, draw.Src)
	source := &Source{
		rgba:          rgba,
		digest:        sha256.Sum256(encoded),
		canonicalPath: canonicalPath,
		format:        format,
		cpuBytes:      cpuBytes,
		encodedBytes:  uint64(len(encoded)),
	}
	if err := budget.reserveDecoded(source, cpuBytes); err != nil {
		source.rgba = nil
		return nil, err
	}
	return source, nil
}

func validateDecodedDimensions(imageIndex, width, height int) (uint64, error) {
	if width <= 0 || height <= 0 {
		return 0, fmt.Errorf("image %d dimensions: expected positive values", imageIndex)
	}
	if width > MaxImageDimension || height > MaxImageDimension {
		return 0, fmt.Errorf("image %d dimensions: exceed limit", imageIndex)
	}
	pixels, ok := checkedMultiply(uint64(width), uint64(height))
	if !ok || pixels > MaxImagePixels {
		return 0, fmt.Errorf("image %d pixel budget: exceeds limit", imageIndex)
	}
	cpuBytes, ok := checkedMultiply(pixels, 4)
	if !ok || cpuBytes > uint64(maxInt()) {
		return 0, fmt.Errorf("image %d decoded byte budget: overflow", imageIndex)
	}
	return cpuBytes, nil
}

func canonicalizePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("empty path")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	canonical = filepath.Clean(canonical)
	if runtime.GOOS == "windows" {
		canonical = strings.ToLower(canonical)
	}
	return canonical, nil
}

func countGIFFrames(encoded []byte) (int, error) {
	if len(encoded) < 13 || (string(encoded[:6]) != "GIF87a" && string(encoded[:6]) != "GIF89a") {
		return 0, errors.New("invalid header")
	}
	position := 13
	packed := encoded[10]
	if packed&0x80 != 0 {
		size := 3 * (1 << ((packed & 0x07) + 1))
		position += size
	}
	if position > len(encoded) {
		return 0, errors.New("truncated color table")
	}
	frames := 0
	for position < len(encoded) {
		switch encoded[position] {
		case 0x3b:
			if frames == 0 {
				return 0, errors.New("no frames")
			}
			return frames, nil
		case 0x21:
			position += 2 // extension introducer and label
			if position > len(encoded) {
				return 0, errors.New("truncated extension")
			}
			var err error
			position, err = skipGIFSubBlocks(encoded, position)
			if err != nil {
				return 0, err
			}
		case 0x2c:
			frames++
			if frames > 1 {
				return frames, nil
			}
			if position+10 > len(encoded) {
				return 0, errors.New("truncated image descriptor")
			}
			localPacked := encoded[position+9]
			position += 10
			if localPacked&0x80 != 0 {
				position += 3 * (1 << ((localPacked & 0x07) + 1))
			}
			if position >= len(encoded) {
				return 0, errors.New("truncated image data")
			}
			position++ // LZW minimum code size
			var err error
			position, err = skipGIFSubBlocks(encoded, position)
			if err != nil {
				return 0, err
			}
		default:
			return 0, errors.New("unknown block")
		}
	}
	return 0, errors.New("missing trailer")
}

func skipGIFSubBlocks(encoded []byte, position int) (int, error) {
	for {
		if position >= len(encoded) {
			return 0, errors.New("truncated sub-block")
		}
		size := int(encoded[position])
		position++
		if size == 0 {
			return position, nil
		}
		if size > len(encoded)-position {
			return 0, errors.New("truncated sub-block data")
		}
		position += size
	}
}
