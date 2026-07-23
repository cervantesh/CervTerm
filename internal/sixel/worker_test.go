package sixel

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"cervterm/internal/termimage"
)

func sealedCommand(t testing.TB, store *termimage.Store, frame string) Command {
	t.Helper()
	adapter := NewAdapter(store)
	out := adapter.Advance(time.Now(), DCSEvent{Data: []byte(frame), Final: true})
	if out.Command == nil || out.Failure != FailureNone {
		t.Fatalf("adapter=%#v", out)
	}
	command := *out.Command
	out.Command.Transfer = nil
	return command
}
func TestSixelDecodeWorkerRendersPaletteAndSpan(t *testing.T) {
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	command := sealedCommand(t, store, `"1;1;2;2#1;2;100;0;0@@`)
	job, failure := NewDecodeJob(store, command, DecodeSpec{CellPixelWidth: 3, CellPixelHeight: 1})
	if failure != FailureNone {
		t.Fatal(failure)
	}
	result := job.Run(context.Background())
	if result.Failure != FailureNone || result.Candidate == nil || result.Span != (Span{Cols: 1, Rows: 2}) {
		t.Fatalf("result=%#v", result)
	}
	want := []byte{255, 0, 0, 255, 255, 0, 0, 255, 0, 0, 0, 0, 0, 0, 0, 0}
	if got := result.Candidate.RGBA(); !bytes.Equal(got, want) {
		t.Fatalf("rgba=%v", got)
	}
	result.Close()
	if store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
		t.Fatalf("usage=%#v", store.Usage())
	}
}
func TestSixelDecodeWorkerTransparentCanvas(t *testing.T) {
	_, store, _ := newAdapterTestStore()
	command := sealedCommand(t, store, `"1;1;2;3`)
	job, _ := NewDecodeJob(store, command, DecodeSpec{CellPixelWidth: 2, CellPixelHeight: 3})
	result := job.Run(context.Background())
	if result.Failure != FailureNone || result.Span != (Span{Cols: 1, Rows: 1}) || !bytes.Equal(result.Candidate.RGBA(), make([]byte, 24)) {
		t.Fatalf("result=%#v", result)
	}
	result.Close()
}
func TestSixelDecodeWorkerRejectsBoundsAndDrawing(t *testing.T) {
	for name, frame := range map[string]string{"dimension": `"1;1;4097;1~`, "outside width": `"1;1;1;1@@`, "outside height": `"1;1;1;1~`, "span": `"1;1;257;1?`} {
		t.Run(name, func(t *testing.T) {
			process := termimage.NewProcessBudget()
			store := termimage.NewStore(process, termimage.DefaultLimits())
			command := sealedCommand(t, store, frame)
			job, f := NewDecodeJob(store, command, DecodeSpec{CellPixelWidth: 1, CellPixelHeight: 1})
			if f != FailureNone {
				t.Fatal(f)
			}
			result := job.Run(context.Background())
			if result.Failure == FailureNone || result.Candidate != nil {
				t.Fatalf("result=%#v", result)
			}
			result.Close()
			if store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
				t.Fatalf("usage=%#v", store.Usage())
			}
		})
	}
}
func TestSixelDecodeWorkerCancellationAndClose(t *testing.T) {
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	command := sealedCommand(t, store, `"1;1;1;1@`)
	job, _ := NewDecodeJob(store, command, DecodeSpec{CellPixelWidth: 1, CellPixelHeight: 1})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := job.Run(ctx)
	if result.Failure != FailureCancelled {
		t.Fatalf("result=%#v", result)
	}
	result.Close()
	job.Close()
	if store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
		t.Fatalf("usage=%#v", store.Usage())
	}
	command = sealedCommand(t, store, `"1;1;1;1@`)
	job, _ = NewDecodeJob(store, command, DecodeSpec{CellPixelWidth: 1, CellPixelHeight: 1})
	job.Close()
	job.Close()
	if store.Usage() != (termimage.Usage{}) {
		t.Fatalf("close usage=%#v", store.Usage())
	}
}
func TestSixelDecodeWorkerRevalidatesAdapterMetadata(t *testing.T) {
	_, store, _ := newAdapterTestStore()
	command := sealedCommand(t, store, `"1;1;2;2@@`)
	command.Raster = Raster{Width: 1, Height: 1}
	job, _ := NewDecodeJob(store, command, DecodeSpec{CellPixelWidth: 1, CellPixelHeight: 1})
	result := job.Run(context.Background())
	if result.Failure != FailureInvalid {
		t.Fatalf("result=%#v", result)
	}
	result.Close()
}

func TestSixelDecodeWorkerRejectsDirectPixelBeforeRaster(t *testing.T) {
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	image, _ := store.AllocateInternalImageID()
	placement, _ := store.AllocateInternalPlacementID()
	transfer, err := store.BeginTransfer(termimage.Header{Transfer: termimage.TransferID(image), Image: image})
	if err != nil {
		t.Fatal(err)
	}
	if err = transfer.Append([]byte{'@'}); err != nil {
		t.Fatal(err)
	}
	if err = transfer.Seal(); err != nil {
		t.Fatal(err)
	}
	job, failure := NewDecodeJob(store, Command{Image: image, Placement: placement, Raster: Raster{Width: 1, Height: 1}, Transfer: transfer}, DecodeSpec{CellPixelWidth: 1, CellPixelHeight: 1})
	if failure != FailureNone {
		t.Fatal(failure)
	}
	result := job.Run(context.Background())
	if result.Failure != FailureInvalid || result.Candidate != nil {
		t.Fatalf("result=%#v", result)
	}
	result.Close()
	if store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
		t.Fatalf("usage=%#v", store.Usage())
	}
}

func TestSixelDecodeWorkerRejectsOperationBomb(t *testing.T) {
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	frame := `"1;1;4096;1` + strings.Repeat("!4096?$", 1025)
	command := sealedCommand(t, store, frame)
	job, failure := NewDecodeJob(store, command, DecodeSpec{CellPixelWidth: 16, CellPixelHeight: 16})
	if failure != FailureNone {
		t.Fatal(failure)
	}
	result := job.Run(context.Background())
	if result.Failure != FailureLimit || result.Candidate != nil {
		t.Fatalf("result=%#v", result)
	}
	result.Close()
	if store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
		t.Fatalf("usage=%#v", store.Usage())
	}
}
