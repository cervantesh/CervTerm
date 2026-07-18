package background

import (
	"crypto/sha256"
	"image"
	"image/color"
)

func testSource(width, height int, value color.RGBA, identity string) *Source {
	rgba := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			rgba.SetRGBA(x, y, value)
		}
	}
	return &Source{
		rgba: rgba, digest: sha256.Sum256([]byte(identity)), format: "test",
		cpuBytes:     uint64(width * height * 4),
		encodedBytes: uint64(len(identity)),
	}
}
