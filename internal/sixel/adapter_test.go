package sixel

import (
	"bytes"
	"testing"
	"time"

	"cervterm/internal/termimage"
)

const validFrame = `"1;1;2;1#1;2;100;0;50!2~$-?`

func newAdapterTestStore() (*termimage.ProcessBudget, *termimage.Store, *Adapter) {
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	return process, store, NewAdapter(store)
}

func TestAdapterSealsExactFrameAcrossEverySplit(t *testing.T) {
	input := []byte(validFrame)
	for split := 0; split <= len(input); split++ {
		process, store, adapter := newAdapterTestStore()
		first := adapter.Advance(time.Now(), DCSEvent{Data: input[:split]})
		if first.Command != nil || first.Failure != FailureNone {
			t.Fatalf("split %d first=%#v", split, first)
		}
		out := adapter.Advance(time.Now(), DCSEvent{Data: input[split:], Final: true})
		if out.Failure != FailureNone || out.Command == nil {
			t.Fatalf("split %d out=%#v", split, out)
		}
		if out.Command.Raster != (Raster{Width: 2, Height: 1}) || out.Command.Image < termimage.MinInternalImageID || out.Command.Placement < termimage.MinInternalPlacementID {
			t.Fatalf("split %d command=%#v", split, out.Command)
		}
		payload, header, _, err := out.Command.Transfer.SealedEncodedCopy(store)
		if err != nil || !bytes.Equal(payload, input) || header.Image != out.Command.Image || header.Transfer != termimage.TransferID(out.Command.Image) {
			t.Fatalf("split %d payload=%q header=%#v err=%v", split, payload, header, err)
		}
		out.Command.Close()
		adapter.Close()
		if store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
			t.Fatalf("split %d usage=%#v", split, store.Usage())
		}
	}
}

func TestScannerExactGrammar(t *testing.T) {
	valid := []string{`"1;1;1;1~`, `#0"1;1;1;1?`, `#255;2;0;100;50"1;1;1;1!1~`, `"1;1;4294967295;1!4096~$-`}
	for _, input := range valid {
		var scan scanner
		if failure := scan.feed([]byte(input)); failure != FailureNone {
			t.Fatalf("valid %q failure=%v", input, failure)
		}
		if failure := scan.finish(); failure != FailureNone {
			t.Fatalf("valid final %q failure=%v", input, failure)
		}
	}
	invalid := map[string]Failure{
		`~`: FailureInvalid, `"1;1;1;1"1;1;1;1~`: FailureInvalid,
		`"2;1;1;1~`: FailureInvalid, `"1;1;0;1~`: FailureInvalid, `"1;1;4294967296;1~`: FailureInvalid,
		`"1;1;1;1!0~`: FailureInvalid, `"1;1;1;1!4097~`: FailureInvalid, `"1;1;1;1!`: FailureInvalid,
		`#256"1;1;1;1~`: FailureInvalid, `#1;2;101;0;0"1;1;1;1~`: FailureInvalid,
		`#1;1;0;0;0"1;1;1;1~`: FailureUnsupported, `#1;3;0;0;0"1;1;1;1~`: FailureInvalid,
		`"1;1;1;1 `: FailureInvalid,
	}
	for input, want := range invalid {
		var scan scanner
		got := scan.feed([]byte(input))
		if got == FailureNone {
			got = scan.finish()
		}
		if got != want {
			t.Fatalf("invalid %q got=%v want=%v", input, got, want)
		}
	}
}

func TestAdapterRejectsRollsBackAndRecovers(t *testing.T) {
	for name, event := range map[string]DCSEvent{
		"invalid":   {Data: []byte(`"1;1;1;1 `)},
		"cancel":    {Data: []byte(`"1;1;1;1`), Cancelled: true},
		"overflow":  {Data: []byte(`"1;1;1;1`), Overflow: true},
		"truncated": {Data: []byte(`"1;1;1;1!2`), Final: true},
	} {
		t.Run(name, func(t *testing.T) {
			process, store, adapter := newAdapterTestStore()
			out := adapter.Advance(time.Now(), event)
			want := FailureInvalid
			if name == "cancel" {
				want = FailureCancelled
			}
			if name == "overflow" {
				want = FailureLimit
			}
			if out.Failure != want {
				t.Fatalf("out=%#v want=%v", out, want)
			}
			if store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
				t.Fatalf("usage=%#v", store.Usage())
			}
			adapter.Advance(time.Now(), DCSEvent{Data: []byte("hidden"), Final: true})
			recovered := adapter.Advance(time.Now(), DCSEvent{Data: []byte(`"1;1;1;1~`), Final: true})
			if recovered.Command == nil {
				t.Fatalf("recovery=%#v", recovered)
			}
			recovered.Command.Close()
		})
	}
}

func TestAdapterAcceptsTransparentDeclaredCanvas(t *testing.T) {
	_, _, adapter := newAdapterTestStore()
	out := adapter.Advance(time.Now(), DCSEvent{Data: []byte(`"1;1;2;3`), Final: true})
	if out.Failure != FailureNone || out.Command == nil || out.Command.Raster != (Raster{Width: 2, Height: 3}) {
		t.Fatalf("out=%#v", out)
	}
	out.Command.Close()
}

