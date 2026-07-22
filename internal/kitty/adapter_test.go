package kitty

import (
	"math"
	"testing"
	"time"

	"cervterm/internal/termimage"
)

func testAdapter(t *testing.T) (*Adapter, *termimage.Store, *termimage.ProcessBudget) {
	t.Helper()
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	return NewAdapter(store), store, process
}

func TestAdapterOneShotTransferOwnership(t *testing.T) {
	a, store, process := testAdapter(t)
	now := time.Unix(10, 0)
	out := a.Advance(now, APCEvent{Data: []byte("Ga=t,i=1,s=1,v=1;AAAA"), Final: true})
	if out.Command == nil || out.Failure != ReplyNone || out.Command.Transfer == nil {
		t.Fatalf("out=%#v", out)
	}
	encoded, err := out.Command.Transfer.EncodedCopy()
	if err != nil || string(encoded) != "AAAA" {
		t.Fatalf("encoded=%q err=%v", encoded, err)
	}
	out.Command.Transfer.Close()
	if store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
		t.Fatal("ownership leaked")
	}
}

func TestAdapterChunkingPreservesOrderAndRollsBackMalformed(t *testing.T) {
	a, store, _ := testAdapter(t)
	now := time.Unix(20, 0)
	if out := a.Advance(now, APCEvent{Data: []byte("Ga=t,i=2,s=2,v=1,m=1;QUJD"), Final: true}); out != (Outcome{}) {
		t.Fatalf("initial=%#v", out)
	}
	out := a.Advance(now.Add(time.Second), APCEvent{Data: []byte("Gm=0;RA=="), Final: true})
	if out.Command == nil {
		t.Fatalf("final=%#v", out)
	}
	encoded, _ := out.Command.Transfer.EncodedCopy()
	if string(encoded) != "QUJDRA==" {
		t.Fatalf("encoded=%q", encoded)
	}
	out.Command.Transfer.Close()
	if out = a.Advance(now, APCEvent{Data: []byte("Ga=t,i=3,s=1,v=1,m=1;AAAA"), Final: true}); out != (Outcome{}) {
		t.Fatal(out)
	}
	out = a.Advance(now, APCEvent{Data: []byte("Gm=1,a=t;AAAA"), Final: true})
	if out.Failure == ReplyNone || out.Command != nil || store.Usage() != (termimage.Usage{}) {
		t.Fatalf("rollback=%#v usage=%#v", out, store.Usage())
	}
}

func TestAdapterExpiryIsPureBoundedAndIdempotent(t *testing.T) {
	a, store, _ := testAdapter(t)
	now := time.Now()
	a.Advance(now, APCEvent{Data: []byte("Ga=t,i=4,s=1,v=1,m=1;AAAA"), Final: true})
	deadline, ok := a.NextExpiry()
	if !ok || deadline.Before(now.Add(termimage.HardTransferLifetime)) || deadline.After(now.Add(termimage.HardTransferLifetime+time.Second)) {
		t.Fatalf("deadline=%v %v", deadline, ok)
	}
	if out := a.Expire(deadline.Add(-time.Nanosecond)); out != (Outcome{}) {
		t.Fatal(out)
	}
	out := a.Expire(deadline)
	if out.Failure != ReplyTimeout || store.Usage() != (termimage.Usage{}) {
		t.Fatalf("out=%#v usage=%#v", out, store.Usage())
	}
	if out = a.Expire(deadline.Add(time.Second)); out != (Outcome{}) {
		t.Fatal("repeated expiry emitted")
	}
}

func TestAdapterExpiryClearsExternallyClosedTransfer(t *testing.T) {
	a, store, _ := testAdapter(t)
	now := time.Now()
	a.Advance(now, APCEvent{Data: []byte("Ga=t,i=4,s=1,v=1,m=1;AAAA"), Final: true})
	deadline, ok := a.NextExpiry()
	if !ok {
		t.Fatal("missing deadline")
	}
	a.active.Close()
	out := a.Expire(deadline)
	if out.Failure != ReplyCancelled || store.Usage() != (termimage.Usage{}) {
		t.Fatalf("out=%#v usage=%#v", out, store.Usage())
	}
	if deadline, ok = a.NextExpiry(); ok || !deadline.IsZero() {
		t.Fatalf("stale deadline=%v ok=%v", deadline, ok)
	}
}

func TestAdapterFragmentationAndCancellation(t *testing.T) {
	a, store, _ := testAdapter(t)
	now := time.Unix(40, 0)
	input := []byte("Ga=q,i=5,f=100;AAAA")
	a.Advance(now, APCEvent{Data: input[:7]})
	out := a.Advance(now, APCEvent{Data: input[7:], Final: true})
	if out.Command == nil || out.Command.Action != ActionQuery {
		t.Fatalf("fragmented=%#v", out)
	}
	out.Command.Transfer.Close()
	a.Advance(now, APCEvent{Data: []byte("Ga=t,i=6,s=1,v=1,m=1;AAAA"), Final: true})
	out = a.Advance(now, APCEvent{Cancelled: true})
	if out.Failure != ReplyCancelled || store.Usage() != (termimage.Usage{}) {
		t.Fatalf("cancel=%#v usage=%#v", out, store.Usage())
	}
}

