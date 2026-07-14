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
	writes       []string
	notices      []string
	selection    string
	clipboard    string
	scrollback   int
	scrollOffset int
	cols         int
	rows         int
	curRow       int
	curCol       int
	title        string
	cwd          string
	lines        map[int]string
	wrapped      map[int]bool
	fontSize     float64
	fontSizes    []float64
	searches     []string
	searchResult bool
}

func (h *fakeHost) WriteInput(data string) { h.writes = append(h.writes, data) }
func (h *fakeHost) Notify(message string)  { h.notices = append(h.notices, message) }
func (h *fakeHost) Selection() string      { return h.selection }
func (h *fakeHost) SetClipboard(text string) {
	h.clipboard = text
}
func (h *fakeHost) Clipboard() string { return h.clipboard }
func (h *fakeHost) Scroll(lines int) bool {
	previous := h.scrollOffset
	h.scrollOffset = max(0, min(h.scrollOffset+lines, h.scrollback))
	return h.scrollOffset != previous
}
func (h *fakeHost) ScrollToBottom() { h.scrollOffset = 0 }
func (h *fakeHost) ScrollbackLen() int {
	return h.scrollback
}
func (h *fakeHost) Size() (int, int)      { return h.cols, h.rows }
func (h *fakeHost) Cursor() (int, int)    { return h.curRow, h.curCol }
func (h *fakeHost) Title() string         { return h.title }
func (h *fakeHost) Cwd() string           { return h.cwd }
func (h *fakeHost) SetTitle(title string) { h.title = title }
func (h *fakeHost) Line(row int) (string, bool) {
	text, ok := h.lines[row]
	return text, ok
}
func (h *fakeHost) LineWrapped(row int) (bool, bool) {
	if row < 0 || row >= h.rows {
		return false, false
	}
	return h.wrapped[row], true
}
func (h *fakeHost) Search(query string) bool {
	h.searches = append(h.searches, query)
	return h.searchResult
}
func (h *fakeHost) FontSize() float64 { return h.fontSize }
func (h *fakeHost) SetFontSize(pts float64) {
	h.fontSize = pts
	h.fontSizes = append(h.fontSizes, pts)
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
    cwd = function(term, dir) term:notify("cwd:" .. dir) end,
    bell = function(term) term:notify("bell") end,
    resize = function(term, cols, rows) term:notify(string.format("resize:%dx%d", cols, rows)) end,
    focus = function(term, focused) term:notify("focus:" .. tostring(focused)) end,
    scroll = function(term, offset) term:notify(string.format("scroll:%d", offset)) end,
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
	if err := rt.FireCwd(host, "/work/demo"); err != nil {
		t.Fatalf("FireCwd: %v", err)
	}
	if err := rt.FireBell(host); err != nil {
		t.Fatalf("FireBell: %v", err)
	}
	if err := rt.FireResize(host, 80, 24); err != nil {
		t.Fatalf("FireResize: %v", err)
	}
	if err := rt.FireFocus(host, true); err != nil {
		t.Fatalf("FireFocus: %v", err)
	}
	if err := rt.FireScroll(host, 12); err != nil {
		t.Fatalf("FireScroll: %v", err)
	}
	got := strings.Join(host.notices, "|")
	if got != "out:ls\r|title:shell|cwd:/work/demo|bell|resize:80x24|focus:true|scroll:12" {
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
	for _, err := range []error{
		rt.FireOutput(host, "x"), rt.FireTitle(host, "x"), rt.FireCwd(host, "x"), rt.FireBell(host),
		rt.FireResize(host, 1, 1), rt.FireFocus(host, true), rt.FireScroll(host, 0),
	} {
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

func TestTermSelectionEmptyAndNonempty(t *testing.T) {
	path := writeScriptConfig(t, `return {
  keys = {
    { key = "s", action = function(term) term:notify("[" .. term:selection() .. "]") end },
  },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	host := &fakeHost{}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("empty selection Dispatch failed: %v", err)
	}
	host.selection = "selected text"
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("nonempty selection Dispatch failed: %v", err)
	}
	if got := strings.Join(host.notices, "|"); got != "[]|[selected text]" {
		t.Fatalf("selection notices = %q", got)
	}
}

func TestTermClipboardRoundtrip(t *testing.T) {
	path := writeScriptConfig(t, `return {
  keys = {
    { key = "c", action = function(term)
        term:copy("from lua")
        term:notify(term:clipboard())
      end },
  },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	host := &fakeHost{}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if host.clipboard != "from lua" || strings.Join(host.notices, "") != "from lua" {
		t.Fatalf("clipboard roundtrip = %q, notices = %#v", host.clipboard, host.notices)
	}
}

func TestTermScrollBindings(t *testing.T) {
	path := writeScriptConfig(t, `return {
  keys = {
    { key = "s", action = function(term)
        term:notify(string.format("%s,%s,%s:%d",
          tostring(term:scroll(3)), tostring(term:scroll(3)),
          tostring(term:scroll(-1)), term:scrollback()))
        term:scroll_to_bottom()
      end },
  },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	host := &fakeHost{scrollback: 3}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if got := strings.Join(host.notices, ""); got != "true,false,true:3" {
		t.Fatalf("scroll results = %q", got)
	}
	if host.scrollOffset != 0 {
		t.Fatalf("scroll_to_bottom offset = %d, want 0", host.scrollOffset)
	}
}

func TestTermSetTitleRoundtrip(t *testing.T) {
	path := writeScriptConfig(t, `return {
  keys = {
    { key = "t", action = function(term)
        term:set_title("lua title")
        term:notify(term:title())
      end },
  },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	host := &fakeHost{title: "old"}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if host.title != "lua title" || strings.Join(host.notices, "") != "lua title" {
		t.Fatalf("title roundtrip = %q, notices = %#v", host.title, host.notices)
	}
}

func TestTermLineWrappedUsesOneBasedRows(t *testing.T) {
	path := writeScriptConfig(t, `return {
  keys = {
    { key = "w", action = function(term)
        term:notify(string.format("%s,%s,%s",
          tostring(term:line_wrapped(1)), tostring(term:line_wrapped(2)),
          tostring(term:line_wrapped(99))))
      end },
  },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	host := &fakeHost{rows: 2, wrapped: map[int]bool{0: true}}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if got := strings.Join(host.notices, ""); got != "true,false,false" {
		t.Fatalf("line_wrapped results = %q", got)
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
