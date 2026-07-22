package kitty

import (
	"testing"
	"time"

	"cervterm/internal/termimage"
)

func FuzzKittyAdapter(f *testing.F) {
	for _, seed := range [][]byte{[]byte("Ga=t,i=1,s=1,v=1;AAAA"), []byte("Ga=t,i=1,s=1,v=1,m=1;AAAA"), []byte("Ga=d,d=A"), []byte("Gm=1;AAAA"), nil} {
		f.Add(seed, uint8(0))
	}
	f.Fuzz(func(t *testing.T, data []byte, flags uint8) {
		process := termimage.NewProcessBudget()
		store := termimage.NewStore(process, termimage.DefaultLimits())
		adapter := NewAdapter(store)
		now := time.Now()
		consume := func(out Outcome) {
			if out.Command != nil && out.Command.Transfer != nil {
				out.Command.Transfer.Close()
			}
			if reply := out.Reply.Encode(out.Failure); uint64(len(reply)) > termimage.HardReplyBytes {
				t.Fatal("reply bound")
			}
		}
		mid := len(data) / 2
		first := adapter.Advance(now, APCEvent{Data: data[:mid]})
		second := adapter.Advance(now.Add(time.Millisecond), APCEvent{Data: data[mid:], Final: true, Cancelled: flags&2 != 0, Overflow: flags&4 != 0})
		if first.Failure != ReplyNone && (second.Failure != ReplyNone || second.Command != nil) {
			t.Fatal("multiple outcomes for one rejected APC")
		}
		consume(first)
		consume(second)
		if flags&8 != 0 {
			consume(adapter.Advance(now.Add(2*time.Millisecond), APCEvent{Data: []byte("Gm=0;"), Final: true}))
		}
		consume(adapter.Expire(now.Add(2 * termimage.HardTransferLifetime)))
		consume(adapter.Advance(now, APCEvent{Cancelled: true}))
		adapter.Close()
		store.Close()
		if process.Usage() != (termimage.Usage{}) || store.Usage() != (termimage.Usage{}) {
			t.Fatalf("leak process=%#v pane=%#v", process.Usage(), store.Usage())
		}
	})
}
