package kitty

import (
	"strconv"
	"strings"

	"cervterm/internal/termimage"
)

const maxHeaderBytes = 4096
const maxPayloadBytes = 4096

type parsedFrame struct {
	command Command
	reply   ReplyPlan
	more    bool
	payload []byte
}
type parseFailure struct {
	code  ReplyCode
	reply ReplyPlan
}

func scanQuiet(control string) Quiet {
	quiet := QuietNormal
	for _, pair := range strings.Split(control, ",") {
		if strings.HasPrefix(pair, "q=") && len(pair) == 3 && pair[2] >= '0' && pair[2] <= '2' {
			value := Quiet(pair[2] - '0')
			if value > quiet {
				quiet = value
			}
		}
	}
	return quiet
}

func parsePairs(control string) (map[byte]string, ReplyPlan, *parseFailure) {
	reply := ReplyPlan{quiet: scanQuiet(control)}
	if control == "" || len(control) > maxHeaderBytes {
		return nil, reply, &parseFailure{ReplyInvalid, reply}
	}
	fields := make(map[byte]string)
	for _, pair := range strings.Split(control, ",") {
		if len(pair) < 3 || pair[1] != '=' || pair[0] > 0x7f || pair[2:] == "" {
			return nil, reply, &parseFailure{ReplyInvalid, reply}
		}
		key := pair[0]
		if _, exists := fields[key]; exists {
			return nil, reply, &parseFailure{ReplyInvalid, reply}
		}
		for i := 2; i < len(pair); i++ {
			if pair[i] < 0x21 || pair[i] > 0x7e {
				return nil, reply, &parseFailure{ReplyInvalid, reply}
			}
		}
		fields[key] = pair[2:]
	}
	return fields, reply, nil
}

func parseCompleteFrame(frame []byte, continuation bool) (parsedFrame, *parseFailure) {
	if len(frame) < 2 || frame[0] != 'G' {
		return parsedFrame{}, &parseFailure{code: ReplyUnsupported}
	}
	body := string(frame[1:])
	control, payload, _ := strings.Cut(body, ";")
	if len(payload) > maxPayloadBytes {
		reply := ReplyPlan{quiet: scanQuiet(control)}
		return parsedFrame{}, &parseFailure{ReplyLimit, reply}
	}
	fields, reply, failure := parsePairs(control)
	if failure != nil {
		return parsedFrame{}, failure
	}
	if continuation {
		return parseContinuation(fields, []byte(payload), reply)
	}
	action := ActionTransmit
	if value, ok := fields['a']; ok {
		if len(value) != 1 {
			return parsedFrame{}, &parseFailure{ReplyUnsupported, reply}
		}
		action = Action(value[0])
	}
	reply.action = action
	if action != ActionTransmit && action != ActionTransmitAndPlace && action != ActionPlace && action != ActionDelete && action != ActionQuery {
		return parsedFrame{}, &parseFailure{ReplyUnsupported, reply}
	}
	allowed := allowedFields(action)
	for key := range fields {
		if !allowed[key] {
			return parsedFrame{}, &parseFailure{ReplyUnsupported, reply}
		}
	}
	quiet, ok := parseUnsigned(fields, 'q', 0, 2, 0)
	if !ok {
		return parsedFrame{}, &parseFailure{ReplyInvalid, reply}
	}
	reply.quiet = Quiet(quiet)
	image, ok := parseUnsigned(fields, 'i', 1, uint64(^uint32(0)), 0)
	if !ok || (action != ActionDelete && image == 0) || (action == ActionDelete && (fields['d'] == "i" || fields['d'] == "I") && image == 0) {
		return parsedFrame{}, &parseFailure{ReplyInvalid, reply}
	}
	command := Command{Action: action, Image: termimage.ImageID(image)}
	more, ok := parseUnsigned(fields, 'm', 0, 1, 0)
	if !ok {
		return parsedFrame{}, &parseFailure{ReplyInvalid, reply}
	}
	if action == ActionTransmit || action == ActionTransmitAndPlace || action == ActionQuery {
		if transport, exists := fields['t']; exists && transport != "d" {
			return parsedFrame{}, &parseFailure{ReplyUnsupported, reply}
		}
		format, ok := parseUnsigned(fields, 'f', 0, 100, 32)
		if !ok || (format != 24 && format != 32 && format != 100) {
			return parsedFrame{}, &parseFailure{ReplyUnsupported, reply}
		}
		command.Decode.Format = PixelFormat(format)
		if compression, exists := fields['o']; exists {
			if compression != "z" || format == 100 {
				return parsedFrame{}, &parseFailure{ReplyUnsupported, reply}
			}
			command.Decode.Compression = CompressionZlib
		}
		width, wok := parseUnsigned(fields, 's', 0, uint64(termimage.HardImageDimension), 0)
		height, hok := parseUnsigned(fields, 'v', 0, uint64(termimage.HardImageDimension), 0)
		if !wok || !hok || (format != 100 && (width == 0 || height == 0 || width*height > termimage.HardImagePixels)) || (format == 100 && (has(fields, 's') || has(fields, 'v'))) {
			return parsedFrame{}, &parseFailure{ReplyInvalid, reply}
		}
		command.Decode.Width, command.Decode.Height = uint32(width), uint32(height)
		if more == 1 && (len(payload) == 0 || len(payload)%4 != 0) {
			return parsedFrame{}, &parseFailure{ReplyInvalid, reply}
		}
	} else if len(payload) != 0 || more != 0 {
		return parsedFrame{}, &parseFailure{ReplyInvalid, reply}
	}
	if action == ActionTransmitAndPlace || action == ActionPlace {
		placement, fail := parsePlacement(fields)
		if fail != ReplyNone {
			return parsedFrame{}, &parseFailure{fail, reply}
		}
		command.Placement = placement
	}
	if action == ActionDelete {
		selector, fail := parseDelete(fields, command.Image)
		if fail != ReplyNone {
			return parsedFrame{}, &parseFailure{fail, reply}
		}
		command.Delete = selector
	}
	return parsedFrame{command: command, reply: reply, more: more == 1, payload: append([]byte(nil), payload...)}, nil
}

