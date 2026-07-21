package termimage

import "testing"

func TestPreparedResetIsOldOrNewAndFinalizesAllOwnedState(t *testing.T) {
	process := NewProcessBudget()
	store := NewStore(process, DefaultLimits())
	owner := store.ClaimOwner()
	candidate, _ := store.NewDecodedCandidate(1, 1, 1)
	prepared, ref, err := owner.PrepareCandidate(candidate)
	if err != nil {
		t.Fatal(err)
	}
	owner.PublishPrepared(prepared)
	prepared.Finalize()
	transfer, err := store.BeginTransfer(Header{Transfer: 1, Image: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err = transfer.Append([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	placement, err := store.ReservePlacements(1)
	if err != nil {
		t.Fatal(err)
	}
	loose, err := store.NewDecodedCandidate(3, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	oldEpoch := store.Epoch()
	reset, err := owner.PrepareReset()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginTransfer(Header{Transfer: 2, Image: 4}); err == nil {
		t.Fatal("transfer entered prepared reset")
	}
	if _, err := store.NewDecodedCandidate(4, 1, 1); err == nil {
		t.Fatal("candidate entered prepared reset")
	}
	if _, err := store.ReservePlacements(1); err == nil {
		t.Fatal("placement entered prepared reset")
	}
	if _, ok := store.Acquire(ref); !ok || store.Epoch() != oldEpoch {
		t.Fatal("prepared reset became visible early")
	}
	reset.Abort()
	if _, ok := store.Acquire(ref); !ok || store.Usage().Placements != 1 {
		t.Fatal("aborted reset changed state")
	}
	if !loose.ValidFor(store) {
		t.Fatal("aborted reset consumed loose candidate")
	}
	reset, err = owner.PrepareReset()
	if err != nil {
		t.Fatal(err)
	}
	owner.PublishPrepared(reset)
	if _, ok := store.Acquire(ref); ok || store.Epoch() == oldEpoch {
		t.Fatal("reset publication incomplete")
	}
	if store.Usage() == (Usage{}) {
		t.Fatal("ownership released before finalize")
	}
	reset.Finalize()
	if loose.ValidFor(store) || loose.RGBA() != nil {
		t.Fatal("published reset retained loose candidate")
	}
	placement.Close()
	if store.Usage() != (Usage{}) || process.Usage() != (Usage{}) {
		t.Fatalf("reset leaked pane=%#v process=%#v", store.Usage(), process.Usage())
	}
}
