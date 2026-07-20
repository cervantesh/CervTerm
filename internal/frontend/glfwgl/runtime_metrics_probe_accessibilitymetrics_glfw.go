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
	delay := runtimeMetricsDuration("CERVTERM_RUNTIME_METRICS_DELAY", 3*time.Second)
	warmup := runtimeMetricsDuration("CERVTERM_RUNTIME_METRICS_WARMUP", time.Second)
	startPath := os.Getenv("CERVTERM_RUNTIME_METRICS_START")
	probe := &runtimeMetricsProbe{}
	runtimeMetricsProbes.Store(app, probe)
	go func() {
		if !awaitRuntimeMetricsStart(startPath, warmup) {
			runtimeMetricsProbes.Delete(app)
			return
		}
		runtime.GC()
		before := app.meter.Snapshot()
		beforeWakes := probe.wakes.Load()
		time.Sleep(delay)
		runtime.GC()
		after := app.meter.Snapshot()
		payload := struct {
			WarmupMS   int64  `json:"warmup_ms"`
			DelayMS    int64  `json:"delay_ms"`
			Wakes      uint64 `json:"wakes"`
			Frames     uint64 `json:"frames"`
			HeapAlloc  uint64 `json:"heap_alloc"`
			Allocs     uint64 `json:"allocs"`
			TotalAlloc uint64 `json:"total_alloc"`
			NumGC      uint32 `json:"num_gc"`
		}{
			WarmupMS: warmup.Milliseconds(), DelayMS: delay.Milliseconds(), Wakes: probe.wakes.Load() - beforeWakes, Frames: after.Frames - before.Frames,
			HeapAlloc: after.HeapAlloc, Allocs: after.Allocs - before.Allocs, TotalAlloc: after.TotalAlloc - before.TotalAlloc, NumGC: after.NumGC - before.NumGC,
		}
		if encoded, err := json.MarshalIndent(payload, "", "  "); err == nil {
			_ = os.WriteFile(path, append(encoded, '\n'), 0o600)
		}
		runtimeMetricsProbes.Delete(app)
	}()
}

func awaitRuntimeMetricsStart(path string, warmup time.Duration) bool {
	if path == "" {
		time.Sleep(warmup)
		return true
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func runtimeMetricsDuration(name string, fallback time.Duration) time.Duration {
	if parsed, err := time.ParseDuration(os.Getenv(name)); err == nil && parsed > 0 && parsed <= 30*time.Second {
		return parsed
	}
	return fallback
}

func recordRuntimeMetricsWake(app *App) {
	if value, ok := runtimeMetricsProbes.Load(app); ok {
		value.(*runtimeMetricsProbe).wakes.Add(1)
	}
}