func TestAdapterChunkFrameAndPendingBounds(t *testing.T) {
	_, store, adapter := newAdapterTestStore()
	oversized := make([]byte, termimage.HardControlChunkBytes+1)
	if out := adapter.Advance(time.Now(), DCSEvent{Data: oversized}); out.Failure != FailureLimit {
		t.Fatalf("oversized=%#v", out)
	}
	adapter.Advance(time.Now(), DCSEvent{Final: true})

	exactPrefix := []byte(`"1;1;1;1`)
	exact := append(exactPrefix, bytes.Repeat([]byte{'~'}, maxFrameBytes-len(exactPrefix))...)
	bounded := NewAdapter(store)
	commands := 0
	for offset := 0; offset < len(exact); {
		end := offset + int(termimage.HardControlChunkBytes)
		if end > len(exact) {
			end = len(exact)
		}
		out := bounded.Advance(time.Now(), DCSEvent{Data: exact[offset:end], Final: end == len(exact)})
		if out.Failure != FailureNone {
			t.Fatalf("exact frame offset=%d failure=%v", offset, out.Failure)
		}
		if out.Command != nil {
			commands++
			if end != len(exact) {
				t.Fatal("command emitted before final chunk")
			}
			out.Command.Close()
		}
		offset = end
	}
	if commands != 1 {
		t.Fatalf("exact frame commands=%d", commands)
	}
	plusOne := NewAdapter(store)
	for offset := 0; offset < len(exact); {
		end := offset + int(termimage.HardControlChunkBytes)
		if end > len(exact) {
			end = len(exact)
		}
		if out := plusOne.Advance(time.Now(), DCSEvent{Data: exact[offset:end]}); out.Failure != FailureNone {
			t.Fatal(out.Failure)
		}
		offset = end
	}
	if out := plusOne.Advance(time.Now(), DCSEvent{Data: []byte{'~'}}); out.Failure != FailureLimit {
		t.Fatalf("frame +1=%#v", out)
	}

	prefix := []byte(`"1;1;1;1`)
	if out := adapter.Advance(time.Now(), DCSEvent{Data: prefix}); out.Failure != FailureNone {
		t.Fatal(out.Failure)
	}
	for index := uint64(1); index < termimage.HardChunksPerTransfer; index++ {
		if out := adapter.Advance(time.Now(), DCSEvent{Data: []byte("~")}); out.Failure != FailureNone {
			t.Fatalf("chunk %d=%v", index, out.Failure)
		}
	}
	if out := adapter.Advance(time.Now(), DCSEvent{Data: []byte("~")}); out.Failure != FailureLimit {
		t.Fatalf("chunk overflow=%#v", out)
	}
	if store.Usage() != (termimage.Usage{}) {
		t.Fatalf("chunk rollback=%#v", store.Usage())
	}

	process := termimage.NewProcessBudget()
	shared := termimage.NewStore(process, termimage.DefaultLimits())
	adapters := make([]*Adapter, termimage.HardPendingTransfersPerPane)
	for index := range adapters {
		adapters[index] = NewAdapter(shared)
		if out := adapters[index].Advance(time.Now(), DCSEvent{Data: prefix}); out.Failure != FailureNone {
			t.Fatalf("pending %d=%v", index, out.Failure)
		}
	}
	extra := NewAdapter(shared)
	if out := extra.Advance(time.Now(), DCSEvent{Data: prefix}); out.Failure != FailureLimit {
		t.Fatalf("pending overflow=%#v", out)
	}
	for _, current := range adapters {
		current.Close()
	}
	if shared.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
		t.Fatalf("pending cleanup=%#v", shared.Usage())
	}
}

func TestAdapterExpiryResetCloseAndIdempotence(t *testing.T) {
	process, store, adapter := newAdapterTestStore()
	if out := adapter.Advance(time.Now(), DCSEvent{Data: []byte(`"1;1;1;1`)}); out.Failure != FailureNone {
		t.Fatal(out.Failure)
	}
	deadline, ok := adapter.NextExpiry()
	if !ok {
		t.Fatal("deadline missing")
	}
	if out := adapter.Expire(deadline); out.Failure != FailureTimeout {
		t.Fatalf("expire=%#v", out)
	}
	if store.Usage() != (termimage.Usage{}) {
		t.Fatalf("expire usage=%#v", store.Usage())
	}
	adapter.Advance(time.Now(), DCSEvent{Final: true})
	if out := adapter.Advance(time.Now(), DCSEvent{Data: []byte(validFrame), Final: true}); out.Command == nil {
		t.Fatalf("post-expiry=%#v", out)
	} else {
		out.Command.Close()
		out.Command.Close()
	}

	adapter = NewAdapter(store)
	adapter.Advance(time.Now(), DCSEvent{Data: []byte(`"1;1;1;1`)})
	resetDeadline, ok := adapter.NextExpiry()
	if !ok {
		t.Fatal("reset deadline missing")
	}
	store.Reset()
	out := adapter.Expire(resetDeadline)
	if out.Failure != FailureCancelled {
		t.Fatalf("reset expire=%#v", out)
	}
	adapter.Close()
	adapter.Close()
	if store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
		t.Fatalf("final usage=%#v", store.Usage())
	}
}
