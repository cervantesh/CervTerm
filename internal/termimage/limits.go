package termimage

import (
	"fmt"
	"math"
	"time"
)

const (
	HardControlFrameBytes       uint64 = 256 * 1024
	HardControlChunkBytes       uint64 = 16 * 1024
	HardPendingTransfersPerPane uint64 = 8
	HardPendingTransfersProcess uint64 = 32
	HardChunksPerTransfer       uint64 = 4096
	HardEncodedBytesPerPane     uint64 = 8 * 1024 * 1024
	HardEncodedBytesProcess     uint64 = 32 * 1024 * 1024
	HardDecodedImageBytes       uint64 = 64 * 1024 * 1024
	HardDecodedBytesPerPane     uint64 = 64 * 1024 * 1024
	HardDecodedBytesProcess     uint64 = 256 * 1024 * 1024
	HardImagesPerPane           uint64 = 256
	HardImagesProcess           uint64 = 1024
	HardPlacementsPerPane       uint64 = 1024
	HardPlacementsProcess       uint64 = 4096
	HardImageDimension          uint32 = 4096
	HardImagePixels             uint64 = 16_777_216
	HardPlacementSpan           uint16 = 256
	HardDecodeWorkersPerPane    uint64 = 1
	HardDecodeWorkersProcess    uint64 = 2
	HardReplyBytes              uint64 = 512
	HardPendingReplyBytesPane   uint64 = 64 * 1024
	HardGPUEntriesPerContext    uint64 = 512
	HardGPUBytesPerContext      uint64 = 256 * 1024 * 1024
)

const (
	HardTransferLifetime   = 10 * time.Second
	HardAcceptanceDeadline = 250 * time.Millisecond
)

type Limits struct {
	EncodedBytes uint64
	DecodedBytes uint64
	Images       uint64
	Placements   uint64
}

func DefaultLimits() Limits {
	return Limits{
		EncodedBytes: HardEncodedBytesPerPane,
		DecodedBytes: HardDecodedBytesPerPane,
		Images:       HardImagesPerPane,
		Placements:   HardPlacementsPerPane,
	}
}

func ValidateLimits(limits Limits) (Limits, error) {
	if limits.EncodedBytes == 0 || limits.EncodedBytes > HardEncodedBytesPerPane ||
		limits.DecodedBytes == 0 || limits.DecodedBytes > HardDecodedBytesPerPane ||
		limits.Images == 0 || limits.Images > HardImagesPerPane ||
		limits.Placements == 0 || limits.Placements > HardPlacementsPerPane {
		return Limits{}, ErrInvalidLimits
	}
	return limits, nil
}

func CheckedRGBABytes(width, height uint32) (uint32, uint64, error) {
	if width == 0 || height == 0 || width > HardImageDimension || height > HardImageDimension {
		return 0, 0, fmt.Errorf("%w: dimensions", ErrLimitExceeded)
	}
	pixels := uint64(width) * uint64(height)
	if pixels > HardImagePixels {
		return 0, 0, fmt.Errorf("%w: pixels", ErrLimitExceeded)
	}
	stride64 := uint64(width) * 4
	bytes := stride64 * uint64(height)
	if stride64 > math.MaxUint32 || bytes > HardDecodedImageBytes || bytes > uint64(maxInt()) {
		return 0, 0, fmt.Errorf("%w: decoded bytes", ErrLimitExceeded)
	}
	return uint32(stride64), bytes, nil
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
