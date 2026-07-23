package termimage

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"
)

func pngBytesForTest(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 1, G: 2, B: 3, A: 4})
	img.SetNRGBA(1, 0, color.NRGBA{R: 5, G: 6, B: 7, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}
func TestSharedPNGCodecTwoPassExactRGBA(t *testing.T) {
	process := NewProcessBudget()
	store := NewStore(process, DefaultLimits())
	encoded := pngBytesForTest(t)
	opens := 0
	candidate, err := DecodePNG(context.Background(), store, 1, func() (io.Reader, error) { opens++; return bytes.NewReader(encoded), nil })
	if err != nil || opens != 2 {
		t.Fatalf("opens=%d err=%v", opens, err)
	}
	if got, want := candidate.RGBA(), []byte{1, 2, 3, 4, 5, 6, 7, 255}; !bytes.Equal(got, want) {
		t.Fatalf("rgba=%v", got)
	}
	if !candidate.WritesSealed() {
		t.Fatal("candidate writes not sealed")
	}
	candidate.Close()
	if store.Usage() != (Usage{}) || process.Usage() != (Usage{}) {
		t.Fatalf("usage=%#v", store.Usage())
	}
}
func TestSharedPNGCodecRejectsTailCancellationAndFactoryError(t *testing.T) {
	encoded := pngBytesForTest(t)
	for name, run := range map[string]func(*Store) error{"tail": func(store *Store) error {
		payload := append(append([]byte(nil), encoded...), 1)
		_, err := DecodePNG(context.Background(), store, 1, func() (io.Reader, error) { return bytes.NewReader(payload), nil })
		return err
	}, "cancel": func(store *Store) error {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := DecodePNG(ctx, store, 1, func() (io.Reader, error) { return bytes.NewReader(encoded), nil })
		return err
	}, "factory": func(store *Store) error {
		_, err := DecodePNG(context.Background(), store, 1, func() (io.Reader, error) { return nil, ErrClosed })
		return err
	}} {
		t.Run(name, func(t *testing.T) {
			process := NewProcessBudget()
			store := NewStore(process, DefaultLimits())
			err := run(store)
			if err == nil {
				t.Fatal("accepted")
			}
			if name == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation identity lost: %v", err)
			}
			if store.Usage() != (Usage{}) || process.Usage() != (Usage{}) {
				t.Fatalf("usage=%#v", store.Usage())
			}
		})
	}
}
