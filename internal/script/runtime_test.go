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
	cols    int
	rows    int
	curRow  int
	curCol  int
	title   string
	lines   map[int]string
}

func (h *fakeHost) WriteInput(data string) { h.writes = append(h.writes, data) }
func (h *fakeHost) Notify(message string)  { h.notices = append(h.notices, message) }
func (h *fakeHost) Size() (int, int)       { return h.cols, h.rows }
func (h *fakeHost) Cursor() (int, int)     { return h.curRow, h.curCol }
func (h *fakeHost) Title() string          { return h.title }
func (h *fakeHost) Line(row int) (string, bool) {
	text, ok := h.lines[row]
	return text, ok
}

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

func TestFireEvents(t *testing.T) {
	path := writeScriptConfig(t, `return {
  events = {
    output = function(term, data) term:notify("out:" .. data) end,
    title = function(term, title) term:notify("title:" .. title) end,
    bell = function(term) term:notify("bell") end,
  },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	if !rt.WantsOutput() {
		t.Fatal("WantsOutput should be true when an output handler is set")
	}
	host := &fakeHost{}
	if err := rt.FireOutput(host, "ls\r"); err != nil {
		t.Fatalf("FireOutput: %v", err)
	}
	if err := rt.FireTitle(host, "shell"); err != nil {
		t.Fatalf("FireTitle: %v", err)
	}
	if err := rt.FireBell(host); err != nil {
		t.Fatalf("FireBell: %v", err)
	}
	got := strings.Join(host.notices, "|")
	if got != "out:ls\r|title:shell|bell" {
		t.Fatalf("notices = %q", got)
	}
}

func TestFireWithoutHandlersIsNoop(t *testing.T) {
	path := writeScriptConfig(t, `return { font = { size = 12 } }`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	if rt.WantsOutput() {
		t.Fatal("WantsOutput should be false without an output handler")
	}
	host := &fakeHost{}
	for _, err := range []error{rt.FireOutput(host, "x"), rt.FireTitle(host, "x"), rt.FireBell(host)} {
		if err != nil {
			t.Fatalf("no-op fire returned error: %v", err)
		}
	}
	if len(host.notices) != 0 || len(host.writes) != 0 {
		t.Fatalf("no-op fire had side effects: %#v", host)
	}
}

func TestEventErrorDoesNotPoisonRuntime(t *testing.T) {
	path := writeScriptConfig(t, `return {
  events = { output = function(term, data) error("boom") end },
  keys = { { key = "h", action = function(term) term:write("ok") end } },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	if err := rt.FireOutput(&fakeHost{}, "x"); err == nil || !strings.Contains(err.Error(), "events.output") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("FireOutput error = %v", err)
	}
	host := &fakeHost{}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("subsequent Dispatch failed: %v", err)
	}
	if strings.Join(host.writes, "") != "ok" {
		t.Fatalf("writes = %q", host.writes)
	}
}

func TestEventValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{name: "bad handler", body: `return { events = { bell = 7 } }`, want: []string{"events.bell", "function"}},
		{name: "events not table", body: `return { events = "x" }`, want: []string{"events must be a table"}},
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

func TestEventTimeoutDoesNotPoisonRuntime(t *testing.T) {
	path := writeScriptConfig(t, `return {
  events = { output = function(term, data) while true do end end },
  keys = { { key = "h", action = function(term) term:write("ok") end } },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	rt.dispatchTimeout = 100 * time.Millisecond
	if err := rt.FireOutput(&fakeHost{}, "x"); err == nil {
		t.Fatal("expected timeout error")
	}
	host := &fakeHost{}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("subsequent Dispatch failed: %v", err)
	}
	if strings.Join(host.writes, "") != "ok" {
		t.Fatalf("writes = %q", host.writes)
	}
}

func TestTermReadState(t *testing.T) {
	path := writeScriptConfig(t, `return {
  keys = {
    { key = "r", action = function(term)
        local cols, rows = term:size()
        local crow, ccol = term:cursor()
        term:notify(string.format("%dx%d @%d,%d t=%s l=[%s] oob=[%s]",
          cols, rows, crow, ccol, term:title(), term:line(1), term:line(99)))
      end },
  },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	host := &fakeHost{cols: 80, rows: 24, curRow: 2, curCol: 5, title: "sh", lines: map[int]string{0: "hello"}}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	// cursor is reported 1-based (3,6); term:line(1) maps to host row 0; out-of-range is "".
	want := "80x24 @3,6 t=sh l=[hello] oob=[]"
	if got := strings.Join(host.notices, ""); got != want {
		t.Fatalf("notice = %q, want %q", got, want)
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
