package kitty

import "cervterm/internal/termimage"

type ReplyPlan struct {
	action Action
	quiet  Quiet
}

func (r ReplyPlan) NeedsReservation() bool { return r.quiet != QuietAll }
func (r ReplyPlan) Encode(code ReplyCode) []byte {
	if code == ReplyNone || r.quiet == QuietAll || (code == ReplyOK && r.quiet == QuietErrorsOnly) {
		return nil
	}
	text := map[ReplyCode]string{ReplyOK: "OK", ReplyInvalid: "EINVAL", ReplyUnsupported: "ENOTSUP", ReplyLimit: "ENOSPC", ReplyTimeout: "ETIME", ReplyCancelled: "ECANCELED", ReplyNotFound: "ENOENT", ReplyFailed: "EIO"}[code]
	if text == "" {
		return nil
	}
	action := byte(r.action)
	if action == 0 {
		action = 't'
	}
	result := []byte{'\x1b', '_', 'G', 'a', '=', action, ';'}
	result = append(result, text...)
	result = append(result, '\x1b', '\\')
	if uint64(len(result)) > termimage.HardReplyBytes {
		panic("kitty fixed reply exceeds hard bound")
	}
	return result
}
