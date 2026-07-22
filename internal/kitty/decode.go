package kitty

import (
	"compress/zlib"
	"context"
	"encoding/base64"
	"errors"
	"io"

	"cervterm/internal/termimage"
)

var errDecodeLimit = errors.New("kitty decode limit")

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(dst []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(dst)
}

type exactByteReader struct{ reader io.Reader }

func (r *exactByteReader) Read(dst []byte) (int, error) { return r.reader.Read(dst) }
func (r *exactByteReader) ReadByte() (byte, error) {
	var one [1]byte
	_, err := io.ReadFull(r.reader, one[:])
	return one[0], err
}
func base64Reader(ctx context.Context, payload *termimage.SealedEncodedPayload) (io.Reader, error) {
	if payload == nil || payload.EncodedLen() == 0 || payload.EncodedLen()%4 != 0 {
		return nil, termimage.ErrCandidateInvalid
	}
	return base64.NewDecoder(base64.StdEncoding.Strict(), contextReader{ctx: ctx, reader: payload.Reader()}), nil
}

func decodeRaw(ctx context.Context, store *termimage.Store, image termimage.ImageID, spec DecodeSpec, payload *termimage.SealedEncodedPayload) (*termimage.DecodedCandidate, error) {
	stride, _, err := termimage.CheckedRGBABytes(spec.Width, spec.Height)
	if err != nil {
		return nil, err
	}
	reader, err := base64Reader(ctx, payload)
	if err != nil {
		return nil, err
	}
	var closer io.Closer
	var compressed *exactByteReader
	if spec.Compression == CompressionZlib {
		compressed = &exactByteReader{reader: reader}
		z, openErr := zlib.NewReader(compressed)
		if openErr != nil {
			return nil, termimage.ErrCandidateInvalid
		}
		reader = z
		closer = z
	} else if spec.Compression != CompressionNone {
		return nil, termimage.ErrCandidateInvalid
	}
	if closer != nil {
		defer closer.Close()
	}
	candidate, err := store.NewDecodedCandidate(image, spec.Width, spec.Height)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			candidate.Close()
		}
	}()
	sourceWidth := int(spec.Width) * 4
	if spec.Format == FormatRGB24 {
		sourceWidth = int(spec.Width) * 3
	} else if spec.Format != FormatRGBA32 {
		return nil, termimage.ErrCandidateInvalid
	}
	row := make([]byte, int(stride))
	for y := uint32(0); y < spec.Height; y++ {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if _, err = io.ReadFull(reader, row[:sourceWidth]); err != nil {
			return nil, termimage.ErrCandidateInvalid
		}
		if spec.Format == FormatRGB24 {
			for x := int(spec.Width) - 1; x >= 0; x-- {
				row[x*4], row[x*4+1], row[x*4+2], row[x*4+3] = row[x*3], row[x*3+1], row[x*3+2], 0xff
			}
		}
		if err = candidate.WriteRGBAAt(int(y)*int(stride), row[:stride]); err != nil {
			return nil, err
		}
	}
	var extra [1]byte
	n, tailErr := reader.Read(extra[:])
	if n != 0 {
		return nil, errDecodeLimit
	}
	if tailErr != io.EOF {
		return nil, termimage.ErrCandidateInvalid
	}
	if compressed != nil {
		var trailing [1]byte
		if n, trailingErr := compressed.Read(trailing[:]); n != 0 || trailingErr != io.EOF {
			return nil, termimage.ErrCandidateInvalid
		}
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = candidate.SealWrites(); err != nil {
		return nil, err
	}
	ok = true
	return candidate, nil
}
