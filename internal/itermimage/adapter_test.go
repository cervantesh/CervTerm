package itermimage

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"cervterm/internal/termimage"
)

const validInlineFile = "File=inline=1;size=3:AAAA"

func newAdapterTestStore() (*termimage.ProcessBudget, *termimage.Store, *Adapter) {
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	return process, store, NewAdapter(store)
}

func requireNoUsage(t *testing.T, process *termimage.ProcessBudget, store *termimage.Store) {
	t.Helper()
	if got := store.Usage(); got != (termimage.Usage{}) {
		t.Fatalf("pane usage leaked: %#v", got)
	}
	if got := process.Usage(); got != (termimage.Usage{}) {
		t.Fatalf("process usage leaked: %#v", got)
	}
}

func TestAdapterSealsOnlyBase64AcrossEverySplit(t *testing.T) {
	input := []byte(validInlineFile)
	for split := 0; split <= len(input); split++ {
		process, store, adapter := newAdapterTestStore()
		first := adapter.Advance(time.Now(), OSCEvent{Data: input[:split]})
		if first.Command != nil || first.Failure != FailureNone {
			t.Fatalf("split %d first=%#v", split, first)
		}
		out := adapter.Advance(time.Now(), OSCEvent{Data: input[split:], Final: true})
		if out.Failure != FailureNone || out.Command == nil {
			t.Fatalf("split %d out=%#v", split, out)
		}
		if out.Command.Image != termimage.MinInternalImageID || out.Command.Placement != termimage.MinInternalPlacementID {
			t.Fatalf("split %d ids image=%#x placement=%#x", split, out.Command.Image, out.Command.Placement)
		}
		wantMetadata := Metadata{Size: 3, Axis: SizingIntrinsic, PreserveAspectRatio: true}
		if out.Command.Metadata != wantMetadata {
			t.Fatalf("split %d metadata=%#v want=%#v", split, out.Command.Metadata, wantMetadata)
		}
		payload, header, _, err := out.Command.Transfer.SealedEncodedCopy(store)
		if err != nil || !bytes.Equal(payload, []byte("AAAA")) {
			t.Fatalf("split %d payload=%q err=%v", split, payload, err)
		}
		if header.Image != out.Command.Image || header.Transfer != termimage.TransferID(out.Command.Image) {
			t.Fatalf("split %d header=%#v command=%#v", split, header, out.Command)
		}
		if got := store.Usage(); got != (termimage.Usage{EncodedBytes: 4, PendingTransfers: 1}) {
			t.Fatalf("split %d retained metadata or wrong usage: %#v", split, got)
		}
		out.Command.Close()
		out.Command.Close()
		adapter.Close()
		requireNoUsage(t, process, store)
	}
}

func TestMetadataAcceptsOnlyExactInlineFileSubset(t *testing.T) {
	tests := []struct {
		input string
		want  Metadata
	}{
		{"File=size=3;inline=1:AAAA", Metadata{Size: 3, Axis: SizingIntrinsic, PreserveAspectRatio: true}},
		{"File=inline=1;preserveAspectRatio=1;size=3:AAAA", Metadata{Size: 3, Axis: SizingIntrinsic, PreserveAspectRatio: true}},
		{"File=width=1;size=3;inline=1:AAAA", Metadata{Size: 3, Axis: SizingWidth, Cells: 1, PreserveAspectRatio: true}},
		{"File=inline=1;height=256;size=3;preserveAspectRatio=1:AAAA", Metadata{Size: 3, Axis: SizingHeight, Cells: 256, PreserveAspectRatio: true}},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			process, store, adapter := newAdapterTestStore()
			out := adapter.Advance(time.Now(), OSCEvent{Data: []byte(test.input), Final: true})
			if out.Failure != FailureNone || out.Command == nil || out.Command.Metadata != test.want {
				t.Fatalf("out=%#v want=%#v", out, test.want)
			}
			out.Command.Close()
			adapter.Close()
			requireNoUsage(t, process, store)
		})
	}
}

