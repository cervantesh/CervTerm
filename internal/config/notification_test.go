package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNotificationConfigStrictLiveAndComposed(t *testing.T) {
	cfg, err := LoadLua(writeLuaDocument(t, `return {config_version=2,notification={enabled=true,focus="always",rate_limit_ms=750}}`), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Notification != (NotificationConfig{Enabled: true, Focus: "always", RateLimitMS: 750}) {
		t.Fatalf("notification = %#v", cfg.Notification)
	}
	if merged := MergeLiveConfig(Defaults(), cfg); merged.Notification != cfg.Notification {
		t.Fatalf("live notification = %#v", merged.Notification)
	}

	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"base.lua"},notification={focus="always"}}`)
	writeGraphLua(t, dir, "base.lua", `return {config_version=2,notification={enabled=true,rate_limit_ms=900}}`)
	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	composed := FromDocument(Defaults(), composition.Document)
	if composed.Notification != (NotificationConfig{Enabled: true, Focus: "always", RateLimitMS: 900}) {
		t.Fatalf("composed notification = %#v", composed.Notification)
	}
	if _, ok := composition.Provenance.Lookup("notification.enabled"); !ok {
		t.Fatal("missing notification provenance")
	}
}

func TestNotificationConfigRejectsInvalidDocuments(t *testing.T) {
	for _, test := range []struct{ body, want string }{
		{`return {config_version=1,notification={enabled=true}}`, "requires config_version = 2"},
		{`return {config_version=2,notification=true}`, "notification: must be table"},
		{`return {config_version=2,notification={enabled="yes"}}`, "notification.enabled: must be boolean"},
		{`return {config_version=2,notification={focus="sometimes"}}`, "notification.focus"},
		{`return {config_version=2,notification={rate_limit_ms=99}}`, "notification.rate_limit_ms"},
		{`return {config_version=2,notification={unknown=true}}`, "notification.unknown: unknown field"},
	} {
		cfg, err := LoadLua(writeLuaDocument(t, test.body), Defaults())
		if err == nil {
			err = cfg.Validate()
		}
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("%s: err=%v, want %q", test.body, err, test.want)
		}
	}
}
