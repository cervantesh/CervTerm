package vt

import (
	"strings"
	"testing"

	"cervterm/internal/core"
)

func TestParserNotificationMetadataOSC9And777(t *testing.T) {
	term := core.NewTerminal(80, 24)
	var parser Parser
	parser.Advance(term, []byte("\x1b]9;build complete\x07"))
	parser.Advance(term, []byte("\x1b]777;notify;CervTerm;tests; passed\x1b\\"))
	requests, latest, truncated := term.NotificationRequestsSince(0, nil)
	if latest != 2 || truncated || len(requests) != 2 {
		t.Fatalf("latest=%d truncated=%t requests=%#v", latest, truncated, requests)
	}
	if requests[0].Title != "" || requests[0].Body != "build complete" || requests[1].Title != "CervTerm" || requests[1].Body != "tests; passed" {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestParserNotificationRejectsMalformedAtomically(t *testing.T) {
	term := core.NewTerminal(80, 24)
	var parser Parser
	for _, payload := range []string{
		"\x1b]9;\x07",
		"\x1b]9;bad\nbody\x07",
		"\x1b]777;notify;title\x07",
		"\x1b]777;open;title;body\x07",
		"\x1b]777;notify;bad\x7f;body\x07",
		"\x1b]777;notify;title;" + strings.Repeat("x", core.MaxNotificationBodyBytes+1) + "\x07",
	} {
		parser.Advance(term, []byte(payload))
	}
	requests, latest, truncated := term.NotificationRequestsSince(0, nil)
	if latest != 0 || truncated || len(requests) != 0 {
		t.Fatalf("malformed payload mutated requests: latest=%d truncated=%t requests=%#v", latest, truncated, requests)
	}
}
