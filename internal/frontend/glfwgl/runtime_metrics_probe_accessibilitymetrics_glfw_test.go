//go:build glfw && accessibilitymetrics

package glfwgl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeMetricsProbeWritesBoundedSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")
	t.Setenv("CERVTERM_RUNTIME_METRICS_OUT", path)
	t.Setenv("CERVTERM_RUNTIME_METRICS_DELAY", "1ms")
	app := &App{}
	app.meter.AddFrame()
	startRuntimeMetricsProbe(app)
	recordRuntimeMetricsWake(app)
	recordRuntimeMetricsWake(app)
	deadline := time.Now().Add(time.Second)
	var content []byte
	var err error
	for time.Now().Before(deadline) {
		content, err = os.ReadFile(path)
		if err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Wakes  uint64 `json:"wakes"`
		Frames uint64 `json:"frames"`
	}
	if err := json.Unmarshal(content, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Wakes != 2 || snapshot.Frames != 1 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
}
