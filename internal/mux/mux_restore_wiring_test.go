package mux

import (
	"errors"
	"reflect"
	"testing"
)

type muxRestoreCoordinatorValue = restoreCoordinator[
	muxRestorePreparationOperationAdapter,
	muxRestorePublicationOperationAdapter,
]

func TestMuxRestoreCoordinatorEagerInitializationMatchesZeroValue(t *testing.T) {
	m := New(&fakeFactory{}, Options{})
	t.Cleanup(func() { _ = m.Shutdown() })
	if got, want := m.restoreCoordinator, (muxRestoreCoordinatorValue{}); !reflect.DeepEqual(got, want) {
		t.Fatalf("restore coordinator=%#v want eager zero-state value %#v", got, want)
	}
	if got, want := reflect.TypeOf(m.restoreCoordinator), reflect.TypeOf(muxRestoreCoordinatorValue{}); got != want || got.Size() != 0 || got.NumField() != 0 {
		t.Fatalf("restore coordinator type=%s size=%d fields=%d want=%s zero-state", got, got.Size(), got.NumField(), want)
	}
}

func TestMuxRestoreCoordinatorDetachedAliasesRemainDetached(t *testing.T) {
	m := newRestoreMux(&restoreTestFactory{})
	t.Cleanup(func() { _ = m.Shutdown() })
	candidate, err := m.PrepareRestore(blueprintFromSnapshot(t, restoreSnapshot()), restoreGeometries())
	if err != nil {
		t.Fatal(err)
	}

	ids, err := m.RestoreWindowIDs(candidate)
	if err != nil {
		t.Fatal(err)
	}
	ids[0] = 99
	again, err := m.RestoreWindowIDs(candidate)
	if err != nil || !reflect.DeepEqual(again, []WindowID{2, 3}) {
		t.Fatalf("restore window aliases=%v err=%v", again, err)
	}
	if _, err = m.CommitRestore(candidate); err != nil {
		t.Fatal(err)
	}

	snapshot, err := m.FreshSessionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Workspaces[1].Windows[0].Title = "mutated"
	snapshot.Workspaces[1].Windows[0].Tabs[0].Root.First.Launch.Args[0] = "mutated"
	againSnapshot, err := m.FreshSessionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	firstWindow := againSnapshot.Workspaces[1].Windows[0]
	if firstWindow.Title != "first" || !reflect.DeepEqual(firstWindow.Tabs[0].Root.First.Launch.Args, []string{"--a"}) {
		t.Fatalf("fresh snapshot alias leaked title=%q args=%v", firstWindow.Title, firstWindow.Tabs[0].Root.First.Launch.Args)
	}
}

