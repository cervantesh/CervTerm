package core

import (
	"errors"
	"reflect"
	"testing"

	"cervterm/internal/termimage"
)

type imageCloseFingerprint struct {
	store      *termimage.Store
	owner      *termimage.StoreOwner
	sidecars   *imageSidecars
	usage      termimage.Usage
	epoch      termimage.StoreEpoch
	refs       []termimage.ResourceRef
	generation uint64
}

func captureImageCloseFingerprint(terminal *Terminal) imageCloseFingerprint {
	result := imageCloseFingerprint{store: terminal.imageStore, owner: terminal.imageOwner, sidecars: terminal.imageSidecars}
	if terminal.imageStore != nil {
		result.usage = terminal.imageStore.Usage()
		result.epoch = terminal.imageStore.Epoch()
		result.refs = terminal.imageStore.ResourceRefs()
	}
	if terminal.imageSidecars != nil {
		result.generation = terminal.imageSidecars.generation
	}
	return result
}

func TestImageClosePreflightBlocksResetAndCloseWithoutStateChange(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 0, store)
	prepared, err := terminal.PrepareCloseImageStore()
	if err != nil {
		t.Fatal(err)
	}
	terminal.SetCursor(1, 2)
	terminal.SetTitle("preserved")
	before := captureImageCloseFingerprint(terminal)
	if err := terminal.ResetImages(); !errors.Is(err, termimage.ErrOwnerBusy) {
		t.Fatalf("reset during close preflight=%v", err)
	}
	if err := terminal.CloseImageStore(); !errors.Is(err, termimage.ErrOwnerBusy) {
		t.Fatalf("close during close preflight=%v", err)
	}
	terminal.Reset()
	if terminal.cursorRow != 1 || terminal.cursorCol != 2 || terminal.title != "preserved" {
		t.Fatal("rejected RIS reset changed terminal state")
	}
	if after := captureImageCloseFingerprint(terminal); !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected image lifecycle changed state: before=%#v after=%#v", before, after)
	}
	if err := prepared.Abort(); err != nil {
		t.Fatal(err)
	}
	if err := terminal.CloseImageStore(); err != nil {
		t.Fatal(err)
	}
}

func TestImageClosePreflightRejectsWrongOwnerWithoutStoreSidecarDivergence(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 0, store)
	correct := terminal.imageOwner
	otherStore := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	other := otherStore.ClaimOwner()
	if other == nil {
		t.Fatal("other owner unavailable")
	}
	terminal.imageOwner = other
	before := captureImageCloseFingerprint(terminal)
	if prepared, err := terminal.PrepareCloseImageStore(); prepared != nil || !errors.Is(err, termimage.ErrWrongOwner) {
		t.Fatalf("prepared=%#v wrong-owner error=%v", prepared, err)
	}
	if err := terminal.CloseImageStore(); !errors.Is(err, termimage.ErrWrongOwner) {
		t.Fatalf("wrong-owner close=%v", err)
	}
	if after := captureImageCloseFingerprint(terminal); !reflect.DeepEqual(before, after) {
		t.Fatalf("wrong-owner close changed state: before=%#v after=%#v", before, after)
	}
	terminal.imageOwner = correct
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	if err := terminal.CloseImageStore(); err != nil {
		t.Fatal(err)
	}
}
