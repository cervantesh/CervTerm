//go:build glfw && windows

package glfwgl

import (
	"sync"
	"sync/atomic"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"cervterm/internal/accessibility"
)

const (
	uiaVTEmpty = 0
	uiaVTI4    = 3
	uiaVTR8    = 5
	uiaVTBSTR  = 8
	uiaVTBool  = 11
	uiaVTArray = 0x2000
)

const maxNativeUIAObjects = 512

type nativeUIAVariant struct {
	VT        uint16
	Reserved1 uint16
	Reserved2 uint16
	Reserved3 uint16
	Value     uintptr
	Extra     uintptr
}

type nativeUIARect struct{ Left, Top, Width, Height float64 }

type nativeUIASimpleVTable struct {
	QueryInterface         uintptr
	AddRef                 uintptr
	Release                uintptr
	ProviderOptions        uintptr
	GetPatternProvider     uintptr
	GetPropertyValue       uintptr
	HostRawElementProvider uintptr
}

type nativeUIAFragmentVTable struct {
	QueryInterface        uintptr
	AddRef                uintptr
	Release               uintptr
	Navigate              uintptr
	GetRuntimeID          uintptr
	BoundingRectangle     uintptr
	EmbeddedFragmentRoots uintptr
	SetFocus              uintptr
	FragmentRoot          uintptr
}

type nativeUIAFragmentRootVTable struct {
	QueryInterface           uintptr
	AddRef                   uintptr
	Release                  uintptr
	ElementProviderFromPoint uintptr
	GetFocus                 uintptr
}

type nativeUIAInterface struct {
	VTable uintptr
}

type nativeUIAObject struct {
	fragment     uintptr
	simple       uintptr
	fragmentRoot uintptr
	text         uintptr
	textCapable  bool
	provider     *nativeUIAProvider
	node         accessibility.NodeID
	refs         atomic.Uint32
}

type nativeUIAProvider struct {
	root         *uiaRootProvider
	mu           sync.Mutex
	objects      map[accessibility.NodeID]*nativeUIAObject
	rootObject   *nativeUIAObject
	pointer      uintptr
	closeOnce    sync.Once
	finalizeOnce sync.Once
	textRanges   atomic.Int32
}

var (
	uiaSimpleVTable           nativeUIASimpleVTable
	uiaFragmentVTable         nativeUIAFragmentVTable
	uiaFragmentRootVTable     nativeUIAFragmentRootVTable
	uiaVTableOnce             sync.Once
	nativeUIAInterfaces       sync.Map
	uiaAutomationCore         = syscall.NewLazyDLL("UIAutomationCore.dll")
	uiaHostProviderProc       = uiaAutomationCore.NewProc("UiaHostProviderFromHwnd")
	uiaReturnProviderProc     = uiaAutomationCore.NewProc("UiaReturnRawElementProvider")
	uiaDisconnectProviderProc = uiaAutomationCore.NewProc("UiaDisconnectProvider")
	uiaReservedNotSupported   = uiaAutomationCore.NewProc("UiaGetReservedNotSupportedValue")
	uiaKernel32               = syscall.NewLazyDLL("kernel32.dll")
	uiaGlobalAlloc            = uiaKernel32.NewProc("GlobalAlloc")
	uiaGlobalFree             = uiaKernel32.NewProc("GlobalFree")
	uiaOleAut                 = syscall.NewLazyDLL("oleaut32.dll")
	uiaSysAllocStringLen      = uiaOleAut.NewProc("SysAllocStringLen")
	uiaSysStringLen           = uiaOleAut.NewProc("SysStringLen")
	uiaSysFreeString          = uiaOleAut.NewProc("SysFreeString")
	uiaVariantClear           = uiaOleAut.NewProc("VariantClear")
	uiaSafeArrayCreateVector  = uiaOleAut.NewProc("SafeArrayCreateVector")
	uiaSafeArrayAccessData    = uiaOleAut.NewProc("SafeArrayAccessData")
	uiaSafeArrayUnaccessData  = uiaOleAut.NewProc("SafeArrayUnaccessData")
	uiaSafeArrayDestroy       = uiaOleAut.NewProc("SafeArrayDestroy")
)

