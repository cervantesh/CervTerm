package config

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestSourceGraphTraversalDiamondAndSingleEvaluation(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.txt")
	writeGraphLua(t, dir, "primary.lua", graphSource(marker, "P", `includes = {"a.lua", "b.lua"}`))
	writeGraphLua(t, dir, "a.lua", graphSource(marker, "A", `includes = {"c.lua"}`))
	writeGraphLua(t, dir, "b.lua", graphSource(marker, "B", `includes = {"c.lua"}`))
	writeGraphLua(t, dir, "c.lua", graphSource(marker, "C", ``))
	state := lua.NewState()
	defer state.Close()
	graph, err := BuildSourceGraph(state, filepath.Join(dir, "primary.lua"), DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	if got := readGraphMarker(t, marker); got != "PACB" {
		t.Fatalf("evaluation order/count = %q, want PACB", got)
	}
	if len(graph.Sources) != 4 || len(graph.Edges) != 4 {
		t.Fatalf("sources=%d edges=%d", len(graph.Sources), len(graph.Edges))
	}
	gotOrder := make([]string, len(graph.Sources))
	for i, source := range graph.Sources {
		gotOrder[i] = filepath.Base(source.CanonicalPath)
	}
	if got := strings.Join(gotOrder, ","); got != "c.lua,a.lua,b.lua,primary.lua" {
		t.Fatalf("post-order = %s", got)
	}
}

func TestSourceGraphRejectsCyclesAndV1Includes(t *testing.T) {
	t.Run("cycle", func(t *testing.T) {
		dir := t.TempDir()
		writeGraphLua(t, dir, "a.lua", `return {config_version=2, includes={"b.lua"}}`)
		writeGraphLua(t, dir, "b.lua", `return {config_version=2, includes={"a.lua"}}`)
		state := lua.NewState()
		defer state.Close()
		_, err := BuildSourceGraph(state, filepath.Join(dir, "a.lua"), DefaultSourceGraphOptions())
		if err == nil || !strings.Contains(err.Error(), "include cycle") || !strings.Contains(err.Error(), "a.lua") || !strings.Contains(err.Error(), "b.lua") {
			t.Fatalf("cycle error = %v", err)
		}
	})
	t.Run("v1", func(t *testing.T) {
		dir := t.TempDir()
		writeGraphLua(t, dir, "primary.lua", `return {includes={"child.lua"}}`)
		writeGraphLua(t, dir, "child.lua", `return {}`)
		state := lua.NewState()
		defer state.Close()
		_, err := BuildSourceGraph(state, filepath.Join(dir, "primary.lua"), DefaultSourceGraphOptions())
		if err == nil || !strings.Contains(err.Error(), "requires config_version = 2") {
			t.Fatalf("v1 error = %v", err)
		}
	})
}

func TestSourceGraphDeduplicatesFilesystemAliases(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.txt")
	child := writeGraphLua(t, dir, "child.lua", graphSource(marker, "C", ``))
	alias := filepath.Join(dir, "alias.lua")
	if err := os.Link(child, alias); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2, includes={"child.lua", "alias.lua"}}`)
	state := lua.NewState()
	defer state.Close()
	graph, err := BuildSourceGraph(state, filepath.Join(dir, "primary.lua"), DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	if got := readGraphMarker(t, marker); got != "C" || len(graph.Sources) != 2 || len(graph.Edges) != 2 {
		t.Fatalf("marker=%q sources=%d edges=%d", got, len(graph.Sources), len(graph.Edges))
	}
}

func TestSourceGraphLimitsAndPathErrors(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, dir string) (string, SourceGraphOptions)
		want    string
	}{
		{name: "depth", want: "include depth", prepare: func(t *testing.T, dir string) (string, SourceGraphOptions) {
			writeGraphLua(t, dir, "a.lua", `return {config_version=2, includes={"b.lua"}}`)
			writeGraphLua(t, dir, "b.lua", `return {config_version=2, includes={"c.lua"}}`)
			writeGraphLua(t, dir, "c.lua", `return {config_version=2}`)
			opts := DefaultSourceGraphOptions()
			opts.MaxIncludeDepth = 1
			return filepath.Join(dir, "a.lua"), opts
		}},
		{name: "count", want: "source count", prepare: func(t *testing.T, dir string) (string, SourceGraphOptions) {
			writeGraphLua(t, dir, "a.lua", `return {config_version=2, includes={"b.lua", "c.lua"}}`)
			writeGraphLua(t, dir, "b.lua", `return {config_version=2}`)
			writeGraphLua(t, dir, "c.lua", `return {config_version=2}`)
			opts := DefaultSourceGraphOptions()
			opts.MaxDeclarativeFiles = 2
			return filepath.Join(dir, "a.lua"), opts
		}},
		{name: "source bytes", want: "size", prepare: func(t *testing.T, dir string) (string, SourceGraphOptions) {
			path := writeGraphLua(t, dir, "a.lua", `return {config_version=2}`)
			opts := DefaultSourceGraphOptions()
			opts.MaxSourceBytes = 8
			return path, opts
		}},
		{name: "aggregate bytes", want: "aggregate", prepare: func(t *testing.T, dir string) (string, SourceGraphOptions) {
			path := writeGraphLua(t, dir, "a.lua", `return {config_version=2, includes={"b.lua"}}`)
			writeGraphLua(t, dir, "b.lua", `return {config_version=2}`)
			opts := DefaultSourceGraphOptions()
			opts.MaxAggregateBytes = 50
			return path, opts
		}},
		{name: "missing", want: "declared by", prepare: func(t *testing.T, dir string) (string, SourceGraphOptions) {
			return writeGraphLua(t, dir, "a.lua", `return {config_version=2, includes={"missing.lua"}}`), DefaultSourceGraphOptions()
		}},
		{name: "remote", want: "remote config include", prepare: func(t *testing.T, dir string) (string, SourceGraphOptions) {
			return writeGraphLua(t, dir, "a.lua", `return {config_version=2, includes={"https://example.com/base.lua"}}`), DefaultSourceGraphOptions()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			primary, options := tt.prepare(t, dir)
			state := lua.NewState()
			defer state.Close()
			graph, err := BuildSourceGraph(state, primary, options)
			if graph != nil {
				defer graph.Close()
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestSourceGraphCapturesStandardLocalDependencies(t *testing.T) {
	dir := t.TempDir()
	module := writeGraphLua(t, dir, "module.lua", `return {value=1}`)
	helper := writeGraphLua(t, dir, "helper.lua", `return "helper"`)
	loaded := writeGraphLua(t, dir, "loaded.lua", `return function() end`)
	primary := writeGraphLua(t, dir, "primary.lua", `
__cervterm_record_config_dependency = function() end
package.path = `+luaQuote(filepath.Join(dir, "?.lua"))+`
require("module")
dofile(`+luaQuote(helper)+`)
assert(loadfile(`+luaQuote(loaded)+`))
return {config_version=2}
`)
	state := lua.NewState()
	defer state.Close()
	graph, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	got := make(map[string]DependencyKind)
	for _, dependency := range graph.Dependencies {
		got[dependency.Canonical] = dependency.Kind
	}
	for path, kind := range map[string]DependencyKind{module: DependencyRequire, helper: DependencyDoFile, loaded: DependencyLoadFile} {
		canonical, _, _ := canonicalLocalFile(path)
		if got[canonical] != kind {
			t.Fatalf("dependency %s = %q, all=%#v", canonical, got[canonical], graph.Dependencies)
		}
	}
}

func TestSourceGraphV2ProtectsDependencyCapture(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{name: "require", body: `require = function() end`, want: "must not replace require"},
		{name: "loaders", body: `package.loaders[1] = function() end`, want: "package.loaders[1]"},
		{name: "package", body: `local old=package; package={loaders=old.loaders, path=old.path, loaded=old.loaded, preload=old.preload}`, want: "must not replace package"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			primary := writeGraphLua(t, dir, "primary.lua", tt.body+`; return {config_version=2}`)
			state := lua.NewState()
			defer state.Close()
			originalRequire := state.GetGlobal("require")
			originalPackage := state.GetGlobal("package")
			packageTable := originalPackage.(*lua.LTable)
			loaders := packageTable.RawGetString("loaders").(*lua.LTable)
			originalLoader := loaders.RawGetInt(1)
			_, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v want %q", err, tt.want)
			}
			if state.GetGlobal("require") != originalRequire || state.GetGlobal("package") != originalPackage || packageTable.RawGetString("loaders") != loaders || loaders.RawGetInt(1) != originalLoader {
				t.Fatal("failed candidate did not restore dependency-capture loader state")
			}
		})
	}
}

func TestSourceGraphConsumesCandidateStateOnce(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `__cervterm_internal_source_graph_consumed=nil; return {config_version=2}`)
	state := lua.NewState()
	defer state.Close()
	graph, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	if _, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions()); err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("second build error = %v", err)
	}
}

func TestSourceGraphConsumesStateBeforeStagingFailure(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2}`)
	notDirectory := writeGraphLua(t, dir, "not-a-directory", "x")
	state := lua.NewState()
	defer state.Close()
	options := DefaultSourceGraphOptions()
	options.StageDirectory = notDirectory
	if _, err := BuildSourceGraph(state, primary, options); err == nil {
		t.Fatal("expected staging-directory failure")
	}
	if _, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions()); err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("second build after staging failure = %v", err)
	}
}

