//go:build windows

package fontglyph

import "testing"

func TestDirectWriteBridgeCreatesTextAnalyzer(t *testing.T) {
	if !directWriteAvailable() {
		t.Skip("DirectWrite is unavailable on this Windows host")
	}
	factory, err := newDirectWriteFactory()
	if err != nil {
		t.Fatalf("newDirectWriteFactory: %v", err)
	}
	defer factory.release()
	analyzer, err := factory.createTextAnalyzer()
	if err != nil {
		t.Fatalf("createTextAnalyzer: %v", err)
	}
	if !analyzer.hasGlyphShapingMethods() {
		t.Fatalf("TextAnalyzer missing expected glyph shaping method pointers")
	}
	analyzer.release()
	if !directWriteTextAnalyzerAvailable() {
		t.Fatalf("directWriteTextAnalyzerAvailable = false after successful creation")
	}
}
