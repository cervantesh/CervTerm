package background

import (
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestCachePinsLRUAndExactRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "background.png")
	if err := os.WriteFile(path, []byte("identity"), 0o600); err != nil {
		t.Fatal(err)
	}
	cache := NewCache()
	var firstDigest, secondDigest [32]byte
	for index := 0; index < MaxCacheEntries; index++ {
		source := testSource(1, 1, color.RGBA{R: uint8(index), A: 255}, string(rune('a'+index)))
		if index == 0 {
			firstDigest = source.Digest()
		} else if index == 1 {
			secondDigest = source.Digest()
		}
		lease, err := cache.Insert(path, source)
		if err != nil {
			t.Fatal(err)
		}
		if err := cache.Release(lease); err != nil {
			t.Fatal(err)
		}
		if err := cache.Release(lease); !errors.Is(err, ErrLeaseReleased) {
			t.Fatalf("second release = %v", err)
		}
	}
	pinned, hit, err := cache.Acquire(path, firstDigest)
	if err != nil || !hit {
		t.Fatalf("acquire first: hit=%v err=%v", hit, err)
	}
	newSource := testSource(1, 1, color.RGBA{B: 255, A: 255}, "new")
	newLease, err := cache.Insert(path, newSource)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Release(newLease); err != nil {
		t.Fatal(err)
	}
	if err := cache.Release(pinned); err != nil {
		t.Fatal(err)
	}
	first, hit, err := cache.Acquire(path, firstDigest)
	if err != nil || !hit {
		t.Fatalf("pinned oldest was evicted: hit=%v err=%v", hit, err)
	}
	if err := cache.Release(first); err != nil {
		t.Fatal(err)
	}
	if _, hit, err := cache.Acquire(path, secondDigest); err != nil || hit {
		t.Fatalf("least-recently-used entry hit=%v err=%v, want eviction", hit, err)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCacheCloseIsTransactionalWithPins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "background.png")
	if err := os.WriteFile(path, []byte("identity"), 0o600); err != nil {
		t.Fatal(err)
	}
	cache := NewCache()
	source := testSource(1, 1, color.RGBA{A: 255}, "one")
	lease, err := cache.Insert(path, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Close(); !errors.Is(err, ErrCachePinned) || source.closed {
		t.Fatalf("pinned close = %v closed=%v", err, source.closed)
	}
	if err := cache.Release(lease); err != nil {
		t.Fatal(err)
	}
	if err := cache.Close(); err != nil || !source.closed {
		t.Fatalf("close = %v source.closed=%v", err, source.closed)
	}
	if err := cache.Close(); err != nil {
		t.Fatalf("idempotent close = %v", err)
	}
}

func TestCacheVariantIdentityAndResidencyTrim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "background.png")
	if err := os.WriteFile(path, []byte("identity"), 0o600); err != nil {
		t.Fatal(err)
	}
	cache := NewCache()
	source := testSource(1, 1, color.RGBA{A: 255}, "variant")
	variant := CacheVariant{Fit: "cover", Horizontal: "center", Vertical: "top", Width: 80, Height: 24, DPIBits: 96}
	lease, err := cache.InsertVariant(path, source, variant)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Release(lease); err != nil {
		t.Fatal(err)
	}
	if _, hit, err := cache.AcquireVariant(path, source.Digest(), CacheVariant{Fit: "contain", Width: 80, Height: 24, DPIBits: 96}); err != nil || hit {
		t.Fatalf("different transform variant hit=%v err=%v", hit, err)
	}
	hitLease, hit, err := cache.AcquireVariant(path, source.Digest(), variant)
	if err != nil || !hit {
		t.Fatalf("same variant hit=%v err=%v", hit, err)
	}
	if err := cache.Release(hitLease); err != nil {
		t.Fatal(err)
	}
	if cache.ResidentBytes() != 4 {
		t.Fatalf("resident bytes = %d", cache.ResidentBytes())
	}
	if err := cache.TrimFor(MaxAggregateCPUBytes - 3); err != nil {
		t.Fatal(err)
	}
	if cache.ResidentBytes() != 0 || !source.closed {
		t.Fatalf("trim residency=%d closed=%v", cache.ResidentBytes(), source.closed)
	}
}
