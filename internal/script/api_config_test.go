package script

import (
	"testing"

	"cervterm/internal/config"
)

type runtimeConfigFakeHost struct {
	fakeHost
	cfg             config.Config
	reloadRequested bool
}

func (h *runtimeConfigFakeHost) RuntimeConfig() config.Config { return h.cfg }
func (h *runtimeConfigFakeHost) ApplyRuntimeConfig(cfg config.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	h.cfg = cfg
	return nil
}
func (h *runtimeConfigFakeHost) RequestConfigReload() bool {
	h.reloadRequested = true
	return true
}

func TestRuntimeConfigAPI(t *testing.T) {
	path := writeScriptConfig(t, `return { keys = {{
	key = "a",
	action = function(term)
		term:set_background("#010203FF")
		term:set_window_opacity(0.75)
		term:set_text_opacity(0.6)
		if term:text_opacity() ~= 0.6 then error("text opacity not synchronous") end
		term:set_blur(false)
		term:set_scrolling({ history = 77, wheel_multiplier = 5, hide_cursor_when_scrolled = false })
		term:set_scrollbar({ enabled = false, track_click = "jump", fade_ms = 0 })
		term:reload_config()
	end,
}} }`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer rt.Close()
	host := &runtimeConfigFakeHost{cfg: config.Defaults()}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if host.cfg.Colors.Background != "#010203FF" || host.cfg.Window.Opacity != .75 || host.cfg.Window.TextOpacity != .6 || host.cfg.Window.Blur {
		t.Fatalf("appearance API did not apply: %#v %#v", host.cfg.Window, host.cfg.Colors)
	}
	if host.cfg.Scrolling.History != 77 || host.cfg.Scrolling.WheelMultiplier != 5 || host.cfg.Scrolling.HideCursorWhenScrolled {
		t.Fatalf("scrolling API did not apply: %#v", host.cfg.Scrolling)
	}
	if host.cfg.Scrollbar.Enabled || host.cfg.Scrollbar.TrackClick != "jump" || host.cfg.Scrollbar.FadeMS != 0 {
		t.Fatalf("scrollbar API did not apply: %#v", host.cfg.Scrollbar)
	}
	if !host.reloadRequested {
		t.Fatal("reload_config did not request reload")
	}
}

func TestRuntimeConfigAPIRejectsInvalidMutation(t *testing.T) {
	path := writeScriptConfig(t, `return { keys = {{ key = "a", action = function(term)
		term:set_window_opacity(0.5)
	end }} }`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer rt.Close()
	host := &runtimeConfigFakeHost{cfg: config.Defaults()}
	if err := rt.Dispatch(0, host); err == nil {
		t.Fatal("expected mutually-exclusive opacity mutation to fail")
	}
	if host.cfg.Window.Opacity != 1 {
		t.Fatalf("invalid mutation changed host config: %#v", host.cfg.Window)
	}
}

func TestRuntimeConfigAPIBackgroundOpacity(t *testing.T) {
	path := writeScriptConfig(t, `return { keys = {{ key = "a", action = function(term)
		term:set_background_opacity(0.5)
		if term:background_opacity() ~= 0.5 then error("background opacity not synchronous") end
	end }} }`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	host := &runtimeConfigFakeHost{cfg: config.Defaults()}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatal(err)
	}
	if host.cfg.Window.BackgroundOpacity != 0.5 {
		t.Fatalf("background opacity = %v", host.cfg.Window.BackgroundOpacity)
	}
}