func TestMetadataRejectsDuplicatesUnknownsExternalModesAndWhitespace(t *testing.T) {
	tests := []string{
		"", "File=", "file=inline=1;size=3:AAAA", "FileX=inline=1;size=3:AAAA",
		"File=inline=1:AAAA", "File=size=3:AAAA", "File=inline=1;size=0:AAAA",
		"File=inline=1;size=-1:AAAA", "File=inline=1;size=+3:AAAA",
		"File=inline=1;size=18446744073709551616:AAAA",
		"File=inline=1;inline=1;size=3:AAAA", "File=inline=1;size=3;size=3:AAAA",
		"File=inline=1;size=3;width=1;width=2:AAAA",
		"File=inline=1;size=3;preserveAspectRatio=1;preserveAspectRatio=1:AAAA",
		"File=inline=1;size=3;unknown=x:AAAA", "File=inline=1;size=3;name=Zg==:AAAA",
		"File=inline=0;size=3:AAAA", "File=inline=2;size=3:AAAA",
		"File=inline=01;size=3:AAAA", "File=inline=0001;size=3:AAAA",
		"File=inline=1;size=3;width=auto:AAAA", "File=inline=1;size=3;width=1px:AAAA",
		"File=inline=1;size=3;width=50%:AAAA", "File=inline=1;size=3;width=0:AAAA",
		"File=inline=1;size=3;width=257:AAAA", "File=inline=1;size=3;height=1;width=1:AAAA",
		"File=inline=1;size=3;preserveAspectRatio=0:AAAA", "File=inline=1;size=3;preserveAspectRatio=2:AAAA",
		"File=inline=1;size=3;preserveAspectRatio=01:AAAA", "File=inline=1;size=3;preserveAspectRatio=0001:AAAA",
		"File=inline=1; size=3:AAAA", "File=inline =1;size=3:AAAA", "File=inline=1;size=3 :AAAA",
		"File=inline=1;size=3;:AAAA", "File=;inline=1;size=3:AAAA", "File=inline=1;;size=3:AAAA",
		"MultipartFile=inline=1;size=3:AAAA", "FilePart=inline=1;size=3:AAAA",
	}
	for index, input := range tests {
		t.Run(strings.ReplaceAll(input, "/", "_"), func(t *testing.T) {
			process, store, adapter := newAdapterTestStore()
			out := adapter.Advance(time.Now(), OSCEvent{Data: []byte(input), Final: true})
			if out.Command != nil || out.Failure == FailureNone {
				t.Fatalf("case %d input=%q accepted: %#v", index, input, out)
			}
			adapter.Close()
			requireNoUsage(t, process, store)
		})
	}
}

func TestStrictPaddedBase64AcceptsCanonicalEncodingsAcrossEverySplit(t *testing.T) {
	for _, encoded := range []string{"AAAA", "AA==", "AAA=", "YWJjZA==", "////", "+/8="} {
		input := []byte("File=inline=1;size=1:" + encoded)
		for split := 0; split <= len(input); split++ {
			process, store, adapter := newAdapterTestStore()
			if out := adapter.Advance(time.Now(), OSCEvent{Data: input[:split]}); out.Failure != FailureNone {
				t.Fatalf("encoded=%q split=%d first=%#v", encoded, split, out)
			}
			out := adapter.Advance(time.Now(), OSCEvent{Data: input[split:], Final: true})
			if out.Failure != FailureNone || out.Command == nil {
				t.Fatalf("encoded=%q split=%d out=%#v", encoded, split, out)
			}
			payload, _, _, err := out.Command.Transfer.SealedEncodedCopy(store)
			if err != nil || string(payload) != encoded {
				t.Fatalf("encoded=%q split=%d payload=%q err=%v", encoded, split, payload, err)
			}
			out.Command.Close()
			adapter.Close()
			requireNoUsage(t, process, store)
		}
	}
}

