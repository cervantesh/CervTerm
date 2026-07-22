package kitty

import (
	"context"
	"image/color"
	"image/png"
	"io"
	"math"

	"cervterm/internal/termimage"
)

func decodePNG(ctx context.Context, store *termimage.Store, imageID termimage.ImageID, payload *termimage.SealedEncodedPayload) (*termimage.DecodedCandidate, error) {
	reader, err := base64Reader(ctx, payload)
	if err != nil {
		return nil, err
	}
	config, err := png.DecodeConfig(reader)
	if err != nil {
		return nil, termimage.ErrCandidateInvalid
	}
	if config.Width <= 0 || config.Height <= 0 {
		return nil, termimage.ErrCandidateInvalid
	}
	width, height := uint32(config.Width), uint32(config.Height)
	stride, size, err := termimage.CheckedRGBABytes(width, height)
	if err != nil {
		return nil, err
	}
	scanline := uint64(width)*8 + 1
	auxiliary := scanline*2 + uint64(stride)
	if size > (math.MaxUint64-auxiliary)/3 {
		return nil, errDecodeLimit
	}
	scratchBytes := size*3 + auxiliary
	scratch, err := store.ReserveDecodeScratch(scratchBytes)
	if err != nil {
		return nil, err
	}
	defer scratch.Close()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	reader, err = base64Reader(ctx, payload)
	if err != nil {
		return nil, err
	}
	decoded, err := png.Decode(reader)
	if err != nil {
		return nil, termimage.ErrCandidateInvalid
	}
	var extra [1]byte
	if n, tailErr := reader.Read(extra[:]); n != 0 || tailErr != io.EOF {
		return nil, termimage.ErrCandidateInvalid
	}
	if decoded.Bounds().Dx() != int(width) || decoded.Bounds().Dy() != int(height) {
		return nil, termimage.ErrCandidateInvalid
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
