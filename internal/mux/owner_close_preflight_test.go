package mux

import (
	"errors"
	"reflect"
	"testing"

	"cervterm/internal/termimage"
)

func newImageOwnerForClosePreflightTest(t *testing.T) (*Owner, *pane) {
	t.Helper()
	limits := termimage.DefaultLimits()
	owner := NewOwner(&fakeFactory{}, Options{ImageLimits: &limits, KittyEnabled: true})
	if _, paneID, _, err := testWindowOwnerForOwner(owner).Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		closeOwnerForTest(owner)
		t.Fatal(err)
	} else if p, ok := owner.mux.sessions.lookup(paneID); !ok || p.imageStore == nil {
		closeOwnerForTest(owner)
		t.Fatal("image pane unavailable")
	} else {
		return owner, p
	}
	return nil, nil
}

func assertBusyClosePreservesMux(t *testing.T, owner *Owner, p *pane, invoke func() error) {
	t.Helper()
	prepared, err := p.terminal.PrepareCloseImageStore()
	if err != nil {
		t.Fatal(err)
	}
	before := captureL302MuxFingerprint(owner.mux)
	if err := invoke(); !errors.Is(err, termimage.ErrOwnerBusy) {
		_ = prepared.Abort()
		t.Fatalf("close rejection=%v want=%v", err, termimage.ErrOwnerBusy)
	}
	if after := captureL302MuxFingerprint(owner.mux); !reflect.DeepEqual(before, after) {
		_ = prepared.Abort()
		t.Fatalf("close rejection changed mux ownership: before=%#v after=%#v", before, after)
	}
	if owner.Closed() {
		_ = prepared.Abort()
		t.Fatal("rejected close published owner shutdown")
	}
	if err := prepared.Abort(); err != nil {
		t.Fatal(err)
	}
}

func TestMuxPaneTabWindowAndShutdownClosePreflightPreservesOwnership(t *testing.T) {
	t.Run("pane", func(t *testing.T) {
		owner, p := newImageOwnerForClosePreflightTest(t)
		defer closeOwnerForTest(owner)
		assertBusyClosePreservesMux(t, owner, p, func() error {
			_, err := testWindowOwnerForOwner(owner).ClosePane(p.id)
			return err
		})
		if _, err := testWindowOwnerForOwner(owner).ClosePane(p.id); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("tab", func(t *testing.T) {
		owner, p := newImageOwnerForClosePreflightTest(t)
		defer closeOwnerForTest(owner)
		tab, ok := owner.mux.TabForPane(p.id)
		if !ok {
			t.Fatal("pane tab unavailable")
		}
		assertBusyClosePreservesMux(t, owner, p, func() error {
			_, err := testWindowOwnerForOwner(owner).CloseTab(tab)
			return err
		})
		if _, err := testWindowOwnerForOwner(owner).CloseTab(tab); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("window", func(t *testing.T) {
		owner, p := newImageOwnerForClosePreflightTest(t)
		defer closeOwnerForTest(owner)
		window, ok := owner.mux.WindowForPane(p.id)
		if !ok {
			t.Fatal("pane window unavailable")
		}
		assertBusyClosePreservesMux(t, owner, p, func() error {
			_, _, err := owner.CloseWindow(window)
			return err
		})
		if _, _, err := owner.CloseWindow(window); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("shutdown", func(t *testing.T) {
		owner, p := newImageOwnerForClosePreflightTest(t)
		defer closeOwnerForTest(owner)
		assertBusyClosePreservesMux(t, owner, p, owner.Shutdown)
		if err := owner.Shutdown(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestMuxRollbackClosePreflightPreservesPublishedWindowAndRegistry(t *testing.T) {
	owner, _ := newImageOwnerForClosePreflightTest(t)
	defer closeOwnerForTest(owner)
	window, _, err := owner.CreateWindow(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}, "rollback")
	if err != nil {
		t.Fatal(err)
	}
	paneID := window.Tabs[0].Focused
	p, ok := owner.mux.sessions.lookup(paneID)
	if !ok {
		t.Fatal("rollback pane unavailable")
	}
	assertBusyClosePreservesMux(t, owner, p, func() error { return owner.RollbackWindow(window.ID) })
	if err := owner.RollbackWindow(window.ID); err != nil {
		t.Fatal(err)
	}
}

func TestMuxRestoreAbortClosePreflightPreservesCandidateAndRegistry(t *testing.T) {
	limits := termimage.DefaultLimits()
	owner := NewOwner(&restoreTestFactory{}, Options{ImageLimits: &limits, KittyEnabled: true})
	defer closeOwnerForTest(owner)
	candidate, err := owner.PrepareRestore(blueprintFromSnapshot(t, restoreSnapshot()), restoreGeometries())
	if err != nil {
		t.Fatal(err)
	}
	p := candidate.panes[0]
	assertBusyClosePreservesMux(t, owner, p, func() error { return owner.AbortRestore(candidate) })
	if candidate.aborted || candidate.committed || owner.mux.pending != candidate {
		t.Fatal("rejected restore abort changed candidate publication")
	}
	if err := owner.AbortRestore(candidate); err != nil {
		t.Fatal(err)
	}
}
