//go:build glfw && accessibilitymetrics

package glfwgl

import (
	"encoding/json"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type runtimeMetricsProbe struct {
	wakes atomic.Uint64
}

var runtimeMetricsProbes sync.Map

func startRuntimeMetricsProbe(app *App) {
	path := os.Getenv("CERVTERM_RUNTIME_METRICS_OUT")
	if app == nil || path == "" {
		return
	}
	delay := 3 * time.Second
	if value := os.Getenv("CERVTERM_RUNTIME_METRICS_DELAY"); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 && parsed <= 30*time.Second {
			delay = parsed
		}
	}
	probe := &runtimeMetricsProbe{}
	runtimeMetricsProbes.Store(app, probe)
	go func() {
		time.Sleep(delay)
		runtime.GC()
		snapshot := app.meter.Snapshot()
		payload := struct {
			DelayMS    int64  `json:"delay_ms"`
			Wakes      uint64 `json:"wakes"`
			Frames     uint64 `json:"frames"`
			HeapAlloc  uint64 `json:"heap_alloc"`
			Allocs     uint64 `json:"allocs"`
			TotalAlloc uint64 `json:"total_alloc"`
			NumGC      uint32 `json:"num_gc"`
		}{
			DelayMS: delay.Milliseconds(), Wakes: probe.wakes.Load(), Frames: snapshot.Frames,
			HeapAlloc: snapshot.HeapAlloc, Allocs: snapshot.Allocs, TotalAlloc: snapshot.TotalAlloc, NumGC: snapshot.NumGC,
		}
		if encoded, err := json.MarshalIndent(payload, "", "  "); err == nil {
			_ = os.WriteFile(path, append(encoded, '\n'), 0o600)
		}
		runtimeMetricsProbes.Delete(app)
	}()
}

func recordRuntimeMetricsWake(app *App) {
	if value, ok := runtimeMetricsProbes.Load(app); ok {
		value.(*runtimeMetricsProbe).wakes.Add(1)
	}
}
