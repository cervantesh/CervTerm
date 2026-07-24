//go:build glfw

package glfwgl

import (
	"testing"
	"time"

	"cervterm/internal/config"
)

func TestPresentationInterval(t *testing.T) {
	for _, test := range []struct {
		fps  int
		want time.Duration
	}{{0, 0}, {-1, 0}, {1, time.Second}, {40, 25 * time.Millisecond}, {1000, time.Millisecond}} {
		if got := presentationInterval(test.fps); got != test.want {
			t.Fatalf("presentationInterval(%d) = %v, want %v", test.fps, got, test.want)
		}
	}
}

func TestPresentationGatePreservesDeadlineDeterministically(t *testing.T) {
	t0 := time.Unix(100, 0)
	var gate presentationGate
	if !gate.ready(t0, 60) {
		t.Fatal("first presentation must be ready")
	}
	gate.record(t0)
	interval := presentationInterval(60)
	if gate.ready(t0.Add(interval-time.Nanosecond), 60) {
		t.Fatal("presentation became ready before its deadline")
	}
	if got := gate.wait(t0.Add(time.Millisecond), 60); got != interval-time.Millisecond {
		t.Fatalf("wait = %v, want %v", got, interval-time.Millisecond)
	}
	if !gate.ready(t0.Add(interval), 60) {
		t.Fatal("presentation must be ready exactly at its deadline")
	}
	if !gate.ready(t0.Add(time.Millisecond), 0) || gate.wait(t0.Add(time.Millisecond), 0) != 0 {
		t.Fatal("zero must disable the explicit cap")
	}
}

func TestPresentationGateUsesLiveRateAgainstLastPresentation(t *testing.T) {
	t0 := time.Unix(200, 0)
	gate := presentationGate{last: t0}
	if got := gate.wait(t0.Add(10*time.Millisecond), 20); got != 40*time.Millisecond {
		t.Fatalf("20 fps wait = %v, want 40ms", got)
	}
	if got := gate.wait(t0.Add(10*time.Millisecond), 100); got != 0 {
		t.Fatalf("100 fps live update wait = %v, want ready", got)
	}
}

func TestPresentationGateRejectedAdmissionRetainsRedrawDemand(t *testing.T) {
	t0 := time.Unix(300, 0)
	cfg := config.Defaults()
	cfg.Render.MaxFPS = 60
	app := &App{cfg: cfg, needsRedraw: true, presentation: presentationGate{last: t0}}

	if app.shouldRedraw(t0.Add(time.Millisecond)) {
		t.Fatal("on-demand frame was admitted before the presentation deadline")
	}
	if !app.needsRedraw || !app.redrawWanted(t0.Add(time.Millisecond)) {
		t.Fatal("rejected frame discarded redraw demand")
	}
	if app.presentation.last != t0 {
		t.Fatalf("rejected frame changed presentation accounting: %v", app.presentation.last)
	}
}

func TestPresentationGateAdmissionHasNoAllocations(t *testing.T) {
	t0 := time.Unix(400, 0)
	gate := presentationGate{last: t0}
	now := t0.Add(time.Millisecond)
	if allocs := testing.AllocsPerRun(1000, func() {
		_ = gate.ready(now, 60)
		_ = gate.wait(now, 60)
	}); allocs != 0 {
		t.Fatalf("presentation admission allocations = %v, want 0", allocs)
	}
}

func BenchmarkPresentationGateRejectedAdmission(b *testing.B) {
	t0 := time.Unix(500, 0)
	gate := presentationGate{last: t0}
	now := t0.Add(time.Millisecond)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if gate.ready(now, 60) || gate.wait(now, 60) <= 0 {
			b.Fatal("rejected admission became ready")
		}
	}
}
