package sixel

import (
	"context"
	"fmt"
	"testing"

	"cervterm/internal/termimage"
)

func FuzzSixelDecodeWorker(f *testing.F) {
	f.Add(uint16(2), uint16(2), uint16(1), byte('@'), uint16(1), uint16(1))
	f.Add(uint16(4097), uint16(1), uint16(4096), byte('~'), uint16(3), uint16(5))
	f.Fuzz(func(t *testing.T, width, height, repeat uint16, char byte, cellW, cellH uint16) {
		w := uint32(width % 5000)
		h := uint32(height % 5000)
		if w == 0 {
			w = 1
		}
		if h == 0 {
			h = 1
		}
		n := uint32(repeat % 4097)
		if n == 0 {
			n = 1
		}
		char = '?' + char%64
		cw := uint32(cellW % 300)
		ch := uint32(cellH % 300)
		if cw == 0 {
			cw = 1
		}
		if ch == 0 {
			ch = 1
		}
		frame := fmt.Sprintf("\"1;1;%d;%d!%d%c", w, h, n, char)
		process := termimage.NewProcessBudget()
		store := termimage.NewStore(process, termimage.DefaultLimits())
		command := sealedCommand(t, store, frame)
		job, failure := NewDecodeJob(store, command, DecodeSpec{CellPixelWidth: cw, CellPixelHeight: ch})
		if failure == FailureNone {
			result := job.Run(context.Background())
			result.Close()
		} else {
			command.Close()
		}
		if store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
			t.Fatalf("usage=%#v", store.Usage())
		}
	})
}
