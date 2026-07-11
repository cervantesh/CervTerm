package script

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cervterm/internal/config"
)

type fakeHost struct {
	writes  []string
	notices []string
}

func (h *fakeHost) WriteInput(data string) { h.writes = append(h.writes, data) }
func (h *fakeHost) Notify(message string)  { h.notices = append(h.notices, message) }

func TestLoadAndDispatch(t *testing.T) {
	path := writeScriptConfig(t, `local cervterm = require("cervterm")
return {
  window = { width = 1200, height = 800 },
  font = { family = "Go Mono", size = 16 },
  keys = {
    {
      key = "p",
      mods = "ctrl+shift",
      action = function(term)
        term:write("echo hola desde lua\r")
        term:notify("saludo enviado")
      end,
    },
  },
}`)
	cfg, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	if cfg.Window.Width != 1200 || cfg.Font.Size != 16 {
		t.Fatalf("config overrides missing: %#v", cfg)
	}
	bindings := rt.Bindings()
	if len(bindings) != 1 || bindings[0].String() != "ctrl+shift+p" {
		t.Fatalf("bindings = %#v", bindings)
	}
	host := &fakeHost{}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if got := strings.Join(host.writes, ""); got != "echo hola desde lua\r" {
		t.Fatalf("writes = %q", got)
	}
	if got := strings.Join(host.notices, ""); got != "saludo enviado" {
		t.Fatalf("notices = %q", got)
	}
}

func TestDispatchErrorDoesNotPoisonRuntime(t *testing.T) {
	path := writeScriptConfig(t, `return {
  keys = {
    { key = "e", action = function(term) error("boom") end },
    { key = "h", action = function(term) term:write("ok") end },
  },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	if err := rt.Dispatch(0, &fakeHost{}); err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "keys e") {
		t.Fatalf("Dispatch error = %v", err)
	}
	host := &fakeHost{}
	if err := rt.Dispatch(1, host); err != nil {
		t.Fatalf("subsequent Dispatch failed: %v", err)
	}
	if got := strings.Join(host.writes, ""); got != "ok" {
		t.Fatalf("writes = %q", got)
	}
}

func TestDispatchTimeoutDoesNotPoisonRuntime(t *testing.T) {
	path := writeScriptConfig(t, `return {
  keys = {
    { key = "t", action = function(term) while true do end end },
    { key = "h", action = function(term) term:write("ok") end },
  },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	rt.dispatchTimeout = 100 * time.Millisecond
	start := time.Now()
	if err := rt.Dispatch(0, &fakeHost{}); err == nil {
		t.Fatalf("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}
	host := &fakeHost{}
	if err := rt.Dispatch(1, host); err != nil {
		t.Fatalf("subsequent Dispatch failed: %v", err)
	}
	if got := strings.Join(host.writes, ""); got != "ok" {
		t.Fatalf("writes = %q", got)
	}
}

func TestLoadValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{name: "bad action", body: `return { keys = { { key = "p", action = 42 } } }`, want: []string{"keys[1]", "action"}},
		{name: "non table", body: `return "bad"`, want: []string{"config must return a table", "string"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, rt, err := Load(writeScriptConfig(t, tt.body), config.Defaults())
			if rt != nil {
				rt.Close()
			}
			if err == nil {
				t.Fatalf("expected error")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func writeScriptConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