func ensureNativeUIAVTables() {
	uiaVTableOnce.Do(func() {
		uiaSimpleVTable = nativeUIASimpleVTable{
			QueryInterface: syscall.NewCallback(nativeUIAQueryInterface), AddRef: syscall.NewCallback(nativeUIAAddRef), Release: syscall.NewCallback(nativeUIARelease),
			ProviderOptions: syscall.NewCallback(nativeUIAProviderOptions), GetPatternProvider: syscall.NewCallback(nativeUIAGetPatternProvider),
			GetPropertyValue: syscall.NewCallback(nativeUIAGetPropertyValue), HostRawElementProvider: syscall.NewCallback(nativeUIAHostProvider),
		}
		uiaFragmentVTable = nativeUIAFragmentVTable{
			QueryInterface: syscall.NewCallback(nativeUIAQueryInterface), AddRef: syscall.NewCallback(nativeUIAAddRef), Release: syscall.NewCallback(nativeUIARelease),
			Navigate: syscall.NewCallback(nativeUIANavigate), GetRuntimeID: syscall.NewCallback(nativeUIAGetRuntimeID), BoundingRectangle: syscall.NewCallback(nativeUIABoundingRectangle),
			EmbeddedFragmentRoots: syscall.NewCallback(nativeUIAEmbeddedRoots), SetFocus: syscall.NewCallback(nativeUIASetFocus), FragmentRoot: syscall.NewCallback(nativeUIAFragmentRoot),
		}
		uiaFragmentRootVTable = nativeUIAFragmentRootVTable{
			QueryInterface: syscall.NewCallback(nativeUIAQueryInterface), AddRef: syscall.NewCallback(nativeUIAAddRef), Release: syscall.NewCallback(nativeUIARelease),
			ElementProviderFromPoint: nativeUIAElementFromPointCallback(), GetFocus: syscall.NewCallback(nativeUIAGetFocus),
		}
	})
}

func newNativeUIAProvider(root *uiaRootProvider) (*nativeUIAProvider, error) {
	ensureNativeUIAVTables()
	ensureNativeUIATextVTables()
	if root == nil || !root.available() || uiaFragmentRootVTable.ElementProviderFromPoint == 0 {
		return nil, errUIAProviderInvalid
	}
	frame, ok := root.publication.SnapshotFrame()
	if !ok || !frame.screenSpace {
		return nil, errUIAProviderInvalid
	}
	if !frame.root.Valid() {
		return nil, errUIAProviderInvalid
	}
	provider := &nativeUIAProvider{root: root, objects: make(map[accessibility.NodeID]*nativeUIAObject)}
	provider.rootObject = provider.objectLocked(frame.root)
	if provider.rootObject == nil {
		provider.finalize()
		return nil, errUIAProviderInvalid
	}
	provider.rootObject.fragmentRoot = allocateNativeUIAInterface(uintptr(unsafe.Pointer(&uiaFragmentRootVTable)))
	if provider.rootObject.fragmentRoot == 0 {
		provider.finalize()
		return nil, errUIAProviderInvalid
	}
	nativeUIAInterfaces.Store(provider.rootObject.fragmentRoot, provider.rootObject)
	pointer := provider.rootObject.simple
	if !root.nativePointer.CompareAndSwap(0, pointer) {
		provider.finalize()
		return nil, errUIAProviderDuplicate
	}
	provider.pointer = pointer
	root.finalizeMu.Lock()
	root.finalizeNative = provider.finalize
	root.finalizeMu.Unlock()
	root.AddRef()
	return provider, nil
}

