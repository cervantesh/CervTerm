package background

import (
	"fmt"
	"math"
)

const (
	MaxEncodedBytesPerImage  uint64 = 16 << 20
	MaxAggregateEncodedBytes uint64 = 32 << 20
	MaxAggregateCPUBytes     uint64 = 128 << 20
	MaxAggregateGPUBytes     uint64 = 128 << 20
	MaxImageDimension               = 8192
	MaxImagePixels           uint64 = 32_000_000
)

// Budget owns the resource accounting for one background candidate. It is not
// safe for concurrent use; background candidate construction is intentionally
// synchronous.
type Budget struct {
	encodedBytes uint64
	cpuBytes     uint64
	sources      map[*Source]struct{}
}

func NewBudget() *Budget {
	return &Budget{sources: make(map[*Source]struct{})}
}

func (b *Budget) EncodedBytes() uint64 {
	if b == nil {
		return 0
	}
	return b.encodedBytes
}

func (b *Budget) CPUBytes() uint64 {
	if b == nil {
		return 0
	}
	return b.cpuBytes
}

// PinSource accounts one already-decoded cached source in this candidate budget.
func (b *Budget) PinSource(imageIndex int, source *Source) error {
	if source == nil || source.closed {
		return fmt.Errorf("image %d cached source: unavailable", imageIndex)
	}
	if err := b.reserveEncoded(imageIndex, source.encodedBytes); err != nil {
		return err
	}
	return b.reserveDecoded(source, source.cpuBytes)
}

func (b *Budget) reserveEncoded(imageIndex int, size uint64) error {
	if b == nil {
		return fmt.Errorf("image %d budget: owner is required", imageIndex)
	}
	if size > MaxEncodedBytesPerImage {
		return fmt.Errorf("image %d encoded budget: exceeds per-image limit", imageIndex)
	}
	total, ok := checkedAdd(b.encodedBytes, size)
	if !ok || total > MaxAggregateEncodedBytes {
		return fmt.Errorf("encoded aggregate budget: exceeds limit")
	}
	b.encodedBytes = total
	return nil
}

func (b *Budget) reserveDecoded(source *Source, size uint64) error {
	if b == nil {
		return fmt.Errorf("decoded budget: owner is required")
	}
	if _, exists := b.sources[source]; exists {
		return nil
	}
	total, ok := checkedAdd(b.cpuBytes, size)
	if !ok || total > MaxAggregateCPUBytes {
		return fmt.Errorf("decoded aggregate budget: exceeds limit")
	}
	b.cpuBytes = total
	b.sources[source] = struct{}{}
	return nil
}

func (b *Budget) reserveComposition(sources []*Source, outputBytes uint64) error {
	if b == nil {
		return fmt.Errorf("composition budget: owner is required")
	}
	additional := outputBytes
	seen := make(map[*Source]struct{}, len(sources))
	for _, source := range sources {
		if source == nil {
			continue
		}
		if _, duplicate := seen[source]; duplicate {
			continue
		}
		seen[source] = struct{}{}
		if _, included := b.sources[source]; included {
			continue
		}
		var ok bool
		additional, ok = checkedAdd(additional, source.cpuBytes)
		if !ok {
			return fmt.Errorf("composition CPU budget: exceeds limit")
		}
	}
	total, ok := checkedAdd(b.cpuBytes, additional)
	if !ok || total > MaxAggregateCPUBytes {
		return fmt.Errorf("composition CPU budget: exceeds limit")
	}
	b.cpuBytes = total
	for source := range seen {
		b.sources[source] = struct{}{}
	}
	return nil
}

func checkedAdd(left, right uint64) (uint64, bool) {
	if right > math.MaxUint64-left {
		return 0, false
	}
	return left + right, true
}

// SurfaceBytes returns the checked RGBA storage size used for CPU/GPU residency accounting.
func SurfaceBytes(width, height int) (uint64, error) { return checkedRGBABytes(width, height) }

func checkedRGBABytes(width, height int) (uint64, error) {
	if width < 0 || height < 0 {
		return 0, fmt.Errorf("surface dimensions: expected non-negative values")
	}
	if width == 0 || height == 0 {
		return 0, nil
	}
	w := uint64(width)
	h := uint64(height)
	pixels, ok := checkedMultiply(w, h)
	if !ok {
		return 0, fmt.Errorf("surface allocation: dimensions overflow")
	}
	bytes, ok := checkedMultiply(pixels, 4)
	if !ok || bytes > uint64(maxInt()) {
		return 0, fmt.Errorf("surface allocation: byte size overflow")
	}
	return bytes, nil
}

func checkedMultiply(left, right uint64) (uint64, bool) {
	if left != 0 && right > math.MaxUint64/left {
		return 0, false
	}
	return left * right, true
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
