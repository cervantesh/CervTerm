//go:build windows

package fontglyph

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
)

func TestDirectWriteCreatesNonzeroCollectionFace(t *testing.T) {
	if !directWriteAvailable() {
		t.Skip("DirectWrite unavailable")
	}
	path := filepath.Join(t.TempDir(), "two-face.ttc")
	if err := os.WriteFile(path, makeTestTTC(t, gomono.TTF, gomono.TTF), 0o600); err != nil {
		t.Fatal(err)
	}
	factory, err := newDirectWriteFactory()
	if err != nil {
		t.Skipf("DirectWrite factory unavailable: %v", err)
	}
	defer factory.release()
	face, err := factory.createFontFaceFromPathIndex(path, 1)
	if err != nil {
		t.Fatalf("create collection face 1: %v", err)
	}
	face.release()
	if _, err := factory.createFontFaceFromPathIndex(path, 2); err == nil {
		t.Fatal("out-of-range collection face unexpectedly resolved; face index may not have reached DirectWrite")
	}
	if _, err := factory.createFontFaceFromPathIndex(path, -1); err == nil {
		t.Fatal("negative collection index accepted")
	}
}
