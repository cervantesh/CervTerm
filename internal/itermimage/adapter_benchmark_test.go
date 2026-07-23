package itermimage

import (
	"bytes"
	"testing"
	"time"

	"cervterm/internal/termimage"
)

func benchmarkInlineFile(encodedBytes int) []byte {
	prefix := []byte("File=inline=1;size=196608;width=80;preserveAspectRatio=1:")
	return append(prefix, bytes.Repeat([]byte{'A'}, encodedBytes)...)
}

func BenchmarkITermScanner256KiB(b *testing.B) {
	input := benchmarkInlineFile(256 * 1024)
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		var scan scanner
		if _, failure := scan.feed(input); failure != FailureNone || scan.finish() != FailureNone {
			b.Fatal("scan failed")
		}
	}
}

func BenchmarkITermAdapterSeal256KiB(b *testing.B) {
	input := benchmarkInlineFile(256 * 1024)
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
		adapter := NewAdapter(store)
		for offset := 0; offset < len(input); {
			end := offset + int(termimage.HardControlChunkBytes)
			if end > len(input) {
				end = len(input)
			}
			out := adapter.Advance(time.Now(), OSCEvent{Data: input[offset:end], Final: end == len(input)})
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
