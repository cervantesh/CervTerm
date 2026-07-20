//go:build glfw

package glfwgl

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"cervterm/internal/config"
	"cervterm/internal/core"
	termmux "cervterm/internal/mux"
	"cervterm/internal/notificationpolicy"
)

type fakeNotificationSink struct {
	requests []core.NotificationRequest
	err      error
}

func (sink *fakeNotificationSink) Notify(title, body string) error {
	sink.requests = append(sink.requests, core.NotificationRequest{Title: title, Body: body})
	return sink.err
}

func TestNotificationPolicyRequiresConsentFreshnessFocusAndRate(t *testing.T) {
	now := time.Unix(100, 0)
	sink := &fakeNotificationSink{}
	app := &App{cfg: config.Defaults(), notificationState: notificationState{gate: notificationpolicy.NewGate(func() time.Time { return now }), sink: sink}}
	request := core.NotificationRequest{Sequence: 1, Title: "Build", Body: "complete"}
	app.applyNotificationEffectWithFocus(request, true, false)
	app.cfg.Notification = config.NotificationConfig{Enabled: true, Focus: "unfocused", RateLimitMS: 1000}
	app.applyNotificationEffectWithFocus(request, false, false)
	app.applyNotificationEffectWithFocus(request, true, true)
	if len(sink.requests) != 0 {
		t.Fatalf("denied requests reached sink: %#v", sink.requests)
	}
	app.applyNotificationEffectWithFocus(request, true, false)
	app.applyNotificationEffectWithFocus(request, true, false)
	if len(sink.requests) != 1 {
		t.Fatalf("rate limit calls = %d, want 1", len(sink.requests))
	}
	now = now.Add(time.Second)
	app.applyNotificationEffectWithFocus(request, true, false)
	if len(sink.requests) != 2 || sink.requests[1].Title != "Build" || sink.requests[1].Body != "complete" {
		t.Fatalf("allowed requests = %#v", sink.requests)
	}
}

func TestNotificationAdapterErrorsAreRedacted(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previous)
	sink := &fakeNotificationSink{err: errors.New("secret-title secret-body")}
	app := &App{cfg: config.Defaults(), notificationState: notificationState{sink: sink}}
	app.cfg.Notification = config.NotificationConfig{Enabled: true, Focus: "always", RateLimitMS: 100}
	app.applyNotificationEffectWithFocus(core.NotificationRequest{Title: "secret-title", Body: "secret-body"}, true, true)
	app.notificationState.gate.Reset()
	app.applyNotificationEffectWithFocus(core.NotificationRequest{Title: "secret-title", Body: "secret-body"}, true, true)
	if got := logs.String(); strings.Contains(got, "secret") || strings.Count(got, "native notification unavailable") != 1 {
		t.Fatalf("logs were not redacted/coalesced: %q", got)
	}
}

func TestPendingNotificationLosesFreshness(t *testing.T) {
	controller := newWindowController(processServices{}, fakeNativePump{log: &[]string{}})
	controller.queuePending(2, termmux.Event{Kind: termmux.PaneNotificationRequested, Window: 2, Fresh: true, Notification: core.NotificationRequest{Body: "body"}})
	pending := controller.pending[2]
	if len(pending) != 1 || pending[0].Fresh {
		t.Fatalf("pending notification retained freshness: %#v", pending)
	}
}
