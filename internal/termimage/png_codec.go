package termimage

import (
	"context"
	"image/color"
	"image/png"
	"io"
	"math"
)

type PNGReaderFactory func() (io.Reader, error)

type contextPNGReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextPNGReader) Read(dst []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(dst)
}

// DecodePNG performs bounded two-pass PNG decode into an immutable candidate.
// Each factory call must return a fresh reader for the same exact PNG bytes.
func DecodePNG(ctx context.Context, store *Store, imageID ImageID, open PNGReaderFactory) (*DecodedCandidate, error) {
	if store == nil || imageID == 0 || open == nil {
		return nil, ErrCandidateInvalid
	}
	reader, err := open()
	if err != nil {
		return nil, err
	}
	reader = contextPNGReader{ctx: ctx, reader: reader}
	config, err := png.DecodeConfig(reader)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, ErrCandidateInvalid
	}
	if config.Width <= 0 || config.Height <= 0 {
		return nil, ErrCandidateInvalid
	}
	width, height := uint32(config.Width), uint32(config.Height)
	stride, size, err := CheckedRGBABytes(width, height)
	if err != nil {
		return nil, err
	}
	scanline := uint64(width)*8 + 1
	auxiliary := scanline*2 + uint64(stride)
	if size > (math.MaxUint64-auxiliary)/3 {
		return nil, ErrLimitExceeded
	}
	scratch, err := store.ReserveDecodeScratch(size*3 + auxiliary)
	if err != nil {
		return nil, err
	}
	defer scratch.Close()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	reader, err = open()
	if err != nil {
		return nil, err
	}
	reader = contextPNGReader{ctx: ctx, reader: reader}
	decoded, err := png.Decode(reader)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, ErrCandidateInvalid
	}
	var extra [1]byte
	if n, tailErr := reader.Read(extra[:]); n != 0 || tailErr != io.EOF {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, ErrCandidateInvalid
	}
	if decoded.Bounds().Dx() != int(width) || decoded.Bounds().Dy() != int(height) {
		return nil, ErrCandidateInvalid
	}
	candidate, err := store.NewDecodedCandidate(imageID, width, height)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			candidate.Close()
		}
	}()
	row := make([]byte, int(stride))
	bounds := decoded.Bounds()
	for y := 0; y < int(height); y++ {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < int(width); x++ {
			pixel := color.NRGBAModel.Convert(decoded.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			offset := x * 4
			row[offset], row[offset+1], row[offset+2], row[offset+3] = pixel.R, pixel.G, pixel.B, pixel.A
		}
		if err = candidate.WriteRGBAAt(y*int(stride), row); err != nil {
			return nil, err
		}
	}
	if err = candidate.SealWrites(); err != nil {
		return nil, err
	}
	ok = true
	return candidate, nil
}
