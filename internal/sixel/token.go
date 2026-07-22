package sixel

import "math"

const maxTokenBytes = 64

type tokenMode uint8

const (
	tokenRoot tokenMode = iota
	tokenRaster
	tokenRepeat
	tokenColor
)

type scanner struct {
	mode     tokenMode
	token    [maxTokenBytes]byte
	n        int
	raster   Raster
	declared bool
	pixels   bool
}

func (s *scanner) feed(data []byte) Failure {
	for index := 0; index < len(data); {
		consumed, failure := s.feedByte(data[index])
		if failure != FailureNone {
			return failure
		}
		if consumed {
			index++
		}
	}
	return FailureNone
}

func (s *scanner) feedByte(value byte) (bool, Failure) {
	switch s.mode {
	case tokenRoot:
		switch {
		case value == '"':
			return true, s.start(tokenRaster, value)
		case value == '!':
			return true, s.start(tokenRepeat, value)
		case value == '#':
			return true, s.start(tokenColor, value)
		case value == '$' || value == '-':
			return true, FailureNone
		case value >= '?' && value <= '~':
			if !s.declared {
				return true, FailureInvalid
			}
			s.pixels = true
			return true, FailureNone
		default:
			return true, FailureInvalid
		}
	case tokenRepeat:
		if value >= '0' && value <= '9' {
			return true, s.append(value)
		}
		if value < '?' || value > '~' || s.n <= 1 || !s.declared {
			return true, FailureInvalid
		}
		count, ok := decimal(s.token[1:s.n], 4096)
		if !ok || count == 0 {
			return true, FailureInvalid
		}
		s.resetToken()
		s.pixels = true
		return true, FailureNone
	case tokenRaster, tokenColor:
		if value >= '0' && value <= '9' || value == ';' {
			return true, s.append(value)
		}
		failure := s.finishBuffered()
		return false, failure
	default:
		return true, FailureInvalid
	}
}

func (s *scanner) start(mode tokenMode, value byte) Failure {
	s.mode, s.n = mode, 0
	return s.append(value)
}

func (s *scanner) append(value byte) Failure {
	if s.n >= len(s.token) {
		return FailureInvalid
	}
	s.token[s.n] = value
	s.n++
	return FailureNone
}

func (s *scanner) finish() Failure {
	if s.mode == tokenRaster || s.mode == tokenColor {
		if failure := s.finishBuffered(); failure != FailureNone {
			return failure
		}
	} else if s.mode != tokenRoot {
		return FailureInvalid
	}
	if !s.declared {
		return FailureInvalid
	}
	return FailureNone
}

func (s *scanner) finishBuffered() Failure {
	var failure Failure
	switch s.mode {
	case tokenRaster:
		failure = s.finishRaster()
	case tokenColor:
		failure = s.finishColor()
	default:
		failure = FailureInvalid
	}
	if failure == FailureNone {
		s.resetToken()
	}
	return failure
}

func (s *scanner) finishRaster() Failure {
	if s.declared || s.n < 2 {
		return FailureInvalid
	}
	fields, count, ok := decimalFields(s.token[1:s.n])
	if !ok || count != 4 || fields[0] != 1 || fields[1] != 1 || fields[2] == 0 || fields[3] == 0 || fields[2] > math.MaxUint32 || fields[3] > math.MaxUint32 {
		return FailureInvalid
	}
	s.raster = Raster{Width: uint32(fields[2]), Height: uint32(fields[3])}
	s.declared = true
	return FailureNone
}

func (s *scanner) finishColor() Failure {
	if s.n < 2 {
		return FailureInvalid
	}
	fields, count, ok := decimalFields(s.token[1:s.n])
	if !ok || fields[0] > 255 {
		return FailureInvalid
	}
	if count == 1 {
		return FailureNone
	}
	if fields[1] == 1 {
		return FailureUnsupported
	}
	if count != 5 || fields[1] != 2 {
		return FailureInvalid
	}
	for _, value := range fields[2:5] {
		if value > 100 {
			return FailureInvalid
		}
	}
	return FailureNone
}

func (s *scanner) resetToken() { s.mode, s.n = tokenRoot, 0 }

func decimal(value []byte, maximum uint64) (uint64, bool) {
	if len(value) == 0 {
		return 0, false
	}
	var result uint64
	for _, digit := range value {
		if digit < '0' || digit > '9' || result > (maximum-uint64(digit-'0'))/10 {
			return 0, false
		}
		result = result*10 + uint64(digit-'0')
	}
	return result, true
}
func decimalFields(value []byte) ([5]uint64, int, bool) {
	var fields [5]uint64
	count, start := 0, 0
	for index := 0; index <= len(value); index++ {
		if index != len(value) && value[index] != ';' {
			continue
		}
		if count >= len(fields) {
			return fields, 0, false
		}
		parsed, ok := decimal(value[start:index], math.MaxUint32)
		if !ok {
			return fields, 0, false
		}
		fields[count] = parsed
		count++
		start = index + 1
	}
	return fields, count, true
}
