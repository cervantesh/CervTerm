package kitty

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"

	"cervterm/internal/termimage"
)

func decodeTestCommand(t *testing.T, store *termimage.Store, imageID termimage.ImageID, spec DecodeSpec, payload []byte) Command {
	t.Helper()
	transfer, err := store.BeginTransfer(termimage.Header{Transfer: termimage.TransferID(imageID), Image: imageID})
	if err != nil {
		t.Fatal(err)
	}
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(payload)))
	base64.StdEncoding.Encode(encoded, payload)
	if err = transfer.Append(encoded); err != nil {
		t.Fatal(err)
	}
	if err = transfer.Seal(); err != nil {
		t.Fatal(err)
	}
	return Command{Action: ActionTransmit, Image: imageID, Transfer: transfer, Decode: spec}
}

func TestDecodeRawRGBAAndRGBSealed(t *testing.T) {
	for _, tc := range []struct {
		name        string
		format      PixelFormat
		input, want []byte
	}{
		{"rgba", FormatRGBA32, []byte{1, 2, 3, 4}, []byte{1, 2, 3, 4}},
		{"rgb", FormatRGB24, []byte{5, 6, 7}, []byte{5, 6, 7, 255}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			process := termimage.NewProcessBudget()
			store := termimage.NewStore(process, termimage.DefaultLimits())
			job, code := NewDecodeJob(store, decodeTestCommand(t, store, 1, DecodeSpec{Format: tc.format, Width: 1, Height: 1}, tc.input))
			if code != ReplyNone {
				t.Fatal(code)
			}
			result := job.Run(context.Background())
			if result.Failure != ReplyNone || result.Candidate == nil {
				t.Fatalf("result=%#v", result)
			}
			if got := result.Candidate.RGBA(); !bytes.Equal(got, tc.want) {
				t.Fatalf("rgba=%v", got)
			}
			if err := result.Candidate.WriteRGBAAt(0, []byte{9}); err == nil {
				t.Fatal("sealed candidate writable")
			}
			result.Close()
			if process.Usage() != (termimage.Usage{}) {
				t.Fatalf("leak=%#v", process.Usage())
			}
		})
	}
}

func TestDecodeZlibRGBA(t *testing.T) {
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	_, _ = writer.Write([]byte{1, 2, 3, 4})
	_ = writer.Close()
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	command := decodeTestCommand(t, store, 1, DecodeSpec{Format: FormatRGBA32, Compression: CompressionZlib, Width: 1, Height: 1}, compressed.Bytes())
	job, _ := NewDecodeJob(store, command)
	result := job.Run(context.Background())
	if result.Failure != ReplyNone || !bytes.Equal(result.Candidate.RGBA(), []byte{1, 2, 3, 4}) {
		t.Fatalf("result=%#v", result)
	}
	result.Close()
}

func TestDecodePNGAndRollbackInvalid(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	source.SetNRGBA(0, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 40})
	var encodedPNG bytes.Buffer
	if err := png.Encode(&encodedPNG, source); err != nil {
		t.Fatal(err)
	}
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	job, _ := NewDecodeJob(store, decodeTestCommand(t, store, 1, DecodeSpec{Format: FormatPNG}, encodedPNG.Bytes()))
	result := job.Run(context.Background())
	if result.Failure != ReplyNone || !bytes.Equal(result.Candidate.RGBA(), []byte{10, 20, 30, 40}) {
		t.Fatalf("result=%#v rgba=%v", result, result.Candidate.RGBA())
	}
	result.Close()
	job, _ = NewDecodeJob(store, decodeTestCommand(t, store, 2, DecodeSpec{Format: FormatPNG}, []byte("bad")))
	result = job.Run(context.Background())
	if result.Failure != ReplyInvalid || result.Candidate != nil {
		t.Fatalf("invalid=%#v", result)
	}
	if process.Usage() != (termimage.Usage{}) {
		t.Fatalf("rollback=%#v", process.Usage())
	}
}

func TestDecodeJobCancellationAndClose(t *testing.T) {
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	job, _ := NewDecodeJob(store, decodeTestCommand(t, store, 1, DecodeSpec{Format: FormatRGBA32, Width: 1, Height: 1}, []byte{1, 2, 3, 4}))
	job.Close()
	if result := job.Run(context.Background()); result.Failure != ReplyCancelled {
		t.Fatal(result)
	}
	if process.Usage() != (termimage.Usage{}) {
		t.Fatal(process.Usage())
	}
}

func TestDecodeRejectsMalformedShortLongAndTrailing(t *testing.T) {
	for _, tc := range []struct {
		name        string
		encoded     string
		compression Compression
		want        ReplyCode
	}{
		{"bad base64", "!!!!", CompressionNone, ReplyInvalid},
		{"short", base64.StdEncoding.EncodeToString([]byte{1, 2, 3}), CompressionNone, ReplyInvalid},
		{"long", base64.StdEncoding.EncodeToString([]byte{1, 2, 3, 4, 5}), CompressionNone, ReplyLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			process := termimage.NewProcessBudget()
			store := termimage.NewStore(process, termimage.DefaultLimits())
			transfer, err := store.BeginTransfer(termimage.Header{Transfer: 1, Image: 1})
			if err != nil {
				t.Fatal(err)
			}
			if err = transfer.Append([]byte(tc.encoded)); err != nil {
				t.Fatal(err)
			}
			if err = transfer.Seal(); err != nil {
				t.Fatal(err)
			}
			job, _ := NewDecodeJob(store, Command{Action: ActionTransmit, Image: 1, Transfer: transfer, Decode: DecodeSpec{Format: FormatRGBA32, Compression: tc.compression, Width: 1, Height: 1}})
			result := job.Run(context.Background())
			if result.Failure != tc.want || result.Candidate != nil {
				t.Fatalf("result=%#v", result)
			}
			if process.Usage() != (termimage.Usage{}) {
				t.Fatalf("leak=%#v", process.Usage())
			}
		})
	}
}

func TestDecodeZlibBombAndTrailingRollback(t *testing.T) {
	for _, trailing := range []bool{false, true} {
		var compressed bytes.Buffer
		writer := zlib.NewWriter(&compressed)
		_, _ = writer.Write(bytes.Repeat([]byte{1}, 1024))
		_ = writer.Close()
		if trailing {
			compressed.WriteByte(0)
		}
		process := termimage.NewProcessBudget()
		store := termimage.NewStore(process, termimage.DefaultLimits())
		job, _ := NewDecodeJob(store, decodeTestCommand(t, store, 1, DecodeSpec{Format: FormatRGBA32, Compression: CompressionZlib, Width: 1, Height: 1}, compressed.Bytes()))
		result := job.Run(context.Background())
		if result.Failure == ReplyNone || result.Candidate != nil {
			t.Fatalf("trailing=%v result=%#v", trailing, result)
		}
		if process.Usage() != (termimage.Usage{}) {
			t.Fatalf("leak=%#v", process.Usage())
		}
	}
}
