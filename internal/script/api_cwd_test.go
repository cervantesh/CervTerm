package script

import (
	"strings"
	"testing"

	"cervterm/internal/config"
)

func TestTermCwd(t *testing.T) {
	path := writeScriptConfig(t, `return {
  keys = {
    { key = "d", action = function(term) term:notify(term:cwd()) end },
  },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	host := &fakeHost{cwd: "/work/demo"}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if got := strings.Join(host.notices, ""); got != "/work/demo" {
		t.Fatalf("cwd notice = %q", got)
	}
}
