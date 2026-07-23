//go:build glfw && !accessibilitymetrics

package glfwgl

func startRuntimeMetricsProbe(*App) {}

func recordRuntimeMetricsWake(*App) {}
