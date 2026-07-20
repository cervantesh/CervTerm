//go:build glfw

package glfwgl

import (
	"time"
	"unicode/utf8"

	"cervterm/internal/ime"
)

const imeEchoDeadline = 100 * time.Millisecond

type charSuppression struct {
	binding        bool
	echoGeneration uint64
	echoRunes      []rune
	echoIndex      int
	echoDeadline   time.Time
}

func (suppression *charSuppression) armBinding(enabled bool) {
	suppression.binding = enabled
	if enabled {
		suppression.clearEcho()
	}
}

// armIMEEcho is used by the dormant Slice 11.6 decoder but remains disconnected
// from any production native host until the later activation slice.
func (suppression *charSuppression) armIMEEcho(generation uint64, text string, now time.Time) bool {
	if generation == 0 || text == "" || !utf8.ValidString(text) || len(text) > ime.MaxCommitBytes {
		return false
	}
	runes := []rune(text)
	if len(runes) > ime.MaxCommitRunes {
		return false
	}
	suppression.binding = false
	suppression.echoGeneration = generation
	suppression.echoRunes = append(suppression.echoRunes[:0], runes...)
	suppression.echoIndex = 0
	suppression.echoDeadline = now.Add(imeEchoDeadline)
	return true
}

func (suppression *charSuppression) consume(char rune, now time.Time) bool {
	if suppression.binding {
		suppression.binding = false
		return true
	}
	if len(suppression.echoRunes) == 0 {
		return false
	}
	if !now.Before(suppression.echoDeadline) || suppression.echoIndex >= len(suppression.echoRunes) || suppression.echoRunes[suppression.echoIndex] != char {
		suppression.clearEcho()
		return false
	}
	suppression.echoIndex++
	if suppression.echoIndex == len(suppression.echoRunes) {
		suppression.clearEcho()
	}
	return true
}

func (suppression *charSuppression) clearOnNonEchoInput() { suppression.clear() }

func (suppression *charSuppression) clear() {
	suppression.binding = false
	suppression.clearEcho()
}

func (suppression *charSuppression) clearEcho() {
	suppression.echoGeneration = 0
	suppression.echoRunes = suppression.echoRunes[:0]
	suppression.echoIndex = 0
	suppression.echoDeadline = time.Time{}
}

func (suppression *charSuppression) bindingArmed() bool { return suppression.binding }
