//go:build windows

package fontglyph

import "testing"

func TestFallbackFacesKeepSourcePathForDirectWrite(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	for _, face := range backend.faces[1:] {
		if face.sourcePath != "" {
			return
		}
	}
	t.Skip("no fallback system font with source path loaded on this host")
}

func TestDirectWriteBridgeCreatesFontFaceFromFallbackPath(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	var path string
	for _, face := range backend.faces[1:] {
		if face.sourcePath != "" {
			path = face.sourcePath
			break
		}
	}
	if path == "" {
		t.Skip("no fallback system font with source path loaded on this host")
	}
	factory, err := newDirectWriteFactory()
	if err != nil {
		t.Fatalf("newDirectWriteFactory: %v", err)
	}
	defer factory.release()
	fontFace, err := factory.createFontFaceFromPath(path)
	if err != nil {
		t.Fatalf("createFontFaceFromPath(%s): %v", path, err)
	}
	fontFace.release()
}