func TestMuxRestoreCoordinatorIndependentCallsRetainCandidateOwnership(t *testing.T) {
	left := newRestoreMux(&restoreTestFactory{})
	right := newRestoreMux(&restoreTestFactory{})
	t.Cleanup(func() { _ = left.Shutdown() })
	t.Cleanup(func() { _ = right.Shutdown() })
	blueprint := blueprintFromSnapshot(t, restoreSnapshot())
	leftCandidate, err := left.PrepareRestore(blueprint, restoreGeometries())
	if err != nil {
		t.Fatal(err)
	}
	rightCandidate, err := right.PrepareRestore(blueprint, restoreGeometries())
	if err != nil {
		t.Fatal(err)
	}
	if leftCandidate.owner != left || rightCandidate.owner != right || left.pending != leftCandidate || right.pending != rightCandidate {
		t.Fatalf("initial ownership left=%p/%p right=%p/%p", leftCandidate.owner, left.pending, rightCandidate.owner, right.pending)
	}

	if _, err = left.RestoreWindowIDs(rightCandidate); !errors.Is(err, ErrWrongOwner) {
		t.Fatalf("cross-owner window IDs err=%v", err)
	}
	if _, err = left.CommitRestore(rightCandidate); !errors.Is(err, ErrWrongOwner) {
		t.Fatalf("cross-owner commit err=%v", err)
	}
	if err = left.AbortRestore(rightCandidate); !errors.Is(err, ErrWrongOwner) {
		t.Fatalf("cross-owner abort err=%v", err)
	}
	if left.pending != leftCandidate || right.pending != rightCandidate || leftCandidate.owner != left || rightCandidate.owner != right || leftCandidate.aborted || leftCandidate.committed || rightCandidate.aborted || rightCandidate.committed {
		t.Fatalf("independent calls changed ownership left=%#v right=%#v", leftCandidate, rightCandidate)
	}
	if leftIDs, leftErr := left.RestoreWindowIDs(leftCandidate); leftErr != nil || !reflect.DeepEqual(leftIDs, []WindowID{2, 3}) {
		t.Fatalf("left IDs=%v err=%v", leftIDs, leftErr)
	}
	if rightIDs, rightErr := right.RestoreWindowIDs(rightCandidate); rightErr != nil || !reflect.DeepEqual(rightIDs, []WindowID{2, 3}) {
		t.Fatalf("right IDs=%v err=%v", rightIDs, rightErr)
	}
	if err = left.AbortRestore(leftCandidate); err != nil {
		t.Fatal(err)
	}
	if _, err = right.CommitRestore(rightCandidate); err != nil {
		t.Fatal(err)
	}
	if !leftCandidate.aborted || leftCandidate.committed || rightCandidate.aborted || !rightCandidate.committed || left.pending != nil || right.pending != nil {
		t.Fatalf("final independent ownership left=%#v right=%#v", leftCandidate, rightCandidate)
	}
}

var (
	muxRestoreWiringSnapshot  FreshSessionSnapshot
	muxRestoreWiringCandidate *RestoreCandidate
	muxRestoreWiringIDs       []WindowID
	muxRestoreWiringEvents    []Event
	muxRestoreWiringError     error
)

func TestMuxRestoreCoordinatorWiringPreservesAllocationParity(t *testing.T) {
	direct := newRestoreMux(restoreBenchmarkFactory{})
	wired := newRestoreMux(restoreBenchmarkFactory{})
	t.Cleanup(func() { _ = direct.Shutdown() })
	t.Cleanup(func() { _ = wired.Shutdown() })
	blueprint := blueprintFromSnapshot(t, restoreSnapshot())
	geometries := restoreGeometries()
	directCandidate, err := direct.PrepareRestore(blueprint, geometries)
	if err != nil {
		t.Fatal(err)
	}
	wiredCandidate, err := wired.PrepareRestore(blueprint, geometries)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		direct func()
		wired  func()
	}{
		{name: "fresh-invalid", direct: func() {
			muxRestoreWiringSnapshot, muxRestoreWiringError = (muxRestorePreparationOperationAdapter{mux: direct}).freshSessionSnapshot()
		}, wired: func() {
			muxRestoreWiringSnapshot, muxRestoreWiringError = wired.FreshSessionSnapshot()
		}},
		{name: "prepare-pending", direct: func() {
			muxRestoreWiringCandidate, muxRestoreWiringError = (muxRestorePreparationOperationAdapter{mux: direct, blueprint: blueprint, geometries: geometries}).prepareRestore()
		}, wired: func() {
			muxRestoreWiringCandidate, muxRestoreWiringError = wired.PrepareRestore(blueprint, geometries)
		}},
		{name: "window-ids", direct: func() {
			muxRestoreWiringIDs, muxRestoreWiringError = (muxRestorePublicationOperationAdapter{mux: direct}).restoreWindowIDs(directCandidate)
		}, wired: func() {
			muxRestoreWiringIDs, muxRestoreWiringError = wired.RestoreWindowIDs(wiredCandidate)
		}},
		{name: "commit-invalid", direct: func() {
			muxRestoreWiringEvents, muxRestoreWiringError = (muxRestorePublicationOperationAdapter{mux: direct}).commitRestore(nil)
		}, wired: func() {
			muxRestoreWiringEvents, muxRestoreWiringError = wired.CommitRestore(nil)
		}},
		{name: "abort-invalid", direct: func() {
			muxRestoreWiringError = (muxRestorePublicationOperationAdapter{mux: direct}).abortRestore(nil)
		}, wired: func() {
			muxRestoreWiringError = wired.AbortRestore(nil)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.direct()
			test.wired()
			directAllocs := testing.AllocsPerRun(1000, test.direct)
			wiredAllocs := testing.AllocsPerRun(1000, test.wired)
			if wiredAllocs > directAllocs {
				t.Fatalf("%s coordinator allocations=%v direct adapter=%v delta=%v", test.name, wiredAllocs, directAllocs, wiredAllocs-directAllocs)
			}
		})
	}
	if direct.pending != directCandidate || wired.pending != wiredCandidate || directCandidate.owner != direct || wiredCandidate.owner != wired {
		t.Fatal("allocation probes changed candidate ownership")
	}
}

