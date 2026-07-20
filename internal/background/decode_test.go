package background

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func encodedImage(t *testing.T, format string, width, height int) []byte {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, width, height))
	value.SetRGBA(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var output bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&output, value)
	case "jpeg":
		err = jpeg.Encode(&output, value, nil)
	case "gif":
		err = gif.Encode(&output, value, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestDecodeBytesSupportedStaticFormats(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif"} {
		t.Run(format, func(t *testing.T) {
			budget := NewBudget()
			source, err := DecodeBytes(2, encodedImage(t, format, 2, 3), budget)
			if err != nil {
				t.Fatal(err)
			}
			if source.Format() != format || source.Bounds().Dx() != 2 || source.Bounds().Dy() != 3 || budget.CPUBytes() != 24 {
				t.Fatalf("source=%s %v budget=%d", source.Format(), source.Bounds(), budget.CPUBytes())
			}
			if err := source.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDecodeRejectsAnimatedGIFAndRedactsPath(t *testing.T) {
	palette := color.Palette{color.Black, color.White}
	frame := image.NewPaletted(image.Rect(0, 0, 1, 1), palette)
	var output bytes.Buffer
	if err := gif.EncodeAll(&output, &gif.GIF{Image: []*image.Paletted{frame, frame}, Delay: []int{0, 0}}); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeBytes(1, output.Bytes(), NewBudget()); err == nil || !strings.Contains(err.Error(), "multi-frame") {
		t.Fatalf("animated GIF error = %v", err)
	}
	secret := t.TempDir() + "/secret-name.png"
	if _, err := DecodeFile(1, secret, NewBudget()); err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "secret-name") {
		t.Fatalf("redacted path error = %v", err)
	}
}

func TestDecodeBudgetsRejectBeforeAllocation(t *testing.T) {
	budget := NewBudget()
	if err := budget.reserveEncoded(0, MaxEncodedBytesPerImage+1); err == nil {
		t.Fatal("expected per-image encoded limit")
	}
	budget = NewBudget()
	if err := budget.reserveEncoded(0, MaxEncodedBytesPerImage); err != nil {
		t.Fatal(err)
	}
	if err := budget.reserveEncoded(1, MaxEncodedBytesPerImage); err != nil {
		t.Fatal(err)
	}
	if err := budget.reserveEncoded(2, 1); err == nil {
		t.Fatal("expected aggregate encoded limit")
	}
	for _, test := range []struct{ width, height int }{{MaxImageDimension + 1, 1}, {8000, 5000}, {0, 1}} {
		if _, err := validateDecodedDimensions(0, test.width, test.height); err == nil {
			t.Fatalf("dimensions %dx%d accepted", test.width, test.height)
		}
	}
}
