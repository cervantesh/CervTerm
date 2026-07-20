//go:build glfw && windows

package glfwgl

import (
	"errors"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/go-gl/glfw/v3.3/glfw"
)

const (
	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifIcon     = 0x00000002
	nifTip      = 0x00000004
	nifInfo     = 0x00000010
	nifRealtime = 0x00000040

	niifInfo             = 0x00000001
	niifRespectQuietTime = 0x00000080
	cervTermNotifyIconID = 0x43565254
	idiApplication       = 32512
)

type notifyIconDataW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	Tip              [128]uint16
	State            uint32
	StateMask        uint32
	Info             [256]uint16
	TimeoutOrVersion uint32
	InfoTitle        [64]uint16
	InfoFlags        uint32
	GUIDItem         [16]byte
	HBalloonIcon     uintptr
}

var (
	shellNotifyIconWProc = windows.NewLazySystemDLL("shell32.dll").NewProc("Shell_NotifyIconW")
	loadIconWProc        = windows.NewLazySystemDLL("user32.dll").NewProc("LoadIconW")
	shellNotifyIconCall  = func(message uint32, data *notifyIconDataW) bool {
		result, _, _ := shellNotifyIconWProc.Call(uintptr(message), uintptr(unsafe.Pointer(data)))
		return result != 0
	}
	loadApplicationIcon = func() uintptr {
		icon, _, _ := loadIconWProc.Call(0, idiApplication)
		return icon
	}
)

type windowsNotificationEffectSink struct {
	hwnd  uintptr
	added bool
}

func newPlatformNotificationEffectSink(window *glfw.Window) notificationEffectSink {
	var hwnd uintptr
	if window != nil {
		hwnd = uintptr(unsafe.Pointer(window.GetWin32Window()))
	}
	return &windowsNotificationEffectSink{hwnd: hwnd}
}

func (sink *windowsNotificationEffectSink) Notify(title, body string) error {
	if sink.hwnd == 0 {
		return errors.New("native notification window unavailable")
	}
	data := notifyIconDataW{CbSize: uint32(unsafe.Sizeof(notifyIconDataW{})), HWnd: sink.hwnd, UID: cervTermNotifyIconID}
	if !sink.added {
		data.HIcon = loadApplicationIcon()
		if data.HIcon == 0 {
			return errors.New("native notification icon unavailable")
		}
		data.UFlags = nifIcon | nifTip
		copyUTF16(data.Tip[:], "CervTerm")
		if !shellNotifyIconCall(nimAdd, &data) {
			return errors.New("native notification icon add failed")
		}
		sink.added = true
	}
	if title == "" {
		title = "CervTerm"
	}
	data.UFlags = nifInfo | nifRealtime
	data.InfoFlags = niifInfo | niifRespectQuietTime
	copyUTF16(data.InfoTitle[:], title)
	copyUTF16(data.Info[:], body)
	if !shellNotifyIconCall(nimModify, &data) {
		_ = sink.Close()
		return errors.New("native notification display failed")
	}
	return nil
}

func (sink *windowsNotificationEffectSink) Close() error {
	if !sink.added {
		return nil
	}
	data := notifyIconDataW{CbSize: uint32(unsafe.Sizeof(notifyIconDataW{})), HWnd: sink.hwnd, UID: cervTermNotifyIconID}
	if !shellNotifyIconCall(nimDelete, &data) {
		return errors.New("native notification icon delete failed")
	}
	sink.added = false
	return nil
}

func copyUTF16(destination []uint16, value string) {
	if len(destination) == 0 {
		return
	}
	written := 0
	for _, r := range value {
		if r <= 0xffff {
			if written+1 >= len(destination) {
				break
			}
			destination[written] = uint16(r)
			written++
			continue
		}
		if written+2 >= len(destination) {
			break
		}
		first, second := utf16.EncodeRune(r)
		destination[written], destination[written+1] = uint16(first), uint16(second)
		written += 2
	}
	destination[written] = 0
}
