//go:build glfw

package glfwgl

import "testing"

func TestOpenTypeFaceMetrics(t *testing.T) {
	face, metrics, err := newOpenTypeFace(fontSpec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("newOpenTypeFace failed: %v", err)
	}
	defer face.Close()

	if metrics.Ascent <= 0 || metrics.Descent <= 0 || metrics.Height <= 0 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
}
