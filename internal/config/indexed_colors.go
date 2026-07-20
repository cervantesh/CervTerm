package config

import (
	"bytes"
	"fmt"
	"strconv"
)

const (
	firstIndexedColor = 16
	indexedColorCount = 240
)

// IndexedColorOverrides stores optional overrides for terminal palette indexes
// 16 through 255. Array slot zero corresponds to palette index 16.
type IndexedColorOverrides [indexedColorCount]string

// Lookup returns the configured override, or an empty string when the index is
// below the overridable range or the slot has no override.
func (colors IndexedColorOverrides) Lookup(index uint8) string {
	if index < firstIndexedColor {
		return ""
	}
	return colors[int(index)-firstIndexedColor]
}

// Set updates one overridable palette index. Indexes below 16 are owned by the
// ANSI palette and are rejected.
func (colors *IndexedColorOverrides) Set(index uint8, value string) error {
	if index < firstIndexedColor {
		return fmt.Errorf("indexed color %d must be between 16 and 255", index)
	}
	colors[int(index)-firstIndexedColor] = value
	return nil
}

// MarshalJSON emits only configured slots. Keys are written in numeric order
// so diagnostics and other reflection-based JSON remain deterministic.
func (colors IndexedColorOverrides) MarshalJSON() ([]byte, error) {
	var out bytes.Buffer
	out.WriteByte('{')
	first := true
	for slot, value := range colors {
		if value == "" {
			continue
		}
		if !first {
			out.WriteByte(',')
		}
		first = false
		out.WriteString(strconv.Quote(strconv.Itoa(slot + firstIndexedColor)))
		out.WriteByte(':')
		out.WriteString(strconv.Quote(value))
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}
