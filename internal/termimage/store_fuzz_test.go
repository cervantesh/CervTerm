package termimage

import "testing"

func FuzzStoreLifecycle(f *testing.F) {
	f.Add([]byte{0, 1, 1, 2, 3, 4})
	f.Add([]byte{0, 2, 1, 255, 3, 0, 5})
	f.Fuzz(func(t *testing.T, operations []byte) {
		if len(operations) > 4096 {
			operations = operations[:4096]
		}
		process := NewProcessBudget()
		limits := Limits{EncodedBytes: 64 * 1024, DecodedBytes: 64 * 1024, Images: 8, Placements: 8}
		store := NewStore(process, limits)
		active := make(map[TransferID]*CandidateTransfer)
		for index, operation := range operations {
			id := TransferID(operation%4 + 1)
			switch operation % 6 {
			case 0:
				transfer, err := store.BeginTransfer(Header{Transfer: id, Image: ImageID(id)})
				if err == nil {
					active[id] = transfer
				}
			case 1:
				if transfer := active[id]; transfer != nil {
					size := int(operation%31 + 1)
					_ = transfer.Append(make([]byte, size))
				}
			case 2:
				if transfer := active[id]; transfer != nil {
					transfer.Close()
					delete(active, id)
				}
			case 3:
				store.Reset()
				clear(active)
			case 4:
				_, _ = store.Acquire(ResourceRef{Image: ImageID(id), Generation: ResourceGeneration(index + 1)})
			case 5:
				if transfer := active[id]; transfer != nil {
					_, _ = transfer.EncodedCopy()
				}
			}
			paneUsage := store.Usage()
			processUsage := process.Usage()
			if paneUsage != processUsage || paneUsage.EncodedBytes > limits.EncodedBytes || paneUsage.PendingTransfers > HardPendingTransfersPerPane {
				t.Fatalf("usage drift pane=%#v process=%#v", paneUsage, processUsage)
			}
		}
		store.Close()
		if process.Usage() != (Usage{}) || store.Usage() != (Usage{}) {
			t.Fatalf("close leaked pane=%#v process=%#v", store.Usage(), process.Usage())
		}
	})
}
