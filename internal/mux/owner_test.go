package mux

import (
	"errors"
	"reflect"
	"testing"
)

func TestOwnerCapabilityRejectsWrongCopyBusyClosedAndStale(t *testing.T) {
	owner := NewOwner(&fakeFactory{}, Options{})
	other := NewOwner(&fakeFactory{}, Options{})
	t.Cleanup(func() {
		_ = owner.mux.Shutdown()
		_ = other.mux.Shutdown()
	})

	if _, err := owner.enter(other.mux); !errors.Is(err, ErrWrongOwner) {
		t.Fatalf("wrong owner error=%v", err)
	}

	stamp, err := owner.enter(owner.mux)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.enter(owner.mux); !errors.Is(err, ErrOwnerBusy) {
		t.Fatalf("reentrant owner error=%v", err)
	}
	if err := stamp.valid(owner.mux); err != nil {
		t.Fatalf("active stamp error=%v", err)
	}
	owner.leave(stamp)

	stale := ownerStamp{state: owner.state, key: owner.key, generation: owner.generation}
	owner.publishClosed(nil)
	if _, err := owner.enter(owner.mux); !errors.Is(err, ErrOwnerClosed) {
		t.Fatalf("closed owner error=%v", err)
	}
	if err := stale.valid(owner.mux); !errors.Is(err, ErrStaleOwner) {
		t.Fatalf("stale stamp error=%v", err)
	}
}

func TestOwnerCapabilityMisusePreservesMuxState(t *testing.T) {
	owner := NewOwner(&fakeFactory{}, Options{})
	other := NewOwner(&fakeFactory{}, Options{})
	t.Cleanup(func() {
		_ = owner.mux.Shutdown()
		_ = other.mux.Shutdown()
	})
	before := owner.mux.Workspaces()
	if _, err := other.enter(owner.mux); !errors.Is(err, ErrWrongOwner) {
		t.Fatalf("wrong owner error=%v", err)
	}
	after := owner.mux.Workspaces()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("wrong-owner state changed: before=%#v after=%#v", before, after)
	}
}