func parseContinuation(fields map[byte]string, payload []byte, reply ReplyPlan) (parsedFrame, *parseFailure) {
	for key := range fields {
		if key != 'm' && key != 'q' {
			return parsedFrame{}, &parseFailure{ReplyInvalid, reply}
		}
	}
	more, ok := parseUnsigned(fields, 'm', 0, 1, 0)
	if !ok || !has(fields, 'm') {
		return parsedFrame{}, &parseFailure{ReplyInvalid, reply}
	}
	quiet, ok := parseUnsigned(fields, 'q', 0, 2, uint64(reply.quiet))
	if !ok {
		return parsedFrame{}, &parseFailure{ReplyInvalid, reply}
	}
	reply.quiet = Quiet(quiet)
	if more == 1 && (len(payload) == 0 || len(payload)%4 != 0) {
		return parsedFrame{}, &parseFailure{ReplyInvalid, reply}
	}
	return parsedFrame{reply: reply, more: more == 1, payload: append([]byte(nil), payload...)}, nil
}

func allowedFields(action Action) map[byte]bool {
	allowed := map[byte]bool{'a': true, 'q': true, 'i': true}
	if action == ActionTransmit || action == ActionTransmitAndPlace || action == ActionQuery {
		for _, key := range []byte{'f', 't', 's', 'v', 'o', 'm'} {
			allowed[key] = true
		}
	}
	if action == ActionTransmitAndPlace || action == ActionPlace {
		for _, key := range []byte{'p', 'x', 'y', 'w', 'h', 'c', 'r', 'z', 'C'} {
			allowed[key] = true
		}
	}
	if action == ActionDelete {
		allowed['d'] = true
	}
	return allowed
}

func parsePlacement(fields map[byte]string) (*PlacementRequest, ReplyCode) {
	id, ok := parseUnsigned(fields, 'p', 1, uint64(^uint32(0)), 0)
	if !ok || id == 0 {
		return nil, ReplyInvalid
	}
	cols, cok := parseUnsigned(fields, 'c', 1, uint64(termimage.HardPlacementSpan), 1)
	rows, rok := parseUnsigned(fields, 'r', 1, uint64(termimage.HardPlacementSpan), 1)
	if !cok || !rok || has(fields, 'c') != has(fields, 'r') {
		return nil, ReplyInvalid
	}
	x, xok := parseUnsigned(fields, 'x', 0, uint64(^uint32(0)), 0)
	y, yok := parseUnsigned(fields, 'y', 0, uint64(^uint32(0)), 0)
	w, wok := parseUnsigned(fields, 'w', 1, uint64(^uint32(0)), 0)
	h, hok := parseUnsigned(fields, 'h', 1, uint64(^uint32(0)), 0)
	if !xok || !yok || !wok || !hok || has(fields, 'w') != has(fields, 'h') || ((has(fields, 'x') || has(fields, 'y')) && !has(fields, 'w')) {
		return nil, ReplyInvalid
	}
	z := int64(0)
	if value, exists := fields['z']; exists {
		parsed, err := strconv.ParseInt(value, 10, 16)
		if err != nil {
			return nil, ReplyInvalid
		}
		z = parsed
	}
	cursor, ok := parseUnsigned(fields, 'C', 0, 1, 0)
	if !ok {
		return nil, ReplyInvalid
	}
	request := &PlacementRequest{ID: termimage.PlacementID(id), Cols: uint16(cols), Rows: uint16(rows), Z: int16(z), MoveCursor: cursor != 1}
	if has(fields, 'w') {
		request.Crop = &termimage.PixelRect{X: uint32(x), Y: uint32(y), Width: uint32(w), Height: uint32(h)}
	}
	return request, ReplyNone
}

func parseDelete(fields map[byte]string, image termimage.ImageID) (*termimage.DeleteSelector, ReplyCode) {
	mode := fields['d']
	if mode == "" {
		mode = "a"
	}
	if image != 0 && mode != "i" && mode != "I" {
		return nil, ReplyInvalid
	}
	selector := &termimage.DeleteSelector{CurrentScreen: true}
	switch mode {
	case "a":
		selector.All = true
	case "A":
		selector.All = true
		selector.DeleteResource = true
	case "i":
		selector.Image = &image
	case "I":
		selector.Image = &image
		selector.DeleteResource = true
	case "c":
		selector.UnderCursor = true
	case "C":
		selector.UnderCursor = true
		selector.DeleteResource = true
	default:
		return nil, ReplyUnsupported
	}
	return selector, ReplyNone
}

func parseUnsigned(fields map[byte]string, key byte, min, max, def uint64) (uint64, bool) {
	value, exists := fields[key]
	if !exists {
		return def, true
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	return parsed, err == nil && parsed >= min && parsed <= max
}
func has(fields map[byte]string, key byte) bool { _, ok := fields[key]; return ok }