func BenchmarkMuxRestoreCoordinatorAttribution(b *testing.B) {
	direct := newRestoreMux(restoreBenchmarkFactory{})
	wired := newRestoreMux(restoreBenchmarkFactory{})
	b.Cleanup(func() { _ = direct.Shutdown() })
	b.Cleanup(func() { _ = wired.Shutdown() })
	blueprint := blueprintFromSnapshot(b, restoreSnapshot())
	geometries := restoreGeometries()
	directCandidate, err := direct.PrepareRestore(blueprint, geometries)
	if err != nil {
		b.Fatal(err)
	}
	wiredCandidate, err := wired.PrepareRestore(blueprint, geometries)
	if err != nil {
		b.Fatal(err)
	}
	benchmarks := []struct {
		name string
		run  func()
	}{
		{name: "fresh-invalid/direct-adapter", run: func() {
			muxRestoreWiringSnapshot, muxRestoreWiringError = (muxRestorePreparationOperationAdapter{mux: direct}).freshSessionSnapshot()
		}},
		{name: "fresh-invalid/coordinator", run: func() {
			muxRestoreWiringSnapshot, muxRestoreWiringError = wired.FreshSessionSnapshot()
		}},
		{name: "invalid-window-ids/direct-adapter", run: func() {
			muxRestoreWiringIDs, muxRestoreWiringError = (muxRestorePublicationOperationAdapter{mux: direct}).restoreWindowIDs(nil)
		}},
		{name: "invalid-window-ids/coordinator", run: func() {
			muxRestoreWiringIDs, muxRestoreWiringError = wired.RestoreWindowIDs(nil)
		}},
		{name: "pending-prepare/direct-adapter", run: func() {
			muxRestoreWiringCandidate, muxRestoreWiringError = (muxRestorePreparationOperationAdapter{mux: direct, blueprint: blueprint, geometries: geometries}).prepareRestore()
		}},
		{name: "pending-prepare/coordinator", run: func() {
			muxRestoreWiringCandidate, muxRestoreWiringError = wired.PrepareRestore(blueprint, geometries)
		}},
		{name: "window-ids/direct-adapter", run: func() {
			muxRestoreWiringIDs, muxRestoreWiringError = (muxRestorePublicationOperationAdapter{mux: direct}).restoreWindowIDs(directCandidate)
		}},
		{name: "window-ids/coordinator", run: func() {
			muxRestoreWiringIDs, muxRestoreWiringError = wired.RestoreWindowIDs(wiredCandidate)
		}},
		{name: "commit-invalid/direct-adapter", run: func() {
			muxRestoreWiringEvents, muxRestoreWiringError = (muxRestorePublicationOperationAdapter{mux: direct}).commitRestore(nil)
		}},
		{name: "commit-invalid/coordinator", run: func() {
			muxRestoreWiringEvents, muxRestoreWiringError = wired.CommitRestore(nil)
		}},
		{name: "abort-invalid/direct-adapter", run: func() {
			muxRestoreWiringError = (muxRestorePublicationOperationAdapter{mux: direct}).abortRestore(nil)
		}},
		{name: "abort-invalid/coordinator", run: func() {
			muxRestoreWiringError = wired.AbortRestore(nil)
		}},
	}
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			benchmark.run()
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				benchmark.run()
			}
		})
	}
}
