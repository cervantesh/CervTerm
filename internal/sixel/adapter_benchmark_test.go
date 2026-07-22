package sixel

import (
	"bytes"
	"testing"
	"time"

	"cervterm/internal/termimage"
)

func benchmarkSixelFrame() []byte {
	prefix := []byte(`"1;1;4096;64`)
	return append(prefix, bytes.Repeat([]byte{'~'}, maxFrameBytes-len(prefix))...)
}

func BenchmarkSixelTokenizer256KiB(b *testing.B) {
	frame := benchmarkSixelFrame()
	b.SetBytes(int64(len(frame)))
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		var scan scanner
		if scan.feed(frame) != FailureNone || scan.finish() != FailureNone {
			b.Fatal("scan")
		}
	}
}

func BenchmarkSixelAdapterSeal256KiB(b *testing.B) {
	frame := benchmarkSixelFrame()
	b.SetBytes(int64(len(frame)))
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
		adapter := NewAdapter(store)
		for offset := 0; offset < len(frame); {
			end := offset + int(termimage.HardControlChunkBytes)
			if end > len(frame) {
				end = len(frame)
			}
			out := adapter.Advance(time.Now(), DCSEvent{Data: frame[offset:end], Final: end == len(frame)})
			if out.Failure != FailureNone {
				b.Fatal(out.Failure)
			}
			if out.Command != nil {
				out.Command.Close()
			}
			offset = end
		}
		adapter.Close()
	}
}
