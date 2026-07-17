package script

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cervterm/internal/config"

	lua "github.com/yuin/gopher-lua"
)

func TestDeclarativeIncludesRejectImperativeRegistrations(t *testing.T) {
	tests := []struct {
		name   string
		child  string
		module string
		want   string
	}{
		{name: "direct timer", child: `local c=require("cervterm"); c.after(10, function() end); return {config_version=2}`, want: "cervterm.after"},
		{name: "nested module status", child: `require("registration"); return {config_version=2}`, module: `local c=require("cervterm"); c.status("x", "y"); return {}`, want: "cervterm.status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeSourceGraphScript(t, dir, "primary.lua", `package.path=`+quotedLuaPath(filepath.Join(dir, "?.lua"))+`; return {config_version=2, includes={"child.lua"}}`)
			writeSourceGraphScript(t, dir, "child.lua", tt.child)
			if tt.module != "" {
				writeSourceGraphScript(t, dir, "registration.lua", tt.module)
			}
			state, _, _, _ := newGraphScriptState()
			defer state.Close()
			graph, err := config.BuildSourceGraph(state, filepath.Join(dir, "primary.lua"), config.DefaultSourceGraphOptions())
			if graph != nil {
				defer graph.Close()
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "declarative include") {
				t.Fatalf("error = %v, want %q include registration error", err, tt.want)
			}
		})
	}
}

func TestPrimaryMayRegisterAndIncludeMayBuildPureActions(t *testing.T) {
	dir := t.TempDir()
	writeSourceGraphScript(t, dir, "primary.lua", `local c=require("cervterm"); c.after(10, function() end); return {config_version=2, includes={"child.lua"}}`)
	writeSourceGraphScript(t, dir, "child.lua", `local c=require("cervterm"); local action=c.action.ScrollPage(1); return {config_version=2, keys={{key="k", action=action}}}`)
	state, timers, _, _ := newGraphScriptState()
	defer state.Close()
	graph, err := config.BuildSourceGraph(state, filepath.Join(dir, "primary.lua"), config.DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	if len(timers.entries) != 1 {
		t.Fatalf("primary timers = %#v", timers.entries)
	}
	if len(graph.Sources) != 2 {
		t.Fatalf("sources = %#v", graph.Sources)
	}
}

func TestIncludeCannotMutateOverlayHandleCreatedByPrimary(t *testing.T) {
	dir := t.TempDir()
	writeSourceGraphScript(t, dir, "primary.lua", `local c=require("cervterm"); shared_overlay=c.overlay("shared"); return {config_version=2, includes={"child.lua"}}`)
	writeSourceGraphScript(t, dir, "child.lua", `shared_overlay:hide(); return {config_version=2}`)
	state, _, _, overlays := newGraphScriptState()
	defer state.Close()
	graph, err := config.BuildSourceGraph(state, filepath.Join(dir, "primary.lua"), config.DefaultSourceGraphOptions())
	if graph != nil {
		defer graph.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "cervterm.overlay.hide") || !strings.Contains(err.Error(), "declarative include") {
		t.Fatalf("overlay mutation error = %v", err)
	}
	if overlay := overlays.get("shared"); !overlay.visible {
		t.Fatal("include hid primary overlay")
	}
}

func newGraphScriptState() (*lua.LState, *timerTable, *statusTable, *overlayStore) {
	state := lua.NewState()
	timers := &timerTable{}
	statuses := &statusTable{}
	overlays := &overlayStore{}
	state.PreloadModule("cervterm", func(state *lua.LState) int {
		state.Push(buildModule(state, timers, statuses, overlays))
		return 1
	})
	return state, timers, statuses, overlays
}

func writeSourceGraphScript(t *testing.T, dir, name, source string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func quotedLuaPath(path string) string {
	return `"` + strings.ReplaceAll(filepath.ToSlash(path), `"`, `\"`) + `"`
}
