package core

import (
	"fmt"
	"strings"
	"testing"
)

func TestNotificationStoreIsBoundedMonotonicAndDetached(t *testing.T) {
	term := NewTerminal(80, 24)
	for i := 1; i <= MaxNotificationRequests+3; i++ {
		if !term.RequestNotification("title", fmt.Sprintf("body-%d", i)) {
			t.Fatalf("request %d rejected", i)
		}
	}
	requests, latest, truncated := term.NotificationRequestsSince(0, nil)
	if !truncated || latest != MaxNotificationRequests+3 || len(requests) != MaxNotificationRequests {
		t.Fatalf("latest=%d truncated=%t len=%d", latest, truncated, len(requests))
	}
	if requests[0].Sequence != 4 || requests[0].Body != "body-4" || requests[len(requests)-1].Sequence != latest {
		t.Fatalf("retained suffix = %#v .. %#v", requests[0], requests[len(requests)-1])
	}
	requests[0].Body = "mutated"
	again, _, _ := term.NotificationRequestsSince(3, requests[:0])
	if again[0].Body != "body-4" {
		t.Fatal("returned request aliased store")
	}
}

func TestNotificationStoreRejectsUnsafePayloadAtomically(t *testing.T) {
	term := NewTerminal(80, 24)
	for _, request := range []NotificationRequest{
		{Body: ""},
		{Title: "bad\n", Body: "body"},
		{Body: "bad\x7f"},
		{Title: strings.Repeat("t", MaxNotificationTitleBytes+1), Body: "body"},
		{Body: strings.Repeat("b", MaxNotificationBodyBytes+1)},
		{Body: string([]byte{0xff})},
	} {
		if term.RequestNotification(request.Title, request.Body) {
			t.Fatalf("unsafe request accepted: %#v", request)
		}
	}
	requests, latest, truncated := term.NotificationRequestsSince(0, nil)
	if latest != 0 || truncated || len(requests) != 0 {
		t.Fatalf("rejected requests mutated store: latest=%d truncated=%t requests=%v", latest, truncated, requests)
	}
}