func TestAdapterTransferIDExhaustionAndClose(t *testing.T) {
	a, store, _ := testAdapter(t)
	a.nextTransfer = math.MaxUint32
	out := a.Advance(time.Now(), APCEvent{Data: []byte("Ga=t,i=1,s=1,v=1;AAAA"), Final: true})
	if out.Failure != ReplyLimit || store.Usage() != (termimage.Usage{}) {
		t.Fatalf("out=%#v", out)
	}
	a.Close()
	a.Close()
}

func TestAdapterPaneAndProcessPendingTransferBounds(t *testing.T) {
	now := time.Unix(50, 0)
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	var adapters []*Adapter
	for index := 0; index < int(termimage.HardPendingTransfersPerPane); index++ {
		a := NewAdapter(store)
		a.nextTransfer = uint64(index)
		out := a.Advance(now, APCEvent{Data: []byte("Ga=t,i=" + string(rune('1'+index)) + ",s=1,v=1,m=1;AAAA"), Final: true})
		if out != (Outcome{}) {
			t.Fatalf("pane reservation %d=%#v", index, out)
		}
		adapters = append(adapters, a)
	}
	extra := NewAdapter(store)
	extra.nextTransfer = termimage.HardPendingTransfersPerPane
	if out := extra.Advance(now, APCEvent{Data: []byte("Ga=t,i=99,s=1,v=1,m=1;AAAA"), Final: true}); out.Failure != ReplyLimit {
		t.Fatalf("pane overflow=%#v", out)
	}
	for _, a := range adapters {
		a.Close()
	}
	if process.Usage() != (termimage.Usage{}) {
		t.Fatalf("pane cleanup=%#v", process.Usage())
	}

	var stores []*termimage.Store
	adapters = nil
	for index := 0; index < int(termimage.HardPendingTransfersProcess); index++ {
		if index%int(termimage.HardPendingTransfersPerPane) == 0 {
			stores = append(stores, termimage.NewStore(process, termimage.DefaultLimits()))
		}
		a := NewAdapter(stores[len(stores)-1])
		a.nextTransfer = uint64(index % int(termimage.HardPendingTransfersPerPane))
		out := a.Advance(now, APCEvent{Data: []byte("Ga=t,i=1,s=1,v=1,m=1;AAAA"), Final: true})
		if out != (Outcome{}) {
			t.Fatalf("process reservation %d=%#v", index, out)
		}
		adapters = append(adapters, a)
	}
	extraStore := termimage.NewStore(process, termimage.DefaultLimits())
	if out := NewAdapter(extraStore).Advance(now, APCEvent{Data: []byte("Ga=t,i=1,s=1,v=1,m=1;AAAA"), Final: true}); out.Failure != ReplyLimit {
		t.Fatalf("process overflow=%#v", out)
	}
	for _, a := range adapters {
		a.Close()
	}
	if process.Usage() != (termimage.Usage{}) {
		t.Fatalf("process cleanup=%#v", process.Usage())
	}
}

func TestAdapterLogicalFrameLimitRejectsAndReleases(t *testing.T) {
	a, store, _ := testAdapter(t)
	now := time.Unix(60, 0)
	a.Advance(now, APCEvent{Data: []byte("Ga=t,i=1,s=1,v=1,m=1;AAAA"), Final: true})
	a.activeFrames = termimage.HardChunksPerTransfer
	out := a.Advance(now, APCEvent{Data: []byte("Gm=0;"), Final: true})
	if out.Failure != ReplyLimit || store.Usage() != (termimage.Usage{}) {
		t.Fatalf("out=%#v usage=%#v", out, store.Usage())
	}
}

func TestAdapterOversizeDiscardsUntilAPCTerminator(t *testing.T) {
	a, store, _ := testAdapter(t)
	now := time.Now()
	out := a.Advance(now, APCEvent{Data: make([]byte, maxHeaderBytes+maxPayloadBytes+3)})
	if out.Failure != ReplyLimit {
		t.Fatalf("overflow=%#v", out)
	}
	out = a.Advance(now, APCEvent{Data: []byte("Ga=t,i=1,s=1,v=1;AAAA"), Final: true})
	if out != (Outcome{}) || store.Usage() != (termimage.Usage{}) {
		t.Fatalf("suffix reinterpreted out=%#v usage=%#v", out, store.Usage())
	}
	out = a.Advance(now, APCEvent{Data: []byte("Ga=t,i=1,s=1,v=1;AAAA"), Final: true})
	if out.Command == nil {
		t.Fatalf("next APC rejected=%#v", out)
	}
	out.Command.Transfer.Close()
}

func TestAdapterContinuationFailureUsesOriginalReplyPolicy(t *testing.T) {
	a, store, _ := testAdapter(t)
	now := time.Now()
	a.Advance(now, APCEvent{Data: []byte("Ga=T,q=2,i=1,p=1,s=1,v=1,m=1;AAAA"), Final: true})
	out := a.Advance(now, APCEvent{Data: []byte("Gm=1,a=t;AAAA"), Final: true})
	if out.Failure != ReplyInvalid || out.Reply.quiet != QuietAll || out.Reply.action != ActionTransmitAndPlace || len(out.Reply.Encode(out.Failure)) != 0 || store.Usage() != (termimage.Usage{}) {
		t.Fatalf("out=%#v usage=%#v", out, store.Usage())
	}
}
