package kitty

import (
	"bytes"
	"testing"

	"cervterm/internal/termimage"
)

func TestReplyQuietMatrixBoundAndDetachment(t *testing.T) {
	for quiet := QuietNormal; quiet <= QuietAll; quiet++ {
		plan := ReplyPlan{action: ActionTransmit, quiet: quiet}
		success, failure := plan.Encode(ReplyOK), plan.Encode(ReplyInvalid)
		if quiet == QuietNormal && (len(success) == 0 || len(failure) == 0) {
			t.Fatal("normal reply suppressed")
		}
		if quiet == QuietErrorsOnly && (len(success) != 0 || len(failure) == 0) {
			t.Fatal("errors-only matrix")
		}
		if quiet == QuietAll && (len(success) != 0 || len(failure) != 0) {
			t.Fatal("quiet-all matrix")
		}
		if uint64(len(failure)) > termimage.HardReplyBytes {
			t.Fatal("reply exceeds bound")
		}
	}
	plan := ReplyPlan{action: ActionDelete}
	first := plan.Encode(ReplyOK)
	first[0] = 'x'
	second := plan.Encode(ReplyOK)
	if bytes.Equal(first, second) || second[0] != '\x1b' {
		t.Fatal("reply aliases mutable storage")
	}
}

func TestRepliesAreFiniteAndRedacted(t *testing.T) {
	plan := ReplyPlan{action: ActionQuery}
	for code := ReplyOK; code <= ReplyFailed; code++ {
		reply := plan.Encode(code)
		if len(reply) == 0 || bytes.Contains(reply, []byte("123")) {
			t.Fatalf("code=%d reply=%q", code, reply)
		}
	}
}

func TestMalformedQuietUsesMostSuppressiveValidValue(t *testing.T) {
	_, failure := parseCompleteFrame([]byte("Ga=t,q=0,q=2,i=secret;PAYLOAD"), false)
	if failure == nil || len(failure.reply.Encode(failure.code)) != 0 {
		t.Fatalf("failure=%#v", failure)
	}
	_, failure = parseCompleteFrame([]byte("Ga=t,q=bad,i=secret;PAYLOAD"), false)
	if failure == nil {
		t.Fatal("malformed quiet accepted")
	}
	reply := failure.reply.Encode(failure.code)
	if len(reply) == 0 || bytes.Contains(reply, []byte("secret")) || bytes.Contains(reply, []byte("PAYLOAD")) {
		t.Fatalf("reply=%q", reply)
	}
}
