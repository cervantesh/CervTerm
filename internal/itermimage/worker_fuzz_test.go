package itermimage

import (
	"context"
	"encoding/base64"
	"testing"

	"cervterm/internal/termimage"
)

func FuzzITermDecodeWorker(f *testing.F) {
	raw, _ := workerPNG(f, 2, 2, true)
	valid := base64.StdEncoding.EncodeToString(raw)
	f.Add([]byte(valid), uint32(len(raw)), byte(SizingIntrinsic), uint16(0), uint16(8), uint16(16), true)
	f.Add([]byte(valid+"===="), uint32(len(raw)), byte(SizingWidth), uint16(3), uint16(2), uint16(3), true)
	f.Add([]byte("!!!!"), uint32(3), byte(SizingHeight), uint16(1), uint16(1), uint16(1), true)
	f.Add([]byte(base64.StdEncoding.EncodeToString(append(append([]byte(nil), raw...), raw...))), uint32(len(raw)*2), byte(SizingIntrinsic), uint16(0), uint16(1), uint16(1), true)

	f.Fuzz(func(t *testing.T, encoded []byte, declared uint32, axis byte, cells, cellW, cellH uint16, preserve bool) {
		if len(encoded) > 64*1024 {
			return
		}
		process := termimage.NewProcessBudget()
		store := termimage.NewStore(process, termimage.DefaultLimits())
		metadata := Metadata{
			Size:                uint64(declared),
			Axis:                SizingAxis(axis % 5),
			Cells:               cells % 300,
			PreserveAspectRatio: preserve,
		}
		command := directWorkerCommand(t, store, encoded, metadata)
		spec := DecodeSpec{CellPixelWidth: uint32(cellW % 5000), CellPixelHeight: uint32(cellH % 5000)}
		job, failure := NewDecodeJob(store, command, spec)
		if failure == FailureNone {
			result := job.Run(context.Background())
			if result.Failure == FailureNone {
				if result.Candidate == nil || !result.Candidate.WritesSealed() || result.Span.Cols == 0 || result.Span.Rows == 0 || result.Span.Cols > uint32(termimage.HardPlacementSpan) || result.Span.Rows > uint32(termimage.HardPlacementSpan) {
					t.Fatalf("invalid success: %#v", result)
				}
			} else if result.Candidate != nil {
				t.Fatalf("failure retained candidate: %#v", result)
			}
			result.Close()
			job.Close()
		} else {
			command.Close()
		}
		if store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
			t.Fatalf("ownership leak: pane=%#v process=%#v", store.Usage(), process.Usage())
		}
	})
}
