package itermimage

import "math"

const filePrefix = "File="
const maxFieldNameBytes = len("preserveAspectRatio")

type scanMode uint8

const (
	scanPrefix scanMode = iota
	scanFieldName
	scanFieldValue
	scanBase64
)

type metadataField uint8

const (
	fieldNone metadataField = iota
	fieldInline
	fieldSize
	fieldWidth
	fieldHeight
	fieldPreserveAspectRatio
)

const (
	seenInline uint8 = 1 << iota
	seenSize
	seenWidth
	seenHeight
	seenPreserveAspectRatio
)

type scanner struct {
	mode        scanMode
	prefixIndex int
	name        [maxFieldNameBytes]byte
	nameLen     int
	field       metadataField
	value       uint64
	digits      int
	seen        uint8
	metadata    Metadata
	base64      base64Scanner
}

// feed validates data incrementally and returns the sub-slice containing only
// base64 payload. Metadata is held only in fixed-size scanner state.
func (s *scanner) feed(data []byte) ([]byte, Failure) {
	payloadStart := len(data)
	for index, value := range data {
		if s.mode == scanBase64 && payloadStart == len(data) {
			payloadStart = index
		}
		if failure := s.feedByte(value); failure != FailureNone {
			return nil, failure
		}
	}
	return data[payloadStart:], FailureNone
}

func (s *scanner) feedByte(value byte) Failure {
	if isWhitespace(value) {
		return FailureInvalid
	}
	switch s.mode {
	case scanPrefix:
		if s.prefixIndex >= len(filePrefix) || value != filePrefix[s.prefixIndex] {
			return FailureUnsupported
		}
		s.prefixIndex++
		if s.prefixIndex == len(filePrefix) {
			s.mode = scanFieldName
		}
		return FailureNone
	case scanFieldName:
		if value == '=' {
			return s.finishFieldName()
		}
		if value == ';' || value == ':' || s.nameLen >= len(s.name) || !isFieldNameByte(value) {
			return FailureInvalid
		}
		s.name[s.nameLen] = value
		s.nameLen++
		return FailureNone
	case scanFieldValue:
		if value == ';' || value == ':' {
			if failure := s.finishFieldValue(); failure != FailureNone {
				return failure
			}
			if value == ':' {
				if s.seen&seenInline == 0 || s.seen&seenSize == 0 {
					return FailureInvalid
				}
				s.mode = scanBase64
			} else {
				s.mode = scanFieldName
			}
			return FailureNone
		}
		if value < '0' || value > '9' {
			if s.field == fieldWidth || s.field == fieldHeight {
				return FailureUnsupported
			}
			return FailureInvalid
		}
		digit := uint64(value - '0')
		if s.value > (math.MaxUint64-digit)/10 {
			return FailureLimit
		}
		s.value = s.value*10 + digit
		s.digits++
		if (s.field == fieldWidth || s.field == fieldHeight) && s.value > uint64(^uint16(0)) {
			return FailureLimit
		}
		return FailureNone
	case scanBase64:
		return s.base64.feedByte(value)
	default:
		return FailureInvalid
	}
}

func (s *scanner) finishFieldName() Failure {
	if s.nameLen == 0 {
		return FailureInvalid
	}
	name := s.name[:s.nameLen]
	s.nameLen = 0
	s.value = 0
	s.digits = 0
	switch {
	case bytesEqualString(name, "inline"):
		s.field = fieldInline
		if s.seen&seenInline != 0 {
			return FailureInvalid
		}
	case bytesEqualString(name, "size"):
		s.field = fieldSize
		if s.seen&seenSize != 0 {
			return FailureInvalid
		}
	case bytesEqualString(name, "width"):
		s.field = fieldWidth
		if s.seen&seenWidth != 0 {
			return FailureInvalid
		}
		if s.seen&seenHeight != 0 {
			return FailureUnsupported
		}
	case bytesEqualString(name, "height"):
		s.field = fieldHeight
		if s.seen&seenHeight != 0 {
			return FailureInvalid
		}
		if s.seen&seenWidth != 0 {
			return FailureUnsupported
		}
	case bytesEqualString(name, "preserveAspectRatio"):
		s.field = fieldPreserveAspectRatio
		if s.seen&seenPreserveAspectRatio != 0 {
			return FailureInvalid
		}
	case bytesEqualString(name, "name"):
		return FailureUnsupported
	default:
		return FailureInvalid
	}
	s.mode = scanFieldValue
	return FailureNone
}