//go:nocheckptr
func nativeUIAQueryInterface(this, iidPointer, outputPointer uintptr) uintptr {
	if outputPointer == 0 || iidPointer == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(outputPointer)) = 0
	object := nativeUIAObjectFromThis(this)
	if object == nil || object.provider == nil || object.provider.root.refs.Load() == 0 {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	iid := *(*uiaGUID)(unsafe.Pointer(iidPointer))
	var result uintptr
	switch iid {
	case uiaIIDUnknown, uiaIIDRawElementProviderFragment:
		result = object.fragment
	case uiaIIDRawElementProviderSimple:
		result = object.simple
	case uiaIIDRawElementProviderFragmentRoot:
		if object != object.provider.rootObject {
			return uiaHRESULTResult(uiaENoInterface)
		}
		result = object.fragmentRoot
	case uiaIIDTextProvider, uiaIIDTextProvider2:
		result = object.provider.textInterface(object)
		if result == 0 {
			return uiaHRESULTResult(uiaENoInterface)
		}
	default:
		return uiaHRESULTResult(uiaENoInterface)
	}
	if nativeUIARetain(object) == 0 {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	*(*uintptr)(unsafe.Pointer(outputPointer)) = result
	return uiaHRESULTResult(uiaSOK)
}

func nativeUIAAddRef(this uintptr) uintptr {
	object := nativeUIAObjectFromThis(this)
	if object == nil || object.provider == nil {
		return 0
	}
	return uintptr(nativeUIARetain(object))
}
func nativeUIARelease(this uintptr) uintptr {
	object := nativeUIAObjectFromThis(this)
	if object == nil || object.provider == nil {
		return 0
	}
	return uintptr(nativeUIAReleaseObject(object))
}

//go:nocheckptr
func nativeUIAProviderOptions(this, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	object := nativeUIAAvailableObject(this)
	if object == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	value, hr := object.provider.root.ProviderOptions()
	*(*int32)(unsafe.Pointer(output)) = value
	return uiaHRESULTResult(hr)
}

//go:nocheckptr
func nativeUIAGetPatternProvider(this, pattern uintptr, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	object := nativeUIAAvailableObject(this)
	if object == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	if pattern != uiaTextPatternID && pattern != uiaTextPattern2ID {
		return uiaHRESULTResult(uiaSOK)
	}
	text := object.provider.textInterface(object)
	if text == 0 || nativeUIARetain(object) == 0 {
		return uiaHRESULTResult(uiaSOK)
	}
	*(*uintptr)(unsafe.Pointer(output)) = text
	return uiaHRESULTResult(uiaSOK)
}

//go:nocheckptr
func nativeUIAGetPropertyValue(this, property, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	variant := (*nativeUIAVariant)(unsafe.Pointer(output))
	*variant = nativeUIAVariant{}
	object := nativeUIAAvailableObject(this)
	if object == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	value, hr := object.provider.root.Property(object.node, uiaPropertyID(int32(property)))
	if hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	switch value.Kind {
	case uiaVariantInt:
		variant.VT, variant.Value = uiaVTI4, uintptr(uint32(value.Int))
	case uiaVariantBool:
		variant.VT = uiaVTBool
		if value.Bool {
			variant.Value = ^uintptr(0)
		}
	case uiaVariantString:
		units := utf16.Encode([]rune(value.String))
		var source uintptr
		if len(units) != 0 {
			source = uintptr(unsafe.Pointer(&units[0]))
		}
		pointer, _, _ := uiaSysAllocStringLen.Call(source, uintptr(len(units)))
		if pointer == 0 {
			return uiaHRESULTResult(uiaEOutOfMemory)
		}
		variant.VT, variant.Value = uiaVTBSTR, pointer
	case uiaVariantRect:
		array, arrayHR := nativeUIARectSafeArray(value.Rect)
		if arrayHR != uiaSOK {
			return uiaHRESULTResult(arrayHR)
		}
		variant.VT, variant.Value = uiaVTArray|uiaVTR8, array
	default:
		variant.VT = uiaVTEmpty
	}
	return uiaHRESULTResult(uiaSOK)
}

//go:nocheckptr
func nativeUIAHostProvider(this, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	object := nativeUIAAvailableObject(this)
	if object == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	if object != object.provider.rootObject {
		return uiaHRESULTResult(uiaSOK)
	}
	value, hr := object.provider.root.HostProvider()
	*(*uintptr)(unsafe.Pointer(output)) = value
	return uiaHRESULTResult(hr)
}

//go:nocheckptr
func nativeUIANavigate(this, direction, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	object := nativeUIAObjectFromThis(this)
	if object == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	id, hr := object.provider.root.Navigate(object.node, uiaNavigateDirection(direction))
	if hr != uiaSOK || !id.Valid() {
		return uiaHRESULTResult(hr)
	}
	target := object.provider.object(id)
	if target == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	if nativeUIARetain(target) == 0 {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	*(*uintptr)(unsafe.Pointer(output)) = target.fragment
	return uiaHRESULTResult(uiaSOK)
}

//go:nocheckptr
func nativeUIAGetRuntimeID(this, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	object := nativeUIAAvailableObject(this)
	if object == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	if object == object.provider.rootObject {
		return uiaHRESULTResult(uiaSOK)
	}
	id := object.node
	values := [8]int32{3, int32(id.Projection), int32(id.Projection >> 32), int32(id.Kind), int32(id.Object), int32(id.Object >> 32), int32(id.Activation), int32(id.Activation >> 32)}
	array, _, _ := uiaSafeArrayCreateVector.Call(uiaVTI4, 0, uintptr(len(values)))
	if array == 0 {
		return uiaHRESULTResult(uiaEOutOfMemory)
	}
	var data uintptr
	hr, _, _ := uiaSafeArrayAccessData.Call(array, uintptr(unsafe.Pointer(&data)))
	if uiaHRESULT(int32(hr)) != uiaSOK || data == 0 {
		uiaSafeArrayDestroy.Call(array)
		if uiaHRESULT(int32(hr)) != uiaSOK {
			return uintptr(uint32(hr))
		}
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	copy(unsafe.Slice((*int32)(unsafe.Pointer(data)), len(values)), values[:])
	unaccess, _, _ := uiaSafeArrayUnaccessData.Call(array)
	if uiaHRESULT(int32(unaccess)) != uiaSOK {
		uiaSafeArrayDestroy.Call(array)
		return uintptr(uint32(unaccess))
	}
	*(*uintptr)(unsafe.Pointer(output)) = array
	return uiaHRESULTResult(uiaSOK)
}

//go:nocheckptr
func nativeUIABoundingRectangle(this, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	object := nativeUIAObjectFromThis(this)
	if object == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	value, hr := object.provider.root.Property(object.node, uiaPropertyBoundingRectangle)
	if hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	*(*nativeUIARect)(unsafe.Pointer(output)) = nativeUIARect{Left: value.Rect.X, Top: value.Rect.Y, Width: value.Rect.Width, Height: value.Rect.Height}
	return uiaHRESULTResult(uiaSOK)
}

func uiaHRESULTResult(value uiaHRESULT) uintptr { return uintptr(uint32(int32(value))) }

type windowsUIANativeAPI struct{}

func (windowsUIANativeAPI) HostProviderFromHWND(hwnd uintptr) (uintptr, uiaHRESULT) {
	var provider uintptr
	result, _, _ := uiaHostProviderProc.Call(hwnd, uintptr(unsafe.Pointer(&provider)))
	return provider, uiaHRESULT(int32(result))
}
func (windowsUIANativeAPI) ReturnRawElementProvider(hwnd, wParam, lParam, provider uintptr) uintptr {
	result, _, _ := uiaReturnProviderProc.Call(hwnd, wParam, lParam, provider)
	return result
}
func (windowsUIANativeAPI) DisconnectProvider(provider uintptr) uiaHRESULT {
	result, _, _ := uiaDisconnectProviderProc.Call(provider)
	return uiaHRESULT(int32(result))
}
