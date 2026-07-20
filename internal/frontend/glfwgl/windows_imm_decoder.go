//go:build glfw

package glfwgl

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
	"unicode/utf16"

	"cervterm/internal/ime"
)

const (
	wmIMEStartComposition = 0x010d
	wmIMEEndComposition   = 0x010e
	wmIMEComposition      = 0x010f

	gcsCompStr   = 0x0008
	gcsCompAttr  = 0x0010
	gcsCursorPos = 0x0080
	gcsResultStr = 0x0800
)

var (
	errIMMContextUnavailable = errors.New("IMM context is unavailable")
	errIMMReadInvalid        = errors.New("IMM composition read is invalid")
	errIMMEchoNotArmed       = errors.New("IMM commit echo suppression was not armed")
	errIMMCallbackPanic      = errors.New("IMM callback panic")
	errIMMCandidateInvalid   = errors.New("IMM candidate rectangle is invalid")
	errIMMCommitMismatch     = errors.New("IMM commit callback returned different text")
)

type immContextAPI interface {
	Acquire(hwnd uintptr) (uintptr, error)
	Release(hwnd, context uintptr) error
	Read(context uintptr, index uint32, destination []byte) (int32, error)
	SetCandidate(context uintptr, rect nativeCandidateRect, visible bool) error
}

type immCallbacks struct {
	Start   func() (uint64, error)
	Update  func(uint64, ime.NativeUpdate) error
	Commit  func(uint64, []uint16) (string, error)
	Cancel  func(ime.CancelReason) error
	ArmEcho func(uint64, string, time.Time) bool
	Now     func() time.Time
}

type immDecoder struct {
	hwnd       uintptr
	api        immContextAPI
	callbacks  immCallbacks
	generation uint64
	active     bool
}

type immPayload struct {
	result  []uint16
	preedit *ime.NativeUpdate
}

func (decoder *immDecoder) handleMessage(message uint32, lParam uintptr) (handled bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.Join(errIMMCallbackPanic, decoder.cancelMalformed())
			handled = true
		}
	}()
	switch message {
	case wmIMEStartComposition:
		if decoder.active {
			return true, nil
		}
		return true, decoder.start()
	case wmIMEEndComposition:
		if !decoder.active {
			return true, nil
		}
		err := decoder.safeCancel(ime.CancelExplicit)
		decoder.active, decoder.generation = false, 0
		return true, err
	case wmIMEComposition:
		return true, decoder.handleComposition(uint32(lParam))
	default:
		return false, nil
	}
}

func (decoder *immDecoder) start() error {
	if decoder.callbacks.Start == nil {
		return errIMMContextUnavailable
	}
	generation, err := decoder.callbacks.Start()
	if err != nil {
		return errors.Join(err, decoder.safeCancel(ime.CancelMalformed))
	}
	if generation == 0 {
		return errors.Join(ime.ErrInvalidGeneration, decoder.safeCancel(ime.CancelMalformed))
	}
	decoder.generation, decoder.active = generation, true
	return nil
}

func (decoder *immDecoder) handleComposition(flags uint32) (err error) {
	if decoder.api == nil {
		return errIMMContextUnavailable
	}
	context, err := decoder.api.Acquire(decoder.hwnd)
	if err != nil || context == 0 {
		return errors.Join(errIMMContextUnavailable, err, decoder.cancelMalformed())
	}
	defer func() { err = errors.Join(err, decoder.api.Release(decoder.hwnd, context)) }()
	payload, err := decoder.readPayload(context, flags)
	if err != nil {
		return errors.Join(err, decoder.cancelMalformed())
	}
	if err := validateIMMPayload(payload); err != nil {
		return errors.Join(err, decoder.cancelMalformed())
	}
	if len(payload.result) > 0 {
		if !decoder.active {
			if err := decoder.start(); err != nil {
				return err
			}
		}
		generation := decoder.generation
		text, err := decoder.callbacks.Commit(generation, payload.result)
		if err != nil {
			return errors.Join(err, decoder.cancelMalformed())
		}
		decoder.active, decoder.generation = false, 0
		if text != string(utf16.Decode(payload.result)) {
			return errIMMCommitMismatch
		}
		now := time.Now()
		if decoder.callbacks.Now != nil {
			now = decoder.callbacks.Now()
		}
		if decoder.callbacks.ArmEcho == nil || !decoder.callbacks.ArmEcho(generation, text, now) {
			return errIMMEchoNotArmed
		}
	}
	if payload.preedit != nil {
		if !decoder.active {
			if err := decoder.start(); err != nil {
				return err
			}
		}
		if decoder.callbacks.Update == nil {
			return errors.Join(errIMMContextUnavailable, decoder.cancelMalformed())
		}
		if err := decoder.callbacks.Update(decoder.generation, *payload.preedit); err != nil {
			return errors.Join(err, decoder.cancelMalformed())
		}
	}
	return nil
}