func TestSourceGraphStagesTealWithoutPublishingAndRejectsCollision(t *testing.T) {
	dir := t.TempDir()
	installFakeGraphTeal(t, dir)
	t.Run("stage", func(t *testing.T) {
		source := writeGraphLua(t, dir, "standalone.tl", `return {config_version=2}`)
		state := lua.NewState()
		defer state.Close()
		graph, err := BuildSourceGraph(state, source, DefaultSourceGraphOptions())
		if err != nil {
			t.Fatal(err)
		}
		if len(graph.StagedTeal) != 1 {
			t.Fatalf("staged=%#v", graph.StagedTeal)
		}
		staged := graph.StagedTeal[0]
		if _, err := os.Stat(staged.EvaluationLua); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(staged.PublishedLua); !os.IsNotExist(err) {
			t.Fatalf("published output changed: %v", err)
		}
		stageRoot := graph.stageRoot
		if err := graph.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(stageRoot); !os.IsNotExist(err) {
			t.Fatalf("stage root remains: %v", err)
		}
	})
	t.Run("configured staging owns child only", func(t *testing.T) {
		source := writeGraphLua(t, dir, "configured.tl", `return {config_version=2}`)
		configured := filepath.Join(dir, "staging-parent")
		options := DefaultSourceGraphOptions()
		options.StageDirectory = configured
		state := lua.NewState()
		defer state.Close()
		graph, err := BuildSourceGraph(state, source, options)
		if err != nil {
			t.Fatal(err)
		}
		stageRoot := graph.stageRoot
		if filepath.Dir(stageRoot) != configured {
			t.Fatalf("stage root %q is not under %q", stageRoot, configured)
		}
		if err := graph.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(stageRoot); !os.IsNotExist(err) {
			t.Fatalf("candidate staging remains: %v", err)
		}
		if info, err := os.Stat(configured); err != nil || !info.IsDir() {
			t.Fatalf("configured staging parent removed: %v", err)
		}
	})
	t.Run("collision", func(t *testing.T) {
		writeGraphLua(t, dir, "collision.lua", `return {config_version=2}`)
		source := writeGraphLua(t, dir, "collision.tl", `return {config_version=2, includes={"collision.lua"}}`)
		state := lua.NewState()
		defer state.Close()
		_, err := BuildSourceGraph(state, source, DefaultSourceGraphOptions())
		if err == nil || !strings.Contains(err.Error(), "collides with generated Teal output") {
			t.Fatalf("collision error=%v", err)
		}
	})
}

