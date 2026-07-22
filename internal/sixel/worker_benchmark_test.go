package sixel

import (
	"context"
	"strings"
	"testing"

	"cervterm/internal/termimage"
)

func BenchmarkSixelDecodeWorker256x64(b *testing.B) {
	frame := `"1;1;256;64#1;2;100;50;0` + strings.Repeat("!256~-", 10) + "!256B"
	b.ReportAllocs()
	b.SetBytes(256 * 64 * 4)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
		command := sealedCommand(b, store, frame)
		job, failure := NewDecodeJob(store, command, DecodeSpec{CellPixelWidth: 8, CellPixelHeight: 16})
		if failure != FailureNone {
			b.Fatal(failure)
		}
		result := job.Run(context.Background())
		if result.Failure != FailureNone {
			b.Fatal(result.Failure)
		}
		result.Close()
	}
}