func (decoder *immDecoder) readPayload(context uintptr, flags uint32) (immPayload, error) {
	var payload immPayload
	if flags&gcsResultStr != 0 {
		data, err := decoder.readBytes(context, gcsResultStr, 2*ime.MaxCommitUTF16Units, true)
		if err != nil {
			return immPayload{}, err
		}
		payload.result = bytesToUTF16(data)
	}
	if flags&(gcsCompStr|gcsCompAttr|gcsCursorPos) != 0 {
		cursorBefore, err := decoder.readCursor(context)
		if err != nil {
			return immPayload{}, err
		}
		text, err := decoder.readBytes(context, gcsCompStr, 2*ime.MaxPreeditUTF16Units, true)
		if err != nil {
			return immPayload{}, err
		}
		attributes, err := decoder.readBytes(context, gcsCompAttr, ime.MaxPreeditUTF16Units, false)
		if err != nil {
			return immPayload{}, err
		}
		cursorAfter, err := decoder.readCursor(context)
		if err != nil || cursorBefore != cursorAfter {
			return immPayload{}, errors.Join(errIMMReadInvalid, err)
		}
		units := bytesToUTF16(text)
		if cursorBefore < 0 || cursorBefore > len(units) || (len(attributes) != 0 && len(attributes) != len(units)) {
			return immPayload{}, errIMMReadInvalid
		}
		payload.preedit = &ime.NativeUpdate{UTF16: units, CursorUTF16: cursorBefore, Attributes: attributes}
	}
	return payload, nil
}

func validateIMMPayload(payload immPayload) error {
	var validator ime.Controller
	generation, err := validator.Start(ime.Target{Kind: ime.TargetPane, ID: 1, Activation: 1})
	if err != nil {
		return err
	}
	if payload.preedit != nil {
		if err := validator.Update(generation, *payload.preedit); err != nil {
			return err
		}
	}
	if len(payload.result) > 0 {
		_, err = validator.Commit(generation, payload.result)
	}
	return err
}

func (decoder *immDecoder) readCursor(context uintptr) (int, error) {
	value, err := decoder.api.Read(context, gcsCursorPos, nil)
	if err != nil || value < 0 {
		return 0, errors.Join(errIMMReadInvalid, err)
	}
	return int(value), nil
}

func (decoder *immDecoder) readBytes(context uintptr, index uint32, limit int, even bool) ([]byte, error) {
	size, err := decoder.api.Read(context, index, nil)
	if err != nil || size < 0 || int64(size) > int64(limit) || (even && size%2 != 0) {
		return nil, errors.Join(errIMMReadInvalid, err)
	}
	if size == 0 {
		return nil, nil
	}
	buffer := make([]byte, int(size))
	read, err := decoder.api.Read(context, index, buffer)
	if err != nil || read != size {
		return nil, errors.Join(errIMMReadInvalid, err)
	}
	return buffer, nil
}

func bytesToUTF16(data []byte) []uint16 {
	units := make([]uint16, len(data)/2)
	for index := range units {
		units[index] = binary.LittleEndian.Uint16(data[index*2:])
	}
	return units
}

func (decoder *immDecoder) cancelMalformed() error {
	err := decoder.safeCancel(ime.CancelMalformed)
	decoder.active, decoder.generation = false, 0
	return err
}

func (decoder *immDecoder) safeCancel(reason ime.CancelReason) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errIMMCallbackPanic
		}
	}()
	if decoder.callbacks.Cancel == nil {
		return nil
	}
	return decoder.callbacks.Cancel(reason)
}

func (decoder *immDecoder) publishCandidate(rect nativeCandidateRect) (err error) {
	return decoder.withContext(func(context uintptr) error { return decoder.api.SetCandidate(context, rect, true) })
}

func (decoder *immDecoder) clearCandidate() (err error) {
	return decoder.withContext(func(context uintptr) error { return decoder.api.SetCandidate(context, nativeCandidateRect{}, false) })
}

func (decoder *immDecoder) withContext(use func(uintptr) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.Join(err, fmt.Errorf("%w", errIMMCallbackPanic))
		}
	}()
	if decoder.api == nil || use == nil {
		return errIMMContextUnavailable
	}
	context, err := decoder.api.Acquire(decoder.hwnd)
	if err != nil || context == 0 {
		return errors.Join(errIMMContextUnavailable, err)
	}
	defer func() { err = errors.Join(err, decoder.api.Release(decoder.hwnd, context)) }()
	return use(context)
}

func newDormantIMMDecoder(app *App, hwnd uintptr, api immContextAPI) *immDecoder {
	if app == nil {
		return &immDecoder{hwnd: hwnd, api: api}
	}
	return &immDecoder{hwnd: hwnd, api: api, callbacks: immCallbacks{
		Start:  app.composition.start,
		Update: app.composition.update,
		Commit: app.composition.commitText,
		Cancel: app.cancelComposition,
		ArmEcho: func(generation uint64, text string, now time.Time) bool {
			return app.charSuppression.armIMEEcho(generation, text, now)
		},
		Now: time.Now,
	}}
}
