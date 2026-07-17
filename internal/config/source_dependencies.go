package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

type DependencyKind string

const (
	DependencyRequire  DependencyKind = "require"
	DependencyDoFile   DependencyKind = "dofile"
	DependencyLoadFile DependencyKind = "loadfile"
)

type SourceDependency struct {
	Kind      DependencyKind
	Requested string
	Canonical string
}

type dependencyCapture struct {
	state            *lua.LState
	originalPackage  lua.LValue
	packageTable     *lua.LTable
	originalRequire  lua.LValue
	originalDoFile   lua.LValue
	originalLoadFile lua.LValue
	wrappedRequire   lua.LValue
	wrappedDoFile    lua.LValue
	wrappedLoadFile  lua.LValue
	loaders          *lua.LTable
	loaderValues     []lua.LValue
	dependencies     map[string]SourceDependency
}

const dependencyRecorderGlobal = "__cervterm_record_config_dependency"

func installDependencyCapture(state *lua.LState) (*dependencyCapture, error) {
	packageValue := state.GetGlobal("package")
	packageTable, ok := packageValue.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("config dependency capture: package table is unavailable")
	}
	capture := &dependencyCapture{
		state: state, originalPackage: packageValue, packageTable: packageTable,
		originalRequire: state.GetGlobal("require"), originalDoFile: state.GetGlobal("dofile"),
		originalLoadFile: state.GetGlobal("loadfile"), dependencies: make(map[string]SourceDependency),
	}
	capture.loaders, _ = packageTable.RawGetString("loaders").(*lua.LTable)
	if capture.loaders == nil {
		return nil, fmt.Errorf("config dependency capture: package.loaders is unavailable")
	}
	capture.loaderValues = make([]lua.LValue, capture.loaders.Len())
	for i := 1; i <= capture.loaders.Len(); i++ {
		capture.loaderValues[i-1] = capture.loaders.RawGetInt(i)
	}
	state.SetGlobal(dependencyRecorderGlobal, state.NewFunction(capture.recordLua))
	if err := state.DoString(`
local cervterm_record_config_dependency = __cervterm_record_config_dependency
local cervterm_original_require = require
local cervterm_original_dofile = dofile
local cervterm_original_loadfile = loadfile
require = function(name)
  cervterm_record_config_dependency("require", name)
  return cervterm_original_require(name)
end
dofile = function(path)
  cervterm_record_config_dependency("dofile", path)
  return cervterm_original_dofile(path)
end
loadfile = function(path)
  cervterm_record_config_dependency("loadfile", path)
  return cervterm_original_loadfile(path)
end
`); err != nil {
		capture.restore()
		return nil, fmt.Errorf("install config dependency capture: %w", err)
	}
	state.SetGlobal(dependencyRecorderGlobal, lua.LNil)
	capture.wrappedRequire = state.GetGlobal("require")
	capture.wrappedDoFile = state.GetGlobal("dofile")
	capture.wrappedLoadFile = state.GetGlobal("loadfile")
	return capture, nil
}

func (c *dependencyCapture) restore() {
	if c == nil || c.state == nil {
		return
	}
	c.state.SetGlobal("require", c.originalRequire)
	c.state.SetGlobal("dofile", c.originalDoFile)
	c.state.SetGlobal("loadfile", c.originalLoadFile)
	c.state.SetGlobal("package", c.originalPackage)
	if c.packageTable != nil && c.loaders != nil {
		c.packageTable.RawSetString("loaders", c.loaders)
		for i, value := range c.loaderValues {
			c.loaders.RawSetInt(i+1, value)
		}
		for c.loaders.Len() > len(c.loaderValues) {
			c.loaders.RawSetInt(c.loaders.Len(), lua.LNil)
		}
	}
	c.state.SetGlobal(dependencyRecorderGlobal, lua.LNil)
}

func (c *dependencyCapture) verifyStrict() error {
	if c.state.GetGlobal("require") != c.wrappedRequire || c.state.GetGlobal("dofile") != c.wrappedDoFile || c.state.GetGlobal("loadfile") != c.wrappedLoadFile {
		return fmt.Errorf("config v2 must not replace require, dofile, or loadfile while dependency capture is active")
	}
	packageTable, ok := c.state.GetGlobal("package").(*lua.LTable)
	if !ok || packageTable != c.packageTable {
		return fmt.Errorf("config v2 must not replace package")
	}
	if packageTable.RawGetString("loaders") != c.loaders || c.loaders.Len() != len(c.loaderValues) {
		return fmt.Errorf("config v2 must not replace package.loaders")
	}
	for i, expected := range c.loaderValues {
		if c.loaders.RawGetInt(i+1) != expected {
			return fmt.Errorf("config v2 must not install or replace package.loaders[%d]", i+1)
		}
	}
	return nil
}

func (c *dependencyCapture) list() []SourceDependency {
	out := make([]SourceDependency, 0, len(c.dependencies))
	for _, dependency := range c.dependencies {
		out = append(out, dependency)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Canonical == out[j].Canonical {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Canonical < out[j].Canonical
	})
	return out
}

func (c *dependencyCapture) recordLua(state *lua.LState) int {
	kind := DependencyKind(state.CheckString(1))
	requested := state.CheckString(2)
	var resolved string
	switch kind {
	case DependencyRequire:
		resolved = c.resolveRequiredModule(requested)
	case DependencyDoFile, DependencyLoadFile:
		resolved = requested
	default:
		return 0
	}
	if resolved == "" {
		return 0
	}
	canonical, _, err := canonicalLocalFile(resolved)
	if err != nil {
		// Preserve standard Lua's own error and return semantics. Dependency capture
		// records only paths that resolve to an existing regular local file.
		return 0
	}
	key := string(kind) + "\x00" + canonicalIdentity(canonical)
	c.dependencies[key] = SourceDependency{Kind: kind, Requested: requested, Canonical: canonical}
	return 0
}

func (c *dependencyCapture) resolveRequiredModule(name string) string {
	packageTable, ok := c.state.GetGlobal("package").(*lua.LTable)
	if !ok {
		return ""
	}
	if loaded, ok := packageTable.RawGetString("loaded").(*lua.LTable); ok && loaded.RawGetString(name) != lua.LNil {
		return ""
	}
	if preload, ok := packageTable.RawGetString("preload").(*lua.LTable); ok && preload.RawGetString(name) != lua.LNil {
		return ""
	}
	pathValue, ok := packageTable.RawGetString("path").(lua.LString)
	if !ok {
		return ""
	}
	modulePath := strings.ReplaceAll(name, ".", string(filepath.Separator))
	for _, pattern := range strings.Split(string(pathValue), ";") {
		if pattern == "" {
			continue
		}
		candidate := strings.ReplaceAll(pattern, "?", modulePath)
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() {
			return candidate
		}
	}
	return ""
}
