package kitty

import (
	"context"
	"testing"

	"cervterm/internal/termimage"
)

func FuzzKittyDecode(f *testing.F) {
	f.Add([]byte("AQIDBA=="), uint8(32), uint16(1), uint16(1))
	f.Add([]byte("!!!!"), uint8(24), uint16(1), uint16(1))
	f.Fuzz(func(t *testing.T, encoded []byte, format uint8, width, height uint16) {
		if len(encoded) == 0 || len(encoded) > 4096 {
			return
		}
		process := termimage.NewProcessBudget()
		store := termimage.NewStore(process, termimage.DefaultLimits())
		transfer, err := store.BeginTransfer(termimage.Header{Transfer: 1, Image: 1})
		if err != nil {
			t.Fatal(err)
		}
		if err = transfer.Append(encoded); err != nil {
			transfer.Close()
			return
		}
		if err = transfer.Seal(); err != nil {
			transfer.Close()
			return
		}
		pixelFormat := FormatRGBA32
		if format%2 == 1 {
			pixelFormat = FormatRGB24
		}
		job, _ := NewDecodeJob(store, Command{Action: ActionTransmit, Image: 1, Transfer: transfer, Decode: DecodeSpec{Format: pixelFormat, Width: uint32(width%64 + 1), Height: uint32(height%64 + 1)}})
		result := job.Run(context.Background())
		if result.Candidate != nil && result.Failure != ReplyNone {
			t.Fatal("candidate and failure")
		}
		result.Close()
		store.Close()
		if process.Usage() != (termimage.Usage{}) || store.Usage() != (termimage.Usage{}) {
			t.Fatalf("leak=%#v", process.Usage())
		}
	})
}
