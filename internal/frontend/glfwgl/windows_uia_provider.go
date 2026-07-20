//go:build glfw

package glfwgl

import (
	"errors"
	"math"
	"sync"
	"sync/atomic"

	"cervterm/internal/accessibility"
)

const (
	wmGetObject       = 0x003d
	uiaRootObjectID   = -25
	maxUIAProviders   = 64
	uiaProviderServer = 1
)

type uiaHRESULT int32

const (
	uiaSOK                  uiaHRESULT = 0
	uiaSFalse               uiaHRESULT = 1
	uiaENoInterface         uiaHRESULT = -2147467262
	uiaEPointer             uiaHRESULT = -2147467261
	uiaEInvalidArg          uiaHRESULT = -2147024809
	uiaEOutOfMemory         uiaHRESULT = -2147024882
	uiaEElementNotAvailable uiaHRESULT = -2147220991
	uiaENotSupported        uiaHRESULT = -2147220992
)

var (
	errUIAProviderInvalid   = errors.New("UIA provider is invalid")
	errUIAProviderLimit     = errors.New("UIA provider dispatcher limit reached")
	errUIAProviderDuplicate = errors.New("UIA provider is already registered")
	errUIAProviderMissing   = errors.New("UIA provider registration is stale")
	errUIAPublicationStale  = errors.New("UIA publication generation is stale")
	errUIAPublicationClosed = errors.New("UIA publication is disconnected")
)

type uiaGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	uiaIIDUnknown                        = uiaGUID{Data4: [8]byte{0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	uiaIIDRawElementProviderSimple       = uiaGUID{Data1: 0xd6dd68d1, Data2: 0x86fd, Data3: 0x4332, Data4: [8]byte{0x86, 0x66, 0x9a, 0xbe, 0xde, 0xa2, 0xd2, 0x4c}}
	uiaIIDRawElementProviderFragment     = uiaGUID{Data1: 0xf7063da8, Data2: 0x8359, Data3: 0x439c, Data4: [8]byte{0x92, 0x97, 0xbb, 0xc5, 0x29, 0x9a, 0x7d, 0x87}}
	uiaIIDRawElementProviderFragmentRoot = uiaGUID{Data1: 0x620ce2a5, Data2: 0xab8f, Data3: 0x40a9, Data4: [8]byte{0x86, 0xcb, 0xde, 0x3c, 0x75, 0x59, 0x9b, 0x58}}
)

type uiaInterface uint8

const (
	uiaInterfaceNone uiaInterface = iota
	uiaInterfaceUnknown
	uiaInterfaceSimple
	uiaInterfaceFragment
	uiaInterfaceFragmentRoot
)

type uiaNavigateDirection uint8

const (
	uiaNavigateParent uiaNavigateDirection = iota
	uiaNavigateNextSibling
	uiaNavigatePreviousSibling
	uiaNavigateFirstChild
	uiaNavigateLastChild
)

type uiaPropertyID int32

const (
	uiaPropertyBoundingRectangle   uiaPropertyID = 30001
	uiaPropertyControlType         uiaPropertyID = 30003
	uiaPropertyName                uiaPropertyID = 30005
	uiaPropertyHasKeyboardFocus    uiaPropertyID = 30008
	uiaPropertyIsKeyboardFocusable uiaPropertyID = 30009
	uiaPropertyIsEnabled           uiaPropertyID = 30010
	uiaPropertyNativeWindow        uiaPropertyID = 30020
)

type uiaVariantKind uint8

const (
	uiaVariantEmpty uiaVariantKind = iota
	uiaVariantInt
	uiaVariantBool
	uiaVariantString
	uiaVariantRect
)

type uiaVariant struct {
	Kind   uiaVariantKind
	Int    int64
	Bool   bool
	String string
	Rect   accessibility.Rect
}

type uiaPublishedDocument struct {
	document     accessibility.Document
	root         accessibility.NodeID
	screenX      float64
	screenY      float64
	windowBounds accessibility.Rect
	screenSpace  bool
}

type uiaPublication struct {
	mu      sync.Mutex
	current atomic.Pointer[uiaPublishedDocument]
	closed  bool
}

func (publication *uiaPublication) Publish(document accessibility.Document) error {
	return publication.publish(document, 0, 0, accessibility.Rect{}, false)
}

func (publication *uiaPublication) PublishScreen(document accessibility.Document, screenX, screenY float64, windowBounds accessibility.Rect) error {
	if !windowBounds.Valid() {
		return errUIAProviderInvalid
	}
	return publication.publish(document, screenX, screenY, windowBounds, true)
}

func (publication *uiaPublication) publish(document accessibility.Document, screenX, screenY float64, windowBounds accessibility.Rect, screenSpace bool) error {
	if publication == nil || document.ProviderID() == 0 || document.Generation() == 0 {
		return errUIAProviderInvalid
	}
	nodes := document.Nodes()
	if len(nodes) == 0 || nodes[0].ID.Kind != accessibility.NodeKindWindow {
		return errUIAProviderInvalid
	}
	publication.mu.Lock()
	defer publication.mu.Unlock()
	if publication.closed {
		return errUIAPublicationClosed
	}
	if current := publication.current.Load(); current != nil {
		if current.document.ProviderID() != document.ProviderID() || current.root != nodes[0].ID || document.Generation() <= current.document.Generation() {
			return errUIAPublicationStale
		}
	}
	publication.current.Store(&uiaPublishedDocument{document: document, root: nodes[0].ID, screenX: screenX, screenY: screenY, windowBounds: windowBounds, screenSpace: screenSpace})
	return nil
}

func (publication *uiaPublication) Snapshot() (accessibility.Document, bool) {
	current, ok := publication.SnapshotFrame()
	if !ok {
		return accessibility.Document{}, false
	}
	return current.document, true
}

func (publication *uiaPublication) SnapshotFrame() (*uiaPublishedDocument, bool) {
	if publication == nil {
		return nil, false
	}
	current := publication.current.Load()
	return current, current != nil
}

func (publication *uiaPublication) Disconnect() {
	if publication == nil {
		return
	}
	publication.mu.Lock()
	publication.closed = true
	publication.current.Store(nil)
	publication.mu.Unlock()
}

type uiaNativeProviderAPI interface {
	HostProviderFromHWND(hwnd uintptr) (uintptr, uiaHRESULT)
	ReturnRawElementProvider(hwnd, wParam, lParam, provider uintptr) uintptr
	DisconnectProvider(provider uintptr) uiaHRESULT
}

type uiaRootProvider struct {
	publication    *uiaPublication
	api            uiaNativeProviderAPI
	hwnd           uintptr
	token          atomic.Uintptr
	nativePointer  atomic.Uintptr
	refs           atomic.Uint32
	disconnected   atomic.Bool
	finalizeMu     sync.Mutex
	finalizeNative func()
}

func newDormantUIARootProvider(publication *uiaPublication, api uiaNativeProviderAPI, hwnd uintptr) (*uiaRootProvider, error) {
	if publication == nil || api == nil || hwnd == 0 {
		return nil, errUIAProviderInvalid
	}
	provider := &uiaRootProvider{publication: publication, api: api, hwnd: hwnd}
	provider.refs.Store(1)
	return provider, nil
}

func (provider *uiaRootProvider) QueryInterface(iid uiaGUID) (uiaInterface, uiaHRESULT) {
	if provider == nil || provider.refs.Load() == 0 {
		return uiaInterfaceNone, uiaEElementNotAvailable
	}
	var result uiaInterface
	switch iid {
	case uiaIIDUnknown:
		result = uiaInterfaceUnknown
	case uiaIIDRawElementProviderSimple:
		result = uiaInterfaceSimple
	case uiaIIDRawElementProviderFragment:
		result = uiaInterfaceFragment
	case uiaIIDRawElementProviderFragmentRoot:
		result = uiaInterfaceFragmentRoot
	default:
		return uiaInterfaceNone, uiaENoInterface
	}
	provider.AddRef()
	return result, uiaSOK
}

func (provider *uiaRootProvider) AddRef() uint32 {
	if provider == nil {
		return 0
	}
	for {
		current := provider.refs.Load()
		if current == math.MaxUint32 || current == 0 {
			return current
		}
		if provider.refs.CompareAndSwap(current, current+1) {
			return current + 1
		}
	}
}

func (provider *uiaRootProvider) Release() uint32 {
	if provider == nil {
		return 0
	}
	for {
		current := provider.refs.Load()
		if current == 0 {
			return 0
		}
		if current == math.MaxUint32 {
			return current
		}
		if provider.refs.CompareAndSwap(current, current-1) {
			if current == 1 {
				provider.disconnected.Store(true)
				provider.finalizeMu.Lock()
				finalize := provider.finalizeNative
				provider.finalizeNative = nil
				provider.finalizeMu.Unlock()
				if finalize != nil {
					finalize()
				}
			}
			return current - 1
		}
	}
}

func (provider *uiaRootProvider) Disconnect() {
	if provider == nil {
		return
	}
	if !provider.disconnected.CompareAndSwap(false, true) {
		return
	}
	raw := provider.nativePointer.Swap(0)
	provider.publication.Disconnect()
	if raw != 0 {
		provider.api.DisconnectProvider(raw)
	}
}

func (provider *uiaRootProvider) ProviderOptions() (int32, uiaHRESULT) {
	if !provider.available() {
		return 0, uiaEElementNotAvailable
	}
	return uiaProviderServer, uiaSOK
}

func (provider *uiaRootProvider) HostProvider() (uintptr, uiaHRESULT) {
	if !provider.available() {
		return 0, uiaEElementNotAvailable
	}
	return provider.api.HostProviderFromHWND(provider.hwnd)
}

func (provider *uiaRootProvider) Property(node accessibility.NodeID, property uiaPropertyID) (uiaVariant, uiaHRESULT) {
	frame, snapshot, ok := provider.frameNode(node)
	if !ok {
		return uiaVariant{}, uiaEElementNotAvailable
	}
	switch property {
	case uiaPropertyName:
		return uiaVariant{Kind: uiaVariantString, String: snapshot.Name}, uiaSOK
	case uiaPropertyControlType:
		return uiaVariant{Kind: uiaVariantInt, Int: int64(uiaControlType(snapshot.Role))}, uiaSOK
	case uiaPropertyHasKeyboardFocus:
		return uiaVariant{Kind: uiaVariantBool, Bool: frame.document.Focus() == node}, uiaSOK
	case uiaPropertyIsKeyboardFocusable:
		return uiaVariant{Kind: uiaVariantBool, Bool: frame.document.Focus() == node}, uiaSOK
	case uiaPropertyIsEnabled:
		return uiaVariant{Kind: uiaVariantBool, Bool: true}, uiaSOK
	case uiaPropertyNativeWindow:
		if snapshot.ID != frame.root {
			return uiaVariant{Kind: uiaVariantEmpty}, uiaSOK
		}
		return uiaVariant{Kind: uiaVariantInt, Int: int64(provider.hwnd)}, uiaSOK
	case uiaPropertyBoundingRectangle:
		return uiaVariant{Kind: uiaVariantRect, Rect: uiaFrameNodeBounds(frame, snapshot.ID)}, uiaSOK
	default:
		return uiaVariant{Kind: uiaVariantEmpty}, uiaSOK
	}
}

func (provider *uiaRootProvider) Navigate(node accessibility.NodeID, direction uiaNavigateDirection) (accessibility.NodeID, uiaHRESULT) {
	frame, _, ok := provider.frameNode(node)
	document := accessibility.Document{}
	if frame != nil {
		document = frame.document
	}
	if !ok {
		return accessibility.NodeID{}, uiaEElementNotAvailable
	}
	nodes := document.Nodes()
	index := -1
	for i := range nodes {
		if nodes[i].ID == node {
			index = i
			break
		}
	}
	if index < 0 {
		return accessibility.NodeID{}, uiaEElementNotAvailable
	}
	switch direction {
	case uiaNavigateParent:
		return nodes[index].Parent, uiaSOK
	case uiaNavigateFirstChild, uiaNavigateLastChild:
		candidate := accessibility.NodeID{}
		for _, child := range nodes {
			if child.Parent == node {
				candidate = child.ID
				if direction == uiaNavigateFirstChild {
					break
				}
			}
		}
		return candidate, uiaSOK
	case uiaNavigateNextSibling:
		for i := index + 1; i < len(nodes); i++ {
			if nodes[i].Parent == nodes[index].Parent {
				return nodes[i].ID, uiaSOK
			}
		}
	case uiaNavigatePreviousSibling:
		for i := index - 1; i >= 0; i-- {
			if nodes[i].Parent == nodes[index].Parent {
				return nodes[i].ID, uiaSOK
			}
		}
	default:
		return accessibility.NodeID{}, uiaEInvalidArg
	}
	return accessibility.NodeID{}, uiaSOK
}

func (provider *uiaRootProvider) Focus() (accessibility.NodeID, uiaHRESULT) {
	if !provider.available() {
		return accessibility.NodeID{}, uiaEElementNotAvailable
	}
	frame, ok := provider.publication.SnapshotFrame()
	if !ok {
		return accessibility.NodeID{}, uiaEElementNotAvailable
	}
	return frame.document.Focus(), uiaSOK
}

func (provider *uiaRootProvider) available() bool {
	if provider == nil || provider.disconnected.Load() || provider.refs.Load() == 0 {
		return false
	}
	_, ok := provider.publication.Snapshot()
	return ok
}

func (provider *uiaRootProvider) frameNode(id accessibility.NodeID) (*uiaPublishedDocument, accessibility.NodeSnapshot, bool) {
	if !provider.available() {
		return nil, accessibility.NodeSnapshot{}, false
	}
	frame, ok := provider.publication.SnapshotFrame()
	if !ok {
		return nil, accessibility.NodeSnapshot{}, false
	}
	node, ok := frame.document.Node(id)
	return frame, node, ok
}

func uiaControlType(role accessibility.Role) int32 {
	switch role {
	case accessibility.RoleWindow:
		return 50032
	case accessibility.RoleTab:
		return 50018
	case accessibility.RoleDialog:
		return 50033
	case accessibility.RoleTextField, accessibility.RoleTerminal:
		return 50004
	case accessibility.RoleList:
		return 50008
	case accessibility.RoleListItem:
		return 50007
	case accessibility.RoleStatus:
		return 50017
	default:
		return 50025
	}
}

func uiaFrameNodeBounds(frame *uiaPublishedDocument, id accessibility.NodeID) accessibility.Rect {
	if frame == nil || !frame.screenSpace {
		return accessibility.Rect{}
	}
	if id == frame.root {
		return frame.windowBounds
	}
	nodes := frame.document.Nodes()
	byID := make(map[accessibility.NodeID]accessibility.NodeSnapshot, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	var collect func(accessibility.NodeID) (accessibility.Rect, bool)
	collect = func(current accessibility.NodeID) (accessibility.Rect, bool) {
		node, exists := byID[current]
		if !exists {
			return accessibility.Rect{}, false
		}
		result := uiaNodeBounds(node)
		set := len(node.Rows) != 0 && (result.Width != 0 || result.Height != 0)
		if set {
			result.X += frame.screenX
			result.Y += frame.screenY
		}
		for _, child := range nodes {
			if child.Parent != current {
				continue
			}
			childBounds, childSet := collect(child.ID)
			if childSet {
				result, set = unionUIARect(result, set, childBounds)
			}
		}
		return result, set
	}
	result, _ := collect(id)
	return result
}

func unionUIARect(current accessibility.Rect, set bool, next accessibility.Rect) (accessibility.Rect, bool) {
	if !set {
		return next, true
	}
	left, top := min(current.X, next.X), min(current.Y, next.Y)
	right, bottom := max(current.X+current.Width, next.X+next.Width), max(current.Y+current.Height, next.Y+next.Height)
	return accessibility.Rect{X: left, Y: top, Width: right - left, Height: bottom - top}, true
}

func uiaNodeBounds(node accessibility.NodeSnapshot) accessibility.Rect {
	var result accessibility.Rect
	set := false
	for _, row := range node.Rows {
		for _, rect := range row.Bounds {
			if !set {
				result, set = rect, true
				continue
			}
			left, top := min(result.X, rect.X), min(result.Y, rect.Y)
			right, bottom := max(result.X+result.Width, rect.X+rect.Width), max(result.Y+result.Height, rect.Y+rect.Height)
			result = accessibility.Rect{X: left, Y: top, Width: right - left, Height: bottom - top}
		}
	}
	return result
}
