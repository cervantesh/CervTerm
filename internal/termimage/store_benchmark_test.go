package termimage

import "testing"

func BenchmarkProcessBudgetReserveRelease(b *testing.B) {
	process := NewProcessBudget()
	pane := paneBudget{limits: DefaultLimits()}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		lease, err := reserve(process, &pane, Usage{EncodedBytes: 1024, PendingTransfers: 1})
		if err != nil {
			b.Fatal(err)
		}
		lease.Close()
	}
}

func BenchmarkStoreBeginTransferCancel(b *testing.B) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		transfer, err := store.BeginTransfer(Header{Transfer: 1, Image: 1})
		if err != nil {
			b.Fatal(err)
		}
		transfer.Close()
	}
}

func BenchmarkStoreAcquireMiss(b *testing.B) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	ref := ResourceRef{Image: 1, Generation: 1}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, ok := store.Acquire(ref); ok {
			b.Fatal("unexpected hit")
		}
	}
}