func TestStrictPaddedBase64RejectsWhitespacePaddingAndPadBits(t *testing.T) {
	for _, encoded := range []string{
		"", "A", "AA", "AAA", "AAAA=", "A===", "=AAA", "AA=A", "====", "AAAA====",
		"AB==", "AAB=", "AA==A", "AA==AAAA", "AAA=A", "YWJjZA=", "YWJjZA===",
		"AA A=", "AA\tA=", "AA\nA=", "AA\rA=", "AA-A", "AA_A", "AA.A",
	} {
		t.Run(strings.ReplaceAll(encoded, "/", "_"), func(t *testing.T) {
			process, store, adapter := newAdapterTestStore()
			out := adapter.Advance(time.Now(), OSCEvent{Data: []byte("File=inline=1;size=1:" + encoded), Final: true})
			if out.Command != nil || out.Failure != FailureInvalid {
				t.Fatalf("encoded=%q out=%#v", encoded, out)
			}
			adapter.Close()
			requireNoUsage(t, process, store)
		})
	}
}

func TestAdapterReservesAfterMetadataAndRollsBackLateFailure(t *testing.T) {
	process, store, adapter := newAdapterTestStore()
	if out := adapter.Advance(time.Now(), OSCEvent{Data: []byte("File=inline=1;size=3:")}); out.Failure != FailureNone {
		t.Fatal(out.Failure)
	}
	requireNoUsage(t, process, store)
	if out := adapter.Advance(time.Now(), OSCEvent{Data: []byte("AAAA")}); out.Failure != FailureNone {
		t.Fatal(out.Failure)
	}
	if got := store.Usage(); got != (termimage.Usage{EncodedBytes: 4, PendingTransfers: 1}) {
		t.Fatalf("usage after base64=%#v", got)
	}
	if out := adapter.Advance(time.Now(), OSCEvent{Data: []byte("!")}); out.Failure != FailureInvalid {
		t.Fatalf("late invalid=%#v", out)
	}
	requireNoUsage(t, process, store)
	if out := adapter.Advance(time.Now(), OSCEvent{Final: true}); out != (Outcome{}) {
		t.Fatalf("discard final=%#v", out)
	}
	out := adapter.Advance(time.Now(), OSCEvent{Data: []byte(validInlineFile), Final: true})
	if out.Command == nil || out.Failure != FailureNone {
		t.Fatalf("recovery=%#v", out)
	}
	out.Command.Close()
	adapter.Close()
	requireNoUsage(t, process, store)
}

func TestAdapterChunkEncodedAndPendingBounds(t *testing.T) {
	t.Run("control chunk", func(t *testing.T) {
		process, store, adapter := newAdapterTestStore()
		oversized := bytes.Repeat([]byte{'A'}, int(termimage.HardControlChunkBytes)+1)
		if out := adapter.Advance(time.Now(), OSCEvent{Data: oversized}); out.Failure != FailureLimit {
			t.Fatalf("out=%#v", out)
		}
		requireNoUsage(t, process, store)
	})

	t.Run("encoded pane", func(t *testing.T) {
		process := termimage.NewProcessBudget()
		limits := termimage.DefaultLimits()
		limits.EncodedBytes = 8
		store := termimage.NewStore(process, limits)
		adapter := NewAdapter(store)
		if out := adapter.Advance(time.Now(), OSCEvent{Data: []byte("File=inline=1;size=1:AAAAAAAA")}); out.Failure != FailureNone {
			t.Fatal(out.Failure)
		}
		if got := store.Usage(); got != (termimage.Usage{EncodedBytes: 8, PendingTransfers: 1}) {
			t.Fatalf("exact usage=%#v", got)
		}
		if out := adapter.Advance(time.Now(), OSCEvent{Data: []byte("AAAA")}); out.Failure != FailureLimit {
			t.Fatalf("overflow=%#v", out)
		}
		requireNoUsage(t, process, store)
	})

	t.Run("chunks", func(t *testing.T) {
		process, store, adapter := newAdapterTestStore()
		if out := adapter.Advance(time.Now(), OSCEvent{Data: []byte("File=inline=1;size=1:")}); out.Failure != FailureNone {
			t.Fatal(out.Failure)
		}
		for index := uint64(0); index < termimage.HardChunksPerTransfer; index++ {
			if out := adapter.Advance(time.Now(), OSCEvent{Data: []byte("AAAA")}); out.Failure != FailureNone {
				t.Fatalf("chunk %d=%#v", index, out)
			}
		}
		if out := adapter.Advance(time.Now(), OSCEvent{Data: []byte("AAAA")}); out.Failure != FailureLimit {
			t.Fatalf("chunk overflow=%#v", out)
		}
		requireNoUsage(t, process, store)
	})

	t.Run("pending pane", func(t *testing.T) {
		process := termimage.NewProcessBudget()
		store := termimage.NewStore(process, termimage.DefaultLimits())
		adapters := make([]*Adapter, termimage.HardPendingTransfersPerPane)
		for index := range adapters {
			adapters[index] = NewAdapter(store)
			if out := adapters[index].Advance(time.Now(), OSCEvent{Data: []byte(validInlineFile)}); out.Failure != FailureNone {
				t.Fatalf("pending %d=%#v", index, out)
			}
		}
		before := store.Usage()
		extra := NewAdapter(store)
		if out := extra.Advance(time.Now(), OSCEvent{Data: []byte(validInlineFile)}); out.Failure != FailureLimit {
			t.Fatalf("pending overflow=%#v", out)
		}
		if store.Usage() != before || process.Usage() != before {
			t.Fatalf("failed pending retained bytes pane=%#v process=%#v before=%#v", store.Usage(), process.Usage(), before)
		}
		for _, adapter := range adapters {
			adapter.Close()
		}
		extra.Close()
		requireNoUsage(t, process, store)
	})
}

