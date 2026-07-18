//go:build glfw

package glfwgl

// paneNeedsFlatBackground preserves the composed surface unless there is no
// layered background or pane-local OSC 11 explicitly overrides it.
func paneNeedsFlatBackground(layerCount int, oscBackgroundSet bool) bool {
	return layerCount == 0 || oscBackgroundSet
}
