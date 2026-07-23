package termimage

import (
	"sync"
	"testing"
)

func TestReservationExactReleaseAndDoubleClose(t *testing.T) {
	process := NewProcessBudget()
	pane := paneBudget{limits: DefaultLimits()}
	want := Usage{EncodedBytes: 7, DecodedBytes: 11, Images: 1, Placements: 2, PendingTransfers: 1}
	lease, err := reserve(process, &pane, want)
	if err != nil {
		t.Fatal(err)
	}
	if got := process.Usage(); got != want {
		t.Fatalf("process usage = %#v", got)
	}
	if got := pane.usage(); got != want {
		t.Fatalf("pane usage = %#v", got)
	}
	lease.Close()
	lease.Close()
	if got := process.Usage(); got != (Usage{}) || pane.usage() != (Usage{}) {
		t.Fatalf("usage after close process=%#v pane=%#v", got, pane.usage())
	}
}

func TestCompositeReservationRollsBackEarlierCounters(t *testing.T) {
	process := NewProcessBudget()
	pane := paneBudget{limits: Limits{EncodedBytes: 8, DecodedBytes: 8, Images: 1, Placements: 1}}
	blocker, err := reserve(process, &pane, Usage{Placements: 1})
	if err != nil {
		t.Fatal(err)
	}
	beforeProcess, beforePane := process.Usage(), pane.usage()
	if _, err := reserve(process, &pane, Usage{EncodedBytes: 8, DecodedBytes: 8, Images: 1, Placements: 1}); err == nil {
		t.Fatal("composite reservation unexpectedly succeeded")
	}
	if process.Usage() != beforeProcess || pane.usage() != beforePane {
		t.Fatalf("failed reservation changed counters process=%#v pane=%#v", process.Usage(), pane.usage())
	}
	blocker.Close()
}

func TestProcessLimitAcrossPanes(t *testing.T) {
	process := NewProcessBudget()
	limits := DefaultLimits()
	panes := make([]paneBudget, 5)
	var leases []*reservation
	for i := range panes {
		panes[i].limits = limits
		count := uint64(256)
		if i == 4 {
			count = 1
		}
		lease, err := reserve(process, &panes[i], Usage{Images: count})
		if i < 4 && err != nil {
			t.Fatalf("pane %d: %v", i, err)
		}
		if i == 4 {
			if err == nil {
				t.Fatal("process image cap exceeded")
			}
			break
		}
		leases = append(leases, lease)
	}
	for _, lease := range leases {
		lease.Close()
	}
	if process.Usage() != (Usage{}) {
		t.Fatalf("process usage leaked: %#v", process.Usage())
	}
}

func TestPlacementCapsExactAcrossPanes(t *testing.T) {
	process := NewProcessBudget()
	panes := make([]paneBudget, 5)
	var leases []*reservation
	for i := range panes {
		panes[i].limits = DefaultLimits()
		amount := HardPlacementsPerPane
		if i == 4 {
			amount = 1
		}
		lease, err := reserve(process, &panes[i], Usage{Placements: amount})
		if i < 4 {
			if err != nil {
				t.Fatalf("pane %d exact placement cap: %v", i, err)
			}
			leases = append(leases, lease)
			continue
		}
		if err == nil {
			t.Fatal("process placement cap exceeded")
		}
	}
	if process.Usage().Placements != HardPlacementsProcess {
		t.Fatalf("process placements = %d", process.Usage().Placements)
	}
	for _, lease := range leases {
		lease.Close()
	}
	if process.Usage() != (Usage{}) {
		t.Fatal("placement reservations leaked")
	}
}

func TestReservationConcurrentClose(t *testing.T) {
	process := NewProcessBudget()
	pane := paneBudget{limits: DefaultLimits()}
	lease, err := reserve(process, &pane, Usage{DecodedBytes: 1024, Images: 1})
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for i := 0; i < 32; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			lease.Close()
		}()
	}
	group.Wait()
	if process.Usage() != (Usage{}) || pane.usage() != (Usage{}) {
		t.Fatalf("concurrent close leaked process=%#v pane=%#v", process.Usage(), pane.usage())
	}
}

func TestReservationHighContentionStaysBounded(t *testing.T) {
	process := NewProcessBudget()
	pane := paneBudget{limits: DefaultLimits()}
	leases := make(chan *reservation, HardImagesPerPane)
	var group sync.WaitGroup
	for i := 0; i < int(HardImagesPerPane*2); i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if lease, err := reserve(process, &pane, Usage{Images: 1}); err == nil {
				leases <- lease
			}
		}()
	}
	group.Wait()
	close(leases)
	if got := pane.usage().Images; got > HardImagesPerPane || process.Usage().Images != got {
		t.Fatalf("bounded images pane=%d process=%d", got, process.Usage().Images)
	}
	for lease := range leases {
		lease.Close()
	}
	if process.Usage() != (Usage{}) || pane.usage() != (Usage{}) {
		t.Fatal("contention reservations leaked")
	}
}

func TestReserveCounterCannotWrap(t *testing.T) {
	process := NewProcessBudget()
	pane := paneBudget{limits: DefaultLimits()}
	if _, err := reserve(process, &pane, Usage{EncodedBytes: ^uint64(0)}); err == nil {
		t.Fatal("overflow-shaped reservation accepted")
	}
	if process.Usage() != (Usage{}) || pane.usage() != (Usage{}) {
		t.Fatal("overflow rejection changed counters")
	}
}
