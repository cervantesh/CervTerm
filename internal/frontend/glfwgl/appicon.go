//go:build glfw

package glfwgl

import (
	"bytes"
	"embed"
	"image"
	_ "image/png"
	"log"
)

//go:embed icons/icon_256.png icons/icon_64.png icons/icon_32.png
var iconFS embed.FS

// windowIcons decodes the embedded application icons, largest first, for
// glfw.Window.SetIcon. GLFW picks the size closest to what the OS requests.
func windowIcons() []image.Image {
	names := []string{"icons/icon_256.png", "icons/icon_64.png", "icons/icon_32.png"}
	icons := make([]image.Image, 0, len(names))
	for _, name := range names {
		data, err := iconFS.ReadFile(name)
		if err != nil {
			log.Printf("window icon %s unavailable: %v", name, err)
			continue
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			log.Printf("window icon %s decode failed: %v", name, err)
			continue
		}
		icons = append(icons, img)
	}
	return icons
}
