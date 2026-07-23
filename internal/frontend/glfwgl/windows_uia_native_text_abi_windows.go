//go:build glfw && windows

package glfwgl

import (
	"sync"
	"sync/atomic"
	"syscall"

	"cervterm/internal/accessibility"
)

const (
	uiaTextPatternID   = 10014
	uiaTextPattern2ID  = 10024
	uiaVTUnknown       = 13
	maxNativeUIARanges = 512
)

var (
	uiaIIDTextProvider       = uiaGUID{Data1: 0x3589c92c, Data2: 0x63f3, Data3: 0x4367, Data4: [8]byte{0x99, 0xbb, 0xad, 0xa6, 0x53, 0xb7, 0x7c, 0xf2}}
	uiaIIDTextProvider2      = uiaGUID{Data1: 0x0dc5e6ed, Data2: 0x3e16, Data3: 0x4bf1, Data4: [8]byte{0x8f, 0x9a, 0xa9, 0x79, 0x87, 0x8b, 0xc1, 0x95}}
	uiaIIDTextRangeProvider  = uiaGUID{Data1: 0x5347ad7b, Data2: 0xc355, Data3: 0x46f8, Data4: [8]byte{0xaf, 0xf5, 0x90, 0x90, 0x33, 0x58, 0x2f, 0x63}}
	uiaIIDTextRangeProvider2 = uiaGUID{Data1: 0x9bbce42c, Data2: 0x1921, Data3: 0x4f18, Data4: [8]byte{0x89, 0xca, 0xdb, 0xa1, 0x91, 0x0a, 0x03, 0x86}}
)

type nativeUIATextProvider2VTable struct {
	QueryInterface         uintptr
	AddRef                 uintptr
	Release                uintptr
	GetSelection           uintptr
	GetVisibleRanges       uintptr
	RangeFromChild         uintptr
	RangeFromPoint         uintptr
	DocumentRange          uintptr
	SupportedTextSelection uintptr
	RangeFromAnnotation    uintptr
	GetCaretRange          uintptr
}

type nativeUIATextRangeProvider2VTable struct {
	QueryInterface        uintptr
	AddRef                uintptr
	Release               uintptr
	Clone                 uintptr
	Compare               uintptr
	CompareEndpoints      uintptr
	ExpandToEnclosingUnit uintptr
	FindAttribute         uintptr
	FindText              uintptr
	GetAttributeValue     uintptr
	GetBoundingRectangles uintptr
	GetEnclosingElement   uintptr
	GetText               uintptr
	Move                  uintptr
	MoveEndpointByUnit    uintptr
	MoveEndpointByRange   uintptr
	Select                uintptr
	AddToSelection        uintptr
	RemoveFromSelection   uintptr
	ScrollIntoView        uintptr
	GetChildren           uintptr
	ShowContextMenu       uintptr
}

type nativeUIATextRange struct {
	pointer uintptr
	owner   *nativeUIAObject
	mu      sync.Mutex
	value   accessibility.Range
	refs    atomic.Uint32
}

var (
	uiaTextProviderVTable nativeUIATextProvider2VTable
	uiaTextRangeVTable    nativeUIATextRangeProvider2VTable
	uiaTextVTableOnce     sync.Once
	nativeUIATextRanges   sync.Map
)

func ensureNativeUIATextVTables() {
	uiaTextVTableOnce.Do(func() {
		uiaTextProviderVTable = nativeUIATextProvider2VTable{
			QueryInterface: syscall.NewCallback(nativeUIAQueryInterface), AddRef: syscall.NewCallback(nativeUIAAddRef), Release: syscall.NewCallback(nativeUIARelease),
			GetSelection: syscall.NewCallback(nativeUIATextGetSelection), GetVisibleRanges: syscall.NewCallback(nativeUIATextGetVisibleRanges),
			RangeFromChild: syscall.NewCallback(nativeUIATextRangeFromChild), RangeFromPoint: nativeUIATextRangeFromPointCallback(),
			DocumentRange: syscall.NewCallback(nativeUIATextDocumentRange), SupportedTextSelection: syscall.NewCallback(nativeUIATextSupportedSelection),
			RangeFromAnnotation: syscall.NewCallback(nativeUIATextRangeFromAnnotation), GetCaretRange: syscall.NewCallback(nativeUIATextGetCaretRange),
		}
		uiaTextRangeVTable = nativeUIATextRangeProvider2VTable{
			QueryInterface: syscall.NewCallback(nativeUIATextRangeQueryInterface), AddRef: syscall.NewCallback(nativeUIATextRangeAddRef), Release: syscall.NewCallback(nativeUIATextRangeRelease),
			Clone: syscall.NewCallback(nativeUIATextRangeClone), Compare: syscall.NewCallback(nativeUIATextRangeCompare), CompareEndpoints: syscall.NewCallback(nativeUIATextRangeCompareEndpoints),
			ExpandToEnclosingUnit: syscall.NewCallback(nativeUIATextRangeExpand), FindAttribute: nativeUIATextFindAttributeCallback(), FindText: syscall.NewCallback(nativeUIATextRangeFindText),
			GetAttributeValue: syscall.NewCallback(nativeUIATextRangeGetAttribute), GetBoundingRectangles: syscall.NewCallback(nativeUIATextRangeGetBoundingRectangles),
			GetEnclosingElement: syscall.NewCallback(nativeUIATextRangeGetEnclosingElement), GetText: syscall.NewCallback(nativeUIATextRangeGetText),
			Move: syscall.NewCallback(nativeUIATextRangeMove), MoveEndpointByUnit: syscall.NewCallback(nativeUIATextRangeMoveEndpointByUnit), MoveEndpointByRange: syscall.NewCallback(nativeUIATextRangeMoveEndpointByRange),
			Select: syscall.NewCallback(nativeUIATextRangeUnsupported), AddToSelection: syscall.NewCallback(nativeUIATextRangeUnsupported), RemoveFromSelection: syscall.NewCallback(nativeUIATextRangeUnsupported),
			ScrollIntoView: syscall.NewCallback(nativeUIATextRangeScrollUnsupported), GetChildren: syscall.NewCallback(nativeUIATextRangeGetChildren), ShowContextMenu: syscall.NewCallback(nativeUIATextRangeUnsupported),
		}
	})
}