func TestAdapterProcessPendingAndEncodedBoundsRollBackExactly(t *testing.T) {
	process := termimage.NewProcessBudget()

	var pendingAdapters []*Adapter
	var pendingStores []*termimage.Store
	for index := uint64(0); index < termimage.HardPendingTransfersProcess; index++ {
		store := termimage.NewStore(process, termimage.DefaultLimits())
		adapter := NewAdapter(store)
		if out := adapter.Advance(time.Now(), OSCEvent{Data: []byte(validInlineFile)}); out.Failure != FailureNone {
			t.Fatalf("pending process %d=%#v", index, out)
		}
		pendingStores = append(pendingStores, store)
		pendingAdapters = append(pendingAdapters, adapter)
	}
	extraStore := termimage.NewStore(process, termimage.DefaultLimits())
	extra := NewAdapter(extraStore)
	before := process.Usage()
	if out := extra.Advance(time.Now(), OSCEvent{Data: []byte(validInlineFile)}); out.Failure != FailureLimit {
		t.Fatalf("process pending overflow=%#v", out)
	}
	if process.Usage() != before || extraStore.Usage() != (termimage.Usage{}) {
		t.Fatalf("process pending rollback process=%#v extra=%#v before=%#v", process.Usage(), extraStore.Usage(), before)
	}
	for _, adapter := range pendingAdapters {
		adapter.Close()
	}
	extra.Close()
	for _, store := range pendingStores {
		if store.Usage() != (termimage.Usage{}) {
			t.Fatalf("pending store leaked: %#v", store.Usage())
		}
	}
	if process.Usage() != (termimage.Usage{}) {
		t.Fatalf("pending process leaked: %#v", process.Usage())
	}

	var encodedAdapters []*Adapter
	var encodedStores []*termimage.Store
	chunk := bytes.Repeat([]byte{'A'}, int(termimage.HardControlChunkBytes))
	for pane := uint64(0); pane < termimage.HardEncodedBytesProcess/termimage.HardEncodedBytesPerPane; pane++ {
		store := termimage.NewStore(process, termimage.DefaultLimits())
		adapter := NewAdapter(store)
		if out := adapter.Advance(time.Now(), OSCEvent{Data: []byte("File=inline=1;size=1:")}); out.Failure != FailureNone {
			t.Fatal(out.Failure)
		}
		for retained := uint64(0); retained < termimage.HardEncodedBytesPerPane; retained += uint64(len(chunk)) {
			if out := adapter.Advance(time.Now(), OSCEvent{Data: chunk}); out.Failure != FailureNone {
				t.Fatalf("encoded pane=%d retained=%d out=%#v", pane, retained, out)
			}
		}
		encodedStores = append(encodedStores, store)
		encodedAdapters = append(encodedAdapters, adapter)
	}
	extraStore = termimage.NewStore(process, termimage.DefaultLimits())
	extra = NewAdapter(extraStore)
	if out := extra.Advance(time.Now(), OSCEvent{Data: []byte("File=inline=1;size=1:")}); out.Failure != FailureNone {
		t.Fatal(out.Failure)
	}
	before = process.Usage()
	if out := extra.Advance(time.Now(), OSCEvent{Data: []byte("AAAA")}); out.Failure != FailureLimit {
		t.Fatalf("process encoded overflow=%#v", out)
	}
	if process.Usage() != before || extraStore.Usage() != (termimage.Usage{}) {
		t.Fatalf("process encoded rollback process=%#v extra=%#v before=%#v", process.Usage(), extraStore.Usage(), before)
	}
	for _, adapter := range encodedAdapters {
		adapter.Close()
	}
	extra.Close()
	for _, store := range encodedStores {
		if store.Usage() != (termimage.Usage{}) {
			t.Fatalf("encoded store leaked: %#v", store.Usage())
		}
	}
	if process.Usage() != (termimage.Usage{}) {
		t.Fatalf("encoded process leaked: %#v", process.Usage())
	}
}