func TestSourceGraphFailureExpectationsIncludeMissingNestedSource(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2, includes={"child.lua"}}`)
	child := writeGraphLua(t, dir, "child.lua", `return {config_version=2, includes={"missing.lua"}}`)
	missing := filepath.Join(dir, "missing.lua")
	state := lua.NewState()
	defer state.Close()

	_, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
	if err == nil {
		t.Fatal("expected missing nested include failure")
	}
	var failure *SourceGraphFailureError
	if !errors.As(err, &failure) {
		t.Fatalf("error type = %T, want *SourceGraphFailureError", err)
	}
	if failure.Unwrap() == nil || failure.Error() != failure.Unwrap().Error() {
		t.Fatalf("failure did not preserve text/unwrap: %v", failure)
	}
	expectations := SourceGraphFailureExpectations(err)
	for _, path := range []string{primary, child, missing} {
		if !failureExpectationsContainPath(expectations, path) {
			t.Errorf("missing watch expectation %q in %#v", path, expectations)
		}
	}
	first := SourceGraphFailureExpectations(err)
	first[0].Path = "mutated"
	if SourceGraphFailureExpectations(err)[0].Path == "mutated" {
		t.Fatal("failure expectations were not detached")
	}
}

func TestSourceGraphFailureExpectationsIncludeMissingLuaDependencies(t *testing.T) {
	tests := []struct {
		name string
		body func(dir, missing string) string
	}{
		{name: "dofile", body: func(_ string, missing string) string {
			return `dofile(` + luaQuote(missing) + `); return {config_version=2}`
		}},
		{name: "loadfile", body: func(_ string, missing string) string {
			return `assert(loadfile(` + luaQuote(missing) + `)); return {config_version=2}`
		}},
		{name: "require", body: func(dir, _ string) string {
			return `package.path=` + luaQuote(filepath.Join(dir, "?.lua")) + `; require("missing"); return {config_version=2}`
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			missing := filepath.Join(dir, "missing.lua")
			primary := writeGraphLua(t, dir, "primary.lua", tt.body(dir, missing))
			state := lua.NewState()
			defer state.Close()
			_, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
			if err == nil {
				t.Fatal("expected dependency failure")
			}
			found := false
			for _, expectation := range SourceGraphFailureExpectations(err) {
				if expectation.Path == missing {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing dependency expectation %q in %#v", missing, SourceGraphFailureExpectations(err))
			}
		})
	}
}

func TestSourceGraphFailureExpectationsIncludeExistingTealSource(t *testing.T) {
	if _, err := exec.LookPath("tl"); err != nil {
		t.Skip("tl not installed")
	}
	dir := t.TempDir()
	path := writeGraphLua(t, dir, "broken.tl", `local value: string = 42; return {config_version=2}`)
	state := lua.NewState()
	defer state.Close()
	_, err := BuildSourceGraph(state, path, DefaultSourceGraphOptions())
	if err == nil {
		t.Fatal("expected Teal check failure")
	}
	found := false
	for _, expectation := range SourceGraphFailureExpectations(err) {
		if expectation.Path == path {
			found = true
		}
	}
	if !found {
		t.Fatalf("existing Teal source missing from %#v", SourceGraphFailureExpectations(err))
	}
}

func writeGraphLua(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func graphSource(marker, value, fields string) string {
	return `local f=assert(io.open(` + luaQuote(marker) + `,"a")); f:write("` + value + `"); f:close(); return {config_version=2, ` + fields + `}`
}

func readGraphMarker(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func luaQuote(value string) string {
	return `"` + strings.ReplaceAll(filepath.ToSlash(value), `"`, `\"`) + `"`
}

func installFakeGraphTeal(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "tl")
	if runtime.GOOS == "windows" {
		path += ".bat"
		writeGraphLua(t, dir, filepath.Base(path), "@echo off\r\nif \"%1\"==\"check\" exit /b 0\r\nif \"%1\"==\"gen\" copy \"%4\" \"%~dpn4.lua\" >nul & exit /b 0\r\nexit /b 1\r\n")
	} else {
		writeGraphLua(t, dir, filepath.Base(path), "#!/bin/sh\nif [ \"$1\" = check ]; then exit 0; fi\nif [ \"$1\" = gen ]; then cp \"$4\" \"${4%.tl}.lua\"; exit 0; fi\nexit 1\n")
		if err := os.Chmod(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func failureExpectationsContainPath(expectations []SourceWatchExpectation, want string) bool {
	for _, expectation := range expectations {
		if canonicalIdentity(expectation.Path) == canonicalIdentity(want) {
			return true
		}
		if filepath.Base(expectation.Path) != filepath.Base(want) {
			continue
		}
		gotParent, gotErr := os.Stat(filepath.Dir(expectation.Path))
		wantParent, wantErr := os.Stat(filepath.Dir(want))
		if gotErr == nil && wantErr == nil && os.SameFile(gotParent, wantParent) {
			return true
		}
	}
	return false
}
