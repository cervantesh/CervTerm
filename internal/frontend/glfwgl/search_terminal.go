//go:build glfw

package glfwgl

// searchTerminal is the narrow pane-aware port used by searchController.
type searchTerminal interface {
	SearchUpward(query string, hasPrev bool, prevRow int) (row, col int, ok bool)
}
