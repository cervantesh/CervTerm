package config

import (
	"crypto/sha256"
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
	Selected  string
	Hash      [sha256.Size]byte
}

// SourceWatchHash binds evaluated bytes to their canonical file identity so a
// symlink retarget is observable even when both targets have identical content.
func SourceWatchHash(canonical string, content []byte) [sha256.Size]byte {
	hash := sha256.New()
	_, _ = hash.Write([]byte(canonicalIdentity(canonical)))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(content)
	var result [sha256.Size]byte
	copy(result[:], hash.Sum(nil))
	return result
}

// FileSourceWatchHash hashes a regular local file and its resolved identity.
func FileSourceWatchHash(path string) ([sha256.Size]byte, error) {
	canonical, _, err := canonicalLocalFile(path)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	content, err := os.ReadFile(canonical)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return SourceWatchHash(canonical, content), nil
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
	expectations     map[string]SourceWatchExpectation
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
		expectations: make(map[string]SourceWatchExpectation),
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
local function pack(...) return {n=select("#", ...), ...} end
local function finish(kind, name, values)
  cervterm_record_config_dependency(kind, name, "after")
  return unpack(values, 1, values.n)
end
require = function(name)
  cervterm_record_config_dependency("require", name, "before")
  return finish("require", name, pack(cervterm_original_require(name)))
end
dofile = function(path)
  cervterm_record_config_dependency("dofile", path, "before")
  return finish("dofile", path, pack(cervterm_original_dofile(path)))
end
loadfile = function(path)
  cervterm_record_config_dependency("loadfile", path, "before")
  return finish("loadfile", path, pack(cervterm_original_loadfile(path)))
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

// restoreLegacy removes only capture wrappers that remain installed. User v1
// replacements of globals, package, or package.loaders retain legacy lifetime.
func (c *dependencyCapture) restoreLegacy() {
	if c == nil || c.state == nil {
		return
	}
	if c.state.GetGlobal("require") == c.wrappedRequire {
		c.state.SetGlobal("require", c.originalRequire)
	}
	if c.state.GetGlobal("dofile") == c.wrappedDoFile {
		c.state.SetGlobal("dofile", c.originalDoFile)
	}
	if c.state.GetGlobal("loadfile") == c.wrappedLoadFile {
		c.state.SetGlobal("loadfile", c.originalLoadFile)
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
	phase := state.OptString(3, "before")
	var resolved string
	switch kind {
	case DependencyRequire:
		resolved = c.resolveRequiredModule(requested, phase)
	case DependencyDoFile, DependencyLoadFile:
		resolved = requested
	default:
		return 0
	}
	if resolved == "" {
		return 0
	}
	selected, err := filepath.Abs(resolved)
	if err != nil {
		return 0
	}
	selected = filepath.Clean(selected)
	c.addExpectation(selected)
	canonical, _, err := canonicalLocalFile(resolved)
	if err != nil {
		// Preserve standard Lua's own error and return semantics while retaining
		// the selected local path so creating the missing file can trigger retry.
		return 0
	}
	c.addExpectation(canonical)
	content, err := os.ReadFile(canonical)
	if err != nil {
		return 0
	}
	key := string(kind) + "\x00" + canonicalIdentity(canonical) + "\x00" + canonicalIdentity(selected)
	watchHash := SourceWatchHash(canonical, content)
	if previous, ok := c.dependencies[key]; ok && phase == "after" && previous.Hash != watchHash {
		// Retain the before-load hash. The frontend will compare it with the
		// post-evaluation file and queue the newer generation.
		return 0
	}
	c.dependencies[key] = SourceDependency{Kind: kind, Requested: requested, Canonical: canonical, Selected: selected, Hash: watchHash}
	return 0
}

func (c *dependencyCapture) resolveRequiredModule(name, phase string) string {
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
		absolute, err := filepath.Abs(candidate)
		if err == nil && phase == "before" {
			c.addExpectation(filepath.Clean(absolute))
		}
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() {
			return candidate
		}
	}
	return ""
}

func (c *dependencyCapture) addExpectation(path string) {
	if path == "" {
		return
	}
	cleaned := filepath.Clean(path)
	c.expectations[canonicalIdentity(cleaned)] = SourceWatchExpectation{Path: cleaned}
}

func (c *dependencyCapture) failureExpectations() []SourceWatchExpectation {
	out := make([]SourceWatchExpectation, 0, len(c.expectations))
	for _, expectation := range c.expectations {
		out = append(out, expectation)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
