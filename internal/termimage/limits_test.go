package termimage

import (
	"math"
	"testing"
	"time"
)

func TestHardLimitsMatchADR0014(t *testing.T) {
	if HardControlFrameBytes != 256*1024 || HardControlChunkBytes != 16*1024 ||
		HardPendingTransfersPerPane != 8 || HardPendingTransfersProcess != 32 ||
		HardChunksPerTransfer != 4096 || HardTransferLifetime != 10*time.Second ||
		HardEncodedBytesPerPane != 8*1024*1024 || HardEncodedBytesProcess != 32*1024*1024 ||
		HardDecodedImageBytes != 64*1024*1024 || HardDecodedBytesPerPane != 64*1024*1024 || HardDecodedBytesProcess != 256*1024*1024 ||
		HardImagesPerPane != 256 || HardImagesProcess != 1024 ||
		HardPlacementsPerPane != 1024 || HardPlacementsProcess != 4096 ||
		HardImageDimension != 4096 || HardImagePixels != 16_777_216 || HardPlacementSpan != 256 ||
		HardDecodeWorkersPerPane != 1 || HardDecodeWorkersProcess != 2 || HardAcceptanceDeadline != 250*time.Millisecond ||
		HardReplyBytes != 512 || HardPendingReplyBytesPane != 64*1024 ||
		HardGPUEntriesPerContext != 512 || HardGPUBytesPerContext != 256*1024*1024 {
		t.Fatal("hard image limits drifted from ADR 0014")
	}
}

func TestValidateLimitsAreLowerOnly(t *testing.T) {
	defaults := DefaultLimits()
	if got, err := ValidateLimits(defaults); err != nil || got != defaults {
		t.Fatalf("defaults = %#v, %v", got, err)
	}
	lower := Limits{EncodedBytes: 1, DecodedBytes: 2, Images: 3, Placements: 4}
	if got, err := ValidateLimits(lower); err != nil || got != lower {
		t.Fatalf("lower limits = %#v, %v", got, err)
	}
	for name, invalid := range map[string]Limits{
		"zero encoded":          {DecodedBytes: 1, Images: 1, Placements: 1},
		"encoded above hard":    {EncodedBytes: HardEncodedBytesPerPane + 1, DecodedBytes: 1, Images: 1, Placements: 1},
		"decoded above hard":    {EncodedBytes: 1, DecodedBytes: HardDecodedBytesPerPane + 1, Images: 1, Placements: 1},
		"images above hard":     {EncodedBytes: 1, DecodedBytes: 1, Images: HardImagesPerPane + 1, Placements: 1},
		"placements above hard": {EncodedBytes: 1, DecodedBytes: 1, Images: 1, Placements: HardPlacementsPerPane + 1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ValidateLimits(invalid); err == nil {
				t.Fatal("invalid limits accepted")
			}
		})
	}
}

func TestCheckedRGBABytes(t *testing.T) {
	stride, size, err := CheckedRGBABytes(4096, 4096)
	if err != nil || stride != 16384 || size != HardDecodedImageBytes {
		t.Fatalf("max image = stride %d size %d err %v", stride, size, err)
	}
	for _, dimensions := range [][2]uint32{{0, 1}, {1, 0}, {4097, 1}, {1, 4097}, {math.MaxUint32, math.MaxUint32}} {
		if _, _, err := CheckedRGBABytes(dimensions[0], dimensions[1]); err == nil {
			t.Fatalf("dimensions %v accepted", dimensions)
		}
	}
}

func FuzzCheckedRGBABytes(f *testing.F) {
	f.Add(uint32(1), uint32(1))
	f.Add(uint32(4096), uint32(4096))
	f.Add(uint32(math.MaxUint32), uint32(math.MaxUint32))
	f.Fuzz(func(t *testing.T, width, height uint32) {
		stride, size, err := CheckedRGBABytes(width, height)
		if err != nil {
			return
		}
		if width == 0 || height == 0 || width > HardImageDimension || height > HardImageDimension ||
			uint64(width)*uint64(height) > HardImagePixels || uint64(stride)*uint64(height) != size || size > HardDecodedImageBytes {
			t.Fatalf("accepted invalid %dx%d stride=%d size=%d", width, height, stride, size)
		}
	})
}
