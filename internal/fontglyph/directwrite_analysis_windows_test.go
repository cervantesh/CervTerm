//go:build windows

package fontglyph

import (
	"testing"
	"unicode/utf16"
)

func TestDirectWriteAnalyzeScriptCallback(t *testing.T) {
	factory, err := newDirectWriteFactory()
	if err != nil {
		t.Fatalf("newDirectWriteFactory: %v", err)
	}
	defer factory.release()
	analyzer, err := factory.createTextAnalyzer()
	if err != nil {
		t.Fatalf("createTextAnalyzer: %v", err)
	}
	defer analyzer.release()
	script, ok, err := analyzer.analyzeScript(utf16.Encode([]rune("ABC")))
	if err != nil {
		t.Fatalf("analyzeScript: %v", err)
	}
	if !ok {
		t.Fatalf("expected script analysis result")
	}
	if script.Script != 49 || script.Shapes != 0 {
		t.Fatalf("unexpected script analysis: %#v; want script=49 shapes=0 for Latin ABC", script)
	}
}
