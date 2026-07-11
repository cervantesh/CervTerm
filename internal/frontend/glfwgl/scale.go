package glfwgl

func effectiveDPI(scaleX, scaleY float32) float64 {
	scale := max(scaleX, scaleY)
	return min(384, max(96, 96*float64(scale)))
}
