//go:build glfw && windows

package glfwgl

import (
	"unicode/utf16"
	"unsafe"

	"cervterm/internal/accessibility"
)

var nativeUIAClientsListening = func() bool {
	result, _, _ := uiaClientsListeningProc.Call()
	return result != 0
}

func (provider *nativeUIAProvider) RaiseSemanticEvent(event accessibility.SemanticEvent) error {
	if provider == nil || provider.root == nil || !provider.root.available() {
		return errUIAProviderInvalid
	}
	if !nativeUIAClientsListening() {
		return nil
	}
	if event.Kind == accessibility.EventAnnouncement {
		return provider.raiseAnnouncement(event.Announcement)
	}
	structureChanged := false
	var eventID uint32
	switch event.Kind {
	case accessibility.EventDocumentInvalidated, accessibility.EventTopologyChanged:
		structureChanged = true
	case accessibility.EventTextChanged:
		eventID = 20015 // UIA_Text_TextChangedEventId
	case accessibility.EventCaretChanged, accessibility.EventSelectionChanged:
		eventID = 20014 // UIA_Text_TextSelectionChangedEventId
	case accessibility.EventFocusChanged:
		eventID = 20005 // UIA_AutomationFocusChangedEventId
	default:
		return nil
	}
	provider.mu.Lock()
	pointer := provider.pointer
	if object := provider.objects[event.Node]; object != nil && object.simple != 0 {
		pointer = object.simple
	}
	provider.mu.Unlock()
	if pointer == 0 {
		return errUIAProviderInvalid
	}
	var result uintptr
	if structureChanged {
		result, _, _ = uiaRaiseStructureEvent.Call(pointer, 2, 0, 0) // ChildrenInvalidated
	} else {
		result, _, _ = uiaRaiseAutomationEvent.Call(pointer, uintptr(eventID))
	}
	if int32(result) < 0 {
		return errUIAProviderInvalid
	}
	return nil
}

func (provider *nativeUIAProvider) raiseAnnouncement(kind accessibility.AnnouncementKind) error {
	message := ""
	switch kind {
	case accessibility.AnnouncementBell:
		message = "Terminal bell"
	case accessibility.AnnouncementNotification:
		message = "Terminal notification"
	default:
		return nil
	}
	messageUnits := utf16.Encode([]rune(message))
	activityUnits := utf16.Encode([]rune("cervterm-terminal"))
	messageBSTR, _, _ := uiaSysAllocStringLen.Call(uintptr(unsafe.Pointer(&messageUnits[0])), uintptr(len(messageUnits)))
	if messageBSTR == 0 {
		return errUIAProviderInvalid
	}
	defer uiaSysFreeString.Call(messageBSTR)
	activityBSTR, _, _ := uiaSysAllocStringLen.Call(uintptr(unsafe.Pointer(&activityUnits[0])), uintptr(len(activityUnits)))
	if activityBSTR == 0 {
		return errUIAProviderInvalid
	}
	defer uiaSysFreeString.Call(activityBSTR)
	result, _, _ := uiaRaiseNotificationEvent.Call(provider.pointer, 4, 2, messageBSTR, activityBSTR)
	if int32(result) < 0 {
		return errUIAProviderInvalid
	}
	return nil
}
