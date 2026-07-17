package script

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cervterm/internal/config"

	lua "github.com/yuin/gopher-lua"
)

func TestLoadVersionedPreservesAuthoredV1Path(t *testing.T) {
	dir := t.TempDir()
	path := writeSourceGraphScript(t, dir, "config.lua", `local c=require("cervterm"); return {font={family="Legacy"},keys={{key="k",action=c.action.ScrollPage(1)}}}`)
	loaded, err := LoadVersioned(path, config.Defaults(), CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AuthoredVersion != 1 || loaded.Runtime == nil || loaded.Candidate != nil {
		t.Fatalf("v1 ownership = %#v", loaded)
	}
	defer loaded.Runtime.Close()
	if loaded.Config.Font.Family != "Legacy" || len(loaded.Runtime.Bindings()) != 1 {
		t.Fatalf("v1 config/runtime = %#v %#v", loaded.Config.Font, loaded.Runtime.Bindings())
	}
}

func TestLoadVersionedBuildsExplicitV2Candidate(t *testing.T) {
	dir := t.TempDir()
	writeSourceGraphScript(t, dir, "base.lua", `return {font={family="Base"}}`)
	path := writeSourceGraphScript(t, dir, "config.lua", `return {config_version=2,includes={"base.lua"},font={size=17}}`)
	loaded, err := LoadVersioned(path, config.Defaults(), CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AuthoredVersion != 2 || loaded.Runtime != nil || loaded.Candidate == nil {
		t.Fatalf("v2 ownership = %#v", loaded)
	}
	defer loaded.Candidate.Close()
	if loaded.Config.Font.Family != "Base" || loaded.Config.Font.Size != 17 {
		t.Fatalf("v2 config = %#v", loaded.Config.Font)
	}
	if loaded.Candidate.graph == nil || len(loaded.Candidate.graph.Sources) != 2 {
		t.Fatal("v2 candidate lost source graph")
	}
}

func TestLoadVersionedRejectsV1CompositionMetadata(t *testing.T) {
	dir := t.TempDir()
	path := writeSourceGraphScript(t, dir, "config.lua", `return {includes={"base.lua"}}`)
	loaded, err := LoadVersioned(path, config.Defaults(), CandidateOptions{})
	if loaded.Runtime != nil {
		loaded.Runtime.Close()
	}
	if loaded.Candidate != nil {
		loaded.Candidate.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "requires config_version = 2") {
		t.Fatalf("v1 composition error = %v", err)
	}
}

func TestLoadVersionedEvaluatesExplicitV2ExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "count.txt")
	body := fmt.Sprintf(`local path=%q
	local f=io.open(path,"r")
	local n=0
	if f then n=tonumber(f:read("*a")) or 0; f:close() end
	f=assert(io.open(path,"w")); f:write(tostring(n+1)); f:close()
	return {config_version=2}`, counter)
	path := writeSourceGraphScript(t, dir, "config.lua", body)
	loaded, err := LoadVersioned(path, config.Defaults(), CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Candidate.Close()
	data, err := os.ReadFile(counter)
	if err != nil || string(data) != "1" {
		t.Fatalf("evaluation count=%q err=%v", data, err)
	}
}

func TestLoadVersionedV1KeepsLastReturnAndGlobalReplacement(t *testing.T) {
	dir := t.TempDir()
	path := writeSourceGraphScript(t, dir, "config.lua", `
		local original=require
		require=function(name)
			if name=="deferred" then return {value="kept"} end
			return original(name)
		end
		return {font={family="first"}}, {font={family="last"}}
	`)
	loaded, err := LoadVersioned(path, config.Defaults(), CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Runtime.Close()
	if loaded.Config.Font.Family != "last" {
		t.Fatalf("v1 selected return = %q", loaded.Config.Font.Family)
	}
	fn, ok := loaded.Runtime.state.GetGlobal("require").(*lua.LFunction)
	if !ok {
		t.Fatal("v1 require replacement was not retained")
	}
	if err := loaded.Runtime.state.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, lua.LString("deferred")); err != nil {
		t.Fatal(err)
	}
	value := loaded.Runtime.state.Get(-1)
	loaded.Runtime.state.Pop(1)
	table, ok := value.(*lua.LTable)
	if !ok || lua.LVAsString(table.RawGetString("value")) != "kept" {
		t.Fatalf("deferred require result = %v", value)
	}
}

func TestLoadVersionedV1TealPublishesWithoutOwnershipMarker(t *testing.T) {
	if _, err := exec.LookPath("tl"); err != nil {
		t.Skip("tl not installed")
	}
	dir := t.TempDir()
	copyFile(t, filepath.Join("..", "..", "docs", "examples", "cervterm.d.tl"), filepath.Join(dir, "cervterm.d.tl"))
	path := filepath.Join(dir, "config.tl")
	v2 := `local c=require("cervterm")
local cfg: c.Config = {config_version=2,font={family="V2 Teal"}}
return cfg
`
	if err := os.WriteFile(path, []byte(v2), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := LoadVersioned(path, config.Defaults(), CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Candidate.PublishTeal(config.TealPublicationOptions{}); err != nil {
		first.Candidate.Close()
		t.Fatal(err)
	}
	first.Candidate.Close()
	published := filepath.Join(dir, "config.lua")
	marker := config.TealOwnershipMarkerPath(published)
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("v2 ownership marker missing: %v", err)
	}

	v1 := `local c=require("cervterm")
local cfg: c.Config = {font={family="Legacy Teal"}}
return cfg
`
	if err := os.WriteFile(path, []byte(v1), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadVersioned(path, config.Defaults(), CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Runtime.Close()
	if loaded.LegacyTransition == nil {
		t.Fatal("v2-to-v1 transition did not retain rollback journal")
	}
	loaded.LegacyTransition.Commit()
	if _, err := os.Stat(published); err != nil {
		t.Fatalf("legacy generated Lua missing: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("v2-to-v1 transition retained ownership marker: %v", err)
	}
}
