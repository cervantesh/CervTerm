package itermimage

import (
	"context"
	"testing"

	"cervterm/internal/termimage"
)

func BenchmarkITermDecodeWorker256x64(b *testing.B) {
	raw, _ := workerPNG(b, 256, 64, false)
	b.ReportAllocs()
	b.SetBytes(256 * 64 * 4)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
		command := sealedWorkerCommand(b, store, raw, SizingIntrinsic, 0)
		job, failure := NewDecodeJob(store, command, DecodeSpec{CellPixelWidth: 8, CellPixelHeight: 16})
		if failure != FailureNone {
			b.Fatal(failure)
		}
		result := job.Run(context.Background())
		if result.Failure != FailureNone || result.Span != (Span{Cols: 32, Rows: 4}) {
			b.Fatalf("result=%#v", result)
		}
		result.Close()
	}
}