func TestAdapterCancellationOverflowExpiryResetDiscardAndClose(t *testing.T) {
	for name, event := range map[string]OSCEvent{
		"cancel":   {Data: []byte(validInlineFile), Cancelled: true},
		"overflow": {Data: []byte(validInlineFile), Overflow: true},
	} {
		t.Run(name, func(t *testing.T) {
			process, store, adapter := newAdapterTestStore()
			out := adapter.Advance(time.Now(), event)
			want := FailureCancelled
			if name == "overflow" {
				want = FailureLimit
			}
			if out.Command != nil || out.Failure != want {
				t.Fatalf("out=%#v want=%v", out, want)
			}
			requireNoUsage(t, process, store)
			if ignored := adapter.Advance(time.Now(), OSCEvent{Data: []byte("secret"), Final: true}); ignored != (Outcome{}) {
				t.Fatalf("discard=%#v", ignored)
			}
			recovered := adapter.Advance(time.Now(), OSCEvent{Data: []byte(validInlineFile), Final: true})
			if recovered.Command == nil {
				t.Fatalf("recovery=%#v", recovered)
			}
			recovered.Command.Close()
			adapter.Close()
			requireNoUsage(t, process, store)
		})
	}

	process, store, adapter := newAdapterTestStore()
	if out := adapter.Advance(time.Now(), OSCEvent{Data: []byte(validInlineFile)}); out.Failure != FailureNone {
		t.Fatal(out.Failure)
	}
	deadline, ok := adapter.NextExpiry()
	if !ok {
		t.Fatal("missing expiry")
	}
	if out := adapter.Expire(deadline.Add(-time.Nanosecond)); out != (Outcome{}) {
		t.Fatalf("early expiry=%#v", out)
	}
	if out := adapter.Expire(deadline); out.Failure != FailureTimeout {
		t.Fatalf("expiry=%#v", out)
	}
	requireNoUsage(t, process, store)
	adapter.Advance(time.Now(), OSCEvent{Final: true})

	adapter = NewAdapter(store)
	if out := adapter.Advance(time.Now(), OSCEvent{Data: []byte(validInlineFile)}); out.Failure != FailureNone {
		t.Fatal(out.Failure)
	}
	deadline, ok = adapter.NextExpiry()
	if !ok {
		t.Fatal("missing reset expiry")
	}
	store.Reset()
	if out := adapter.Expire(deadline); out.Failure != FailureCancelled {
		t.Fatalf("reset expiry=%#v", out)
	}
	adapter.Close()
	adapter.Close()
	requireNoUsage(t, process, store)
}
