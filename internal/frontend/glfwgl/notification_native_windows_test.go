//go:build glfw && windows

package glfwgl

import (
	"strings"
	"testing"
	"unicode/utf16"
	"unsafe"
)

func TestWindowsNotificationAdapterLifecycle(t *testing.T) {
	oldCall, oldLoad := shellNotifyIconCall, loadApplicationIcon
	defer func() { shellNotifyIconCall, loadApplicationIcon = oldCall, oldLoad }()
	loadApplicationIcon = func() uintptr { return 99 }
	var messages []uint32
	var records []notifyIconDataW
	shellNotifyIconCall = func(message uint32, data *notifyIconDataW) bool {
		messages = append(messages, message)
		records = append(records, *data)
		return true
	}
	sink := &windowsNotificationEffectSink{hwnd: 42}
	if err := sink.Notify("Build", "complete"); err != nil {
		t.Fatal(err)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 || messages[0] != nimAdd || messages[1] != nimModify || messages[2] != nimDelete {
		t.Fatalf("messages = %v", messages)
	}
	if records[0].HWnd != 42 || records[0].UID != cervTermNotifyIconID || records[0].UFlags != nifIcon|nifTip || records[0].HIcon != 99 {
		t.Fatalf("add = %#v", records[0])
	}
	if records[1].UFlags != nifInfo|nifRealtime || records[1].InfoFlags != niifInfo|niifRespectQuietTime || utf16String(records[1].InfoTitle[:]) != "Build" || utf16String(records[1].Info[:]) != "complete" {
		t.Fatalf("modify flags=%x infoflags=%x title=%q body=%q", records[1].UFlags, records[1].InfoFlags, utf16String(records[1].InfoTitle[:]), utf16String(records[1].Info[:]))
	}
}

func TestWindowsNotificationAdapterFailureRollsBack(t *testing.T) {
	oldCall, oldLoad := shellNotifyIconCall, loadApplicationIcon
	defer func() { shellNotifyIconCall, loadApplicationIcon = oldCall, oldLoad }()
	loadApplicationIcon = func() uintptr { return 99 }
	var messages []uint32
	shellNotifyIconCall = func(message uint32, _ *notifyIconDataW) bool {
		messages = append(messages, message)
		return message != nimModify
	}
	sink := &windowsNotificationEffectSink{hwnd: 42}
	if err := sink.Notify("", "body"); err == nil || sink.added {
		t.Fatalf("failure err=%v added=%t", err, sink.added)
	}
	if len(messages) != 3 || messages[2] != nimDelete {
		t.Fatalf("rollback messages = %v", messages)
	}
}

func TestWindowsNotificationDeleteFailureRetainsOwnershipForRetry(t *testing.T) {
	oldCall := shellNotifyIconCall
	defer func() { shellNotifyIconCall = oldCall }()
	deleteCalls := 0
	shellNotifyIconCall = func(message uint32, _ *notifyIconDataW) bool {
		if message == nimDelete {
			deleteCalls++
			return deleteCalls > 1
		}
		return true
	}
	sink := &windowsNotificationEffectSink{hwnd: 42, added: true}
	if err := sink.Close(); err == nil || !sink.added {
		t.Fatalf("first close err=%v retained=%t", err, sink.added)
	}
	if err := sink.Close(); err != nil || sink.added {
		t.Fatalf("retry close err=%v retained=%t", err, sink.added)
	}
	if deleteCalls != 2 {
		t.Fatalf("delete calls = %d, want 2", deleteCalls)
	}
}

func TestWindowsNotificationUTF16IsBoundedAndRuneSafe(t *testing.T) {
	var destination [5]uint16
	copyUTF16(destination[:], "A😀BC")
	if got := utf16String(destination[:]); got != "A😀B" {
		t.Fatalf("UTF-16 copy = %q", got)
	}
	wantSize := uintptr(956)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		wantSize = 976
	}
	if got := unsafe.Sizeof(notifyIconDataW{}); got != wantSize {
		t.Fatalf("NOTIFYICONDATAW size = %d, want %d", got, wantSize)
	}
}

func utf16String(value []uint16) string {
	end := 0
	for end < len(value) && value[end] != 0 {
		end++
	}
	return strings.TrimRight(string(utf16.Decode(value[:end])), "\x00")
}
