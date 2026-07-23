//go:build glfw && windows

package glfwgl

import (
	"testing"
	"unsafe"

	"cervterm/internal/accessibility"
)

func accessibilityBenchmarkProvider(tb testing.TB) (*uiaRootProvider, *nativeUIAProvider, accessibility.NodeID) {
	tb.Helper()
	rootID := accessibility.NodeID{Kind: accessibility.NodeKindWindow, Projection: 1, Object: 1}
	paneID := accessibility.NodeID{Kind: accessibility.NodeKindPane, Projection: 1, Object: 2, Activation: 1}
	caret := 1
	bounds := make([]accessibility.Rect, len("benchmark"))
	for index := range bounds {
		bounds[index] = accessibility.Rect{X: float64(index * 8), Width: 8, Height: 16}
	}
	document, err := accessibility.NewDocument(accessibility.DocumentDraft{ProviderID: 7, Generation: 1, Focus: paneID, Nodes: []accessibility.NodeDraft{
		{ID: rootID, Role: accessibility.RoleWindow, Name: "CervTerm"},
		{ID: paneID, Parent: rootID, Role: accessibility.RoleTerminal, Name: "terminal", Rows: []accessibility.RowDraft{{Text: "benchmark", Bounds: bounds}}, Caret: &caret},
	}})
	if err != nil {
		tb.Fatal(err)
	}
	publication := &uiaPublication{}
	if err := publication.PublishScreen(document, 100, 200, accessibility.Rect{X: 100, Y: 200, Width: 800, Height: 600}); err != nil {
		tb.Fatal(err)
	}
	root, err := newDormantUIARootProvider(publication, &fakeUIANativeAPI{host: 88, hostHR: uiaSOK}, 55)
	if err != nil {
		tb.Fatal(err)
	}
	native, err := newNativeUIAProvider(root)
	if err != nil {
		root.Release()
		tb.Fatal(err)
	}
	tb.Cleanup(func() { root.Release() })
	tb.Cleanup(native.Close)
	return root, native, paneID
}

func BenchmarkAccessibilityUIAProviderHelperRead(b *testing.B) {
	root, _, paneID := accessibilityBenchmarkProvider(b)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if value, result := root.Property(paneID, uiaPropertyName); result != uiaSOK || value.String != "terminal" {
			b.Fatalf("property result=%#x value=%q", result, value.String)
		}
		if focus, result := root.Focus(); result != uiaSOK || focus != paneID {
			b.Fatalf("focus result=%#x value=%#v", result, focus)
		}
	}
}

func BenchmarkAccessibilityUIACallbackRead(b *testing.B) {
	_, native, paneID := accessibilityBenchmarkProvider(b)
	pane := native.object(paneID)
	if pane == nil || nativeUIARetain(pane) == 0 {
		b.Fatal("native pane interface is unavailable")
	}
	defer nativeUIARelease(pane.simple)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		var value nativeUIAVariant
		if result := nativeUIAGetPropertyValue(pane.simple, uintptr(uiaPropertyName), uintptr(unsafe.Pointer(&value))); result != uiaHRESULTResult(uiaSOK) || value.VT != uiaVTBSTR {
			b.Fatalf("property result=%#x variant=%#v", result, value)
		}
		uiaVariantClear.Call(uintptr(unsafe.Pointer(&value)))
	}
}

func TestAccessibilityUIAProviderReadAllocationCeiling(t *testing.T) {
	result := testing.Benchmark(BenchmarkAccessibilityUIACallbackRead)
	if result.AllocsPerOp() > 10 || result.AllocedBytesPerOp() > 1024 {
		t.Fatalf("UIA callback read=%d B/op %d allocs/op, ceilings=1024 B/op 10 allocs/op", result.AllocedBytesPerOp(), result.AllocsPerOp())
	}
}