func bytesEqualString(value []byte, expected string) bool {
	if len(value) != len(expected) {
		return false
	}
	for index := range value {
		if value[index] != expected[index] {
			return false
		}
	}
	return true
}

func (s *scanner) finishFieldValue() Failure {
	if s.digits == 0 {
		return FailureInvalid
	}
	switch s.field {
	case fieldInline:
		if s.digits != 1 {
			return FailureInvalid
		}
		if s.value == 0 {
			return FailureUnsupported
		}
		if s.value != 1 {
			return FailureInvalid
		}
		s.seen |= seenInline
	case fieldSize:
		if s.value == 0 {
			return FailureInvalid
		}
		s.metadata.Size = s.value
		s.seen |= seenSize
	case fieldWidth:
		if s.value == 0 {
			return FailureInvalid
		}
		if s.value > 256 {
			return FailureLimit
		}
		s.metadata.Axis = SizingWidth
		s.metadata.Cells = uint16(s.value)
		s.seen |= seenWidth
	case fieldHeight:
		if s.value == 0 {
			return FailureInvalid
		}
		if s.value > 256 {
			return FailureLimit
		}
		s.metadata.Axis = SizingHeight
		s.metadata.Cells = uint16(s.value)
		s.seen |= seenHeight
	case fieldPreserveAspectRatio:
		if s.digits != 1 {
			return FailureInvalid
		}
		if s.value == 0 {
			return FailureUnsupported
		}
		if s.value != 1 {
			return FailureInvalid
		}
		s.seen |= seenPreserveAspectRatio
	default:
		return FailureInvalid
	}
	s.metadata.PreserveAspectRatio = true
	s.field = fieldNone
	s.value = 0
	s.digits = 0
	return FailureNone
}

func (s *scanner) finish() Failure {
	if s.mode != scanBase64 || s.seen&seenInline == 0 || s.seen&seenSize == 0 {
		return FailureInvalid
	}
	return s.base64.finish()
}

type base64Scanner struct {
	position      uint8
	second        byte
	third         byte
	needSecondPad bool
	terminal      bool
	count         uint64
}

func (s *base64Scanner) feedByte(value byte) Failure {
	if s.terminal {
		return FailureInvalid
	}
	if value == '=' {
		switch {
		case s.position == 2 && !s.needSecondPad:
			if s.second&0x0f != 0 {
				return FailureInvalid
			}
			s.needSecondPad = true
			s.position = 3
		case s.position == 3 && s.needSecondPad:
			s.needSecondPad = false
			s.position = 0
			s.terminal = true
		case s.position == 3:
			if s.third&0x03 != 0 {
				return FailureInvalid
			}
			s.position = 0
			s.terminal = true
		default:
			return FailureInvalid
		}
		s.count++
		return FailureNone
	}
	if s.needSecondPad {
		return FailureInvalid
	}
	sextet, ok := base64Sextet(value)
	if !ok {
		return FailureInvalid
	}
	switch s.position {
	case 1:
		s.second = sextet
	case 2:
		s.third = sextet
	}
	s.position++
	if s.position == 4 {
		s.position = 0
	}
	s.count++
	return FailureNone
}

func (s *base64Scanner) finish() Failure {
	if s.count == 0 || s.position != 0 || s.needSecondPad {
		return FailureInvalid
	}
	return FailureNone
}

func base64Sextet(value byte) (byte, bool) {
	switch {
	case value >= 'A' && value <= 'Z':
		return value - 'A', true
	case value >= 'a' && value <= 'z':
		return value - 'a' + 26, true
	case value >= '0' && value <= '9':
		return value - '0' + 52, true
	case value == '+':
		return 62, true
	case value == '/':
		return 63, true
	default:
		return 0, false
	}
}

func isWhitespace(value byte) bool {
	switch value {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}

func isFieldNameByte(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}
