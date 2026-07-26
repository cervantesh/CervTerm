package cache

import (
	"testing"

	"cervterm/internal/fontglyph/internal/face"
)

func BenchmarkL402CacheHitLease(b *testing.B) {
	manager := New(1, 8, func([]byte, int) (*face.Owner[testParsed], error) {
		return face.NewOwner(testParsed{}, nil), nil
	})
	load := func() ([]byte, error) { return []byte{1}, nil }
	_, seed, err := manager.Acquire("test:benchmark-hit", 0, 1, load)
	if err != nil {
		b.Fatal(err)
	}
	seed.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, lease, err := manager.Acquire("test:benchmark-hit", 0, 1, load)
		if err != nil {
			b.Fatal(err)
		}
		lease.Close()
	}
}
