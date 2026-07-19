package mux

import (
	"errors"
	"testing"

	"cervterm/internal/pty"
)

func registeredTestPane(t *testing.T, r *localSessionRegistry, id PaneID) (*pane, *fakeSession) {
	t.Helper()
	p := newPane(id, 80, 24, nil, nil)
	s := newFakeSession()
	p.session = s
	p.state = PaneStateRunning
	if err := r.reserve(id); err != nil {
		t.Fatal(err)
	}
	if err := r.register(p); err != nil {
		t.Fatal(err)
	}
	return p, s
}
func TestLocalSessionRegistryOwnsAttachDetachAndNeverReusesPane(t *testing.T) {
	r := newLocalSessionRegistry(&fakeFactory{}, 4, nil)
	p, _ := registeredTestPane(t, r, 1)
	if err := r.start(1); err != nil {
		t.Fatal(err)
	}
	if err := r.start(1); !errors.Is(err, ErrInvariant) {
		t.Fatalf("duplicate start=%v", err)
	}
	detached := r.detach(1)
	if !detached.owned || detached.pane != p {
		t.Fatalf("detach=%#v", detached)
	}
	if got := r.detach(1); got.owned {
		t.Fatalf("duplicate detach=%#v", got)
	}
	if err := r.reserve(1); !errors.Is(err, ErrInvariant) {
		t.Fatalf("reuse=%v", err)
	}
	if err := p.close(); err != nil {
		t.Fatal(err)
	}
	if err := r.shutdownRegistry(); err != nil {
		t.Fatal(err)
	}
}
func TestLocalSessionRegistryShutdownClosesEachOwnedSessionOnce(t *testing.T) {
	r := newLocalSessionRegistry(&fakeFactory{}, 4, nil)
	sessions := make([]*fakeSession, 2)
	for i := range sessions {
		_, sessions[i] = registeredTestPane(t, r, PaneID(i+1))
		if err := r.start(PaneID(i + 1)); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.shutdownRegistry(); err != nil {
		t.Fatal(err)
	}
	if err := r.shutdownRegistry(); err != nil {
		t.Fatal(err)
	}
	for i, s := range sessions {
		if got := s.closes(); got != 1 {
			t.Fatalf("session %d closes=%d", i, got)
		}
	}
	if r.count() != 0 {
		t.Fatalf("owned=%d", r.count())
	}
}
func TestLocalSessionRegistryRejectsOperationsAfterShutdown(t *testing.T) {
	r := newLocalSessionRegistry(&fakeFactory{}, 1, nil)
	if err := r.shutdownRegistry(); err != nil {
		t.Fatal(err)
	}
	if err := r.reserve(1); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("reserve=%v", err)
	}
	p := newPane(1, 80, 24, nil, nil)
	if err := r.register(p); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("register=%v", err)
	}
	if _, err := r.spawn(24, 80, pty.Options{}); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("spawn=%v", err)
	}
	if err := r.start(1); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("start=%v", err)
	}
}
func TestLocalSessionRegistryDetachUnknownDoesNotMarkClosed(t *testing.T) {
	r := newLocalSessionRegistry(&fakeFactory{}, 1, nil)
	if got := r.detach(99); got.owned || r.wasClosed(99) {
		t.Fatalf("detach=%#v closed=%v", got, r.wasClosed(99))
	}
	if err := r.shutdownRegistry(); err != nil {
		t.Fatal(err)
	}
}
