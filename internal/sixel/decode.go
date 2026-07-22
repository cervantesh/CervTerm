package sixel

import (
	"context"
	"math"

	"cervterm/internal/termimage"
)

type decodedToken struct {
	kind   byte
	values [5]uint64
	count  int
	char   byte
}
type tokenConsumer interface{ consume(decodedToken) Failure }

func walkTokens(data []byte, consumer tokenConsumer) (Raster, Failure) {
	var raster Raster
	declared := false
	for index := 0; index < len(data); {
		start := data[index]
		token := decodedToken{kind: start}
		index++
		switch {
		case start == '"' || start == '#':
			begin := index
			for index < len(data) && (data[index] >= '0' && data[index] <= '9' || data[index] == ';') {
				index++
			}
			fields, count, ok := decimalFields(data[begin:index])
			if !ok {
				return Raster{}, FailureInvalid
			}
			token.values, token.count = fields, count
			if start == '"' {
				if declared || count != 4 || fields[0] != 1 || fields[1] != 1 || fields[2] == 0 || fields[3] == 0 || fields[2] > math.MaxUint32 || fields[3] > math.MaxUint32 {
					return Raster{}, FailureInvalid
				}
				raster = Raster{Width: uint32(fields[2]), Height: uint32(fields[3])}
				declared = true
			}
			if start == '#' {
				if fields[0] > 255 {
					return Raster{}, FailureInvalid
				}
				if count > 1 {
					if fields[1] == 1 {
						return Raster{}, FailureUnsupported
					}
					if count != 5 || fields[1] != 2 || fields[2] > 100 || fields[3] > 100 || fields[4] > 100 {
						return Raster{}, FailureInvalid
					}
				}
			}
		case start == '!':
			if !declared {
				return Raster{}, FailureInvalid
			}
			begin := index
			for index < len(data) && data[index] >= '0' && data[index] <= '9' {
				index++
			}
			n, ok := decimal(data[begin:index], 4096)
			if !ok || n == 0 || index >= len(data) || data[index] < '?' || data[index] > '~' {
				return Raster{}, FailureInvalid
			}
			token.values[0] = n
			token.char = data[index]
			index++
		case start >= '?' && start <= '~':
			if !declared {
				return Raster{}, FailureInvalid
			}
			token.char = start
		case start == '$' || start == '-':
		default:
			return Raster{}, FailureInvalid
		}
		if failure := consumer.consume(token); failure != FailureNone {
			return Raster{}, failure
		}
	}
	if !declared {
		return Raster{}, FailureInvalid
	}
	return raster, FailureNone
}

type decodeWalker struct {
	width, height uint32
	x, y          uint64
	color         uint8
	operations    uint64
	palette       Palette
	candidate     *termimage.DecodedCandidate
	ctx           context.Context
	limitOnly     bool
}

func (w *decodeWalker) consume(token decodedToken) Failure {
	w.operations++
	if w.operations > maxOperations {
		return FailureLimit
	}
	if w.ctx != nil && w.operations&0xfff == 0 {
		if w.ctx.Err() != nil {
			return FailureCancelled
		}
	}
	switch token.kind {
	case '"':
		return FailureNone
	case '#':
		w.color = uint8(token.values[0])
		if token.count == 5 {
			w.palette[w.color] = RGBA{R: percent(token.values[2]), G: percent(token.values[3]), B: percent(token.values[4]), A: 255}
		}
		return FailureNone
	case '$':
		w.x = 0
		return FailureNone
	case '-':
		w.x = 0
		if w.y > math.MaxUint64-6 {
			return FailureLimit
		}
		w.y += 6
		return FailureNone
	case '!':
		count := token.values[0]
		if w.operations > maxOperations-count {
			return FailureLimit
		}
		w.operations += count
		for index := uint64(0); index < count; index++ {
			if failure := w.column(token.char); failure != FailureNone {
				return failure
			}
		}
		return FailureNone
	default:
		w.operations++
		if w.operations > maxOperations {
			return FailureLimit
		}
		return w.column(token.char)
	}
}
func (w *decodeWalker) column(char byte) Failure {
	if w.x >= uint64(w.width) {
		return FailureInvalid
	}
	bits := char - '?'
	for bit := uint64(0); bit < 6; bit++ {
		if bits&(1<<bit) == 0 {
			continue
		}
		row := w.y + bit
		if row >= uint64(w.height) {
			return FailureInvalid
		}
		if !w.limitOnly {
			color := w.palette[w.color]
			offset := int((row*uint64(w.width) + w.x) * 4)
			pixel := [4]byte{color.R, color.G, color.B, color.A}
			if err := w.candidate.WriteRGBAAt(offset, pixel[:]); err != nil {
				return FailureFailed
			}
		}
	}
	w.x++
	return FailureNone
}
func percent(value uint64) uint8 { return uint8((value*255 + 50) / 100) }
