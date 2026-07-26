package discovery

import (
	"path/filepath"
	"testing"

	"cervterm/internal/fontdesc"
)

func BenchmarkL402DiscoveryTopK(b *testing.B) {
	paths := make([]string, fontdesc.MaxDiscoveryFiles+257)
	for i := range paths {
		paths[i] = filepath.Join("fonts", benchmarkPathName(len(paths)-i))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if got := selectTopKPaths(paths, fontdesc.MaxDiscoveryFiles); len(got) != fontdesc.MaxDiscoveryFiles {
			b.Fatalf("selected %d paths", len(got))
		}
	}
}

func benchmarkPathName(value int) string {
	const digits = "0123456789abcdef"
	var name [12]byte
	for i := len(name) - 5; i >= 0; i-- {
		name[i] = digits[value&15]
		value >>= 4
	}
	copy(name[len(name)-4:], ".ttf")
	return string(name[:])
}
