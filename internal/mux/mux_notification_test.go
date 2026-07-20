package mux

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"cervterm/internal/core"
)

func notificationTestMux(t *testing.T) (*Mux, PaneID) {
	t.Helper()
	mux := New(&fakeFactory{err: errors.New("fallback")}, Options{})
	_, pane, _, err := mux.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err == nil {
		t.Fatal("bootstrap should retain a failed fallback pane")
	}
	t.Cleanup(func() { _ = mux.Shutdown() })
	return mux, pane
}

func TestMuxEmitsDetachedNotificationRequest(t *testing.T) {
	mux, pane := notificationTestMux(t)
	events, err := mux.FeedFallback(pane, []byte("\x1b]777;notify;Build;complete\x07"))
	if err != nil {
		t.Fatal(err)
	}
	var got *Event
	for i := range events {
		if events[i].Kind == PaneNotificationRequested {
			got = &events[i]
		}
	}
	if got == nil || got.Pane != pane || got.Window == 0 || got.Revision != 1 || got.Notification != (core.NotificationRequest{Sequence: 1, Title: "Build", Body: "complete"}) {
		t.Fatalf("notification event = %#v", got)
	}
}

func TestMuxReportsBoundedNotificationOverflow(t *testing.T) {
	mux, pane := notificationTestMux(t)
	var payload strings.Builder
	for i := 1; i <= core.MaxNotificationRequests+3; i++ {
		fmt.Fprintf(&payload, "\x1b]9;n-%d\x07", i)
	}
	events, err := mux.FeedFallback(pane, []byte(payload.String()))
	if err != nil {
		t.Fatal(err)
	}
	requests, overflow := 0, 0
	var first, last core.NotificationRequest
	for _, event := range events {
		switch event.Kind {
		case PaneNotificationOverflow:
			overflow++
			if event.Revision != core.MaxNotificationRequests+3 {
				t.Fatalf("overflow revision = %d", event.Revision)
			}
		case PaneNotificationRequested:
			if requests == 0 {
				first = event.Notification
			}
			last = event.Notification
			requests++
		}
	}
	if overflow != 1 || requests != core.MaxNotificationRequests || first.Sequence != 4 || last.Sequence != core.MaxNotificationRequests+3 {
		t.Fatalf("overflow=%d requests=%d first=%#v last=%#v", overflow, requests, first, last)
	}
}
