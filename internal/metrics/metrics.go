package metrics

import (
	"runtime"
	"sync/atomic"
	"time"
)

type Snapshot struct {
	At          time.Time
	Frames      uint64
	Bytes       uint64
	Allocs      uint64
	TotalAlloc  uint64
	HeapAlloc   uint64
	NumGC       uint32
	PauseTotal  time.Duration
	LastGCPause time.Duration
}

type Meter struct {
	frames atomic.Uint64
	bytes  atomic.Uint64
}

func (m *Meter) AddFrame() { m.frames.Add(1) }
func (m *Meter) AddBytes(n int) {
	if n > 0 {
		m.bytes.Add(uint64(n))
	}
}

func (m *Meter) Snapshot() Snapshot {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	var lastPause time.Duration
	if ms.NumGC > 0 {
		lastPause = time.Duration(ms.PauseNs[(ms.NumGC+255)%256])
	}
	return Snapshot{
		At:          time.Now(),
		Frames:      m.frames.Load(),
		Bytes:       m.bytes.Load(),
		Allocs:      ms.Mallocs,
		TotalAlloc:  ms.TotalAlloc,
		HeapAlloc:   ms.HeapAlloc,
		NumGC:       ms.NumGC,
		PauseTotal:  time.Duration(ms.PauseTotalNs),
		LastGCPause: lastPause,
	}
}
