package config

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

type SelectionBasis string

const (
	SelectionExplicit            SelectionBasis = "explicit"
	SelectionEnvironmentVariable SelectionBasis = "environment_variable"
	SelectionDefault             SelectionBasis = "default"
	SelectionGOOS                SelectionBasis = "goos"
)

type SelectionChoice struct {
	Name          string
	Basis         SelectionBasis
	DefaultOrigin *ProvenanceOrigin
}

type SelectionResult struct {
	Environment *SelectionChoice
	Profile     *SelectionChoice
}

// SelectionOptions contains already-resolved external inputs. A non-nil pointer
// means that source was explicitly present (even when its value is empty); this
// package never reads process arguments or environment variables itself.
type SelectionOptions struct {
	EnvironmentOverride      *string
	EnvironmentVariableValue *string
	ProfileOverride          *string
	ProfileVariableValue     *string
	GOOS                     string
}

type selectionDefault struct {
	name   string
	origin ProvenanceOrigin
}

type selectionCatalog struct {
	environments       map[string]struct{}
	profiles           map[string]struct{}
	defaultEnvironment *selectionDefault
	defaultProfile     *selectionDefault
}

func buildSelectionCatalog(graph *SourceGraph) selectionCatalog {
	catalog := selectionCatalog{environments: make(map[string]struct{}), profiles: make(map[string]struct{})}
	for _, source := range graph.Sources {
		collectDeclarationNames(source.Document.Root, "environments", catalog.environments)
		collectDeclarationNames(source.Document.Root, "profiles", catalog.profiles)
		if value, ok := source.Document.Root.RawGetString("default_environment").(lua.LString); ok {
			catalog.defaultEnvironment = &selectionDefault{name: string(value), origin: sourceLayerOrigin(graph, source, LayerInclude, source.RequestedPath)}
		}
		if value, ok := source.Document.Root.RawGetString("default_profile").(lua.LString); ok {
			catalog.defaultProfile = &selectionDefault{name: string(value), origin: sourceLayerOrigin(graph, source, LayerInclude, source.RequestedPath)}
		}
	}
	return catalog
}

func resolveSelection(catalog selectionCatalog, options SelectionOptions) (SelectionResult, error) {
	environment, err := chooseSelection(
		"environment", catalog.environments, options.EnvironmentOverride, options.EnvironmentVariableValue,
		catalog.defaultEnvironment, options.GOOS,
	)
	if err != nil {
		return SelectionResult{}, err
	}
	profile, err := chooseSelection("profile", catalog.profiles, options.ProfileOverride, options.ProfileVariableValue, catalog.defaultProfile, "")
	if err != nil {
		return SelectionResult{}, err
	}
	return SelectionResult{Environment: environment, Profile: profile}, nil
}

func chooseSelection(kind string, names map[string]struct{}, explicit, variable *string, configured *selectionDefault, goos string) (*SelectionChoice, error) {
	if explicit != nil {
		return requireSelection(kind, *explicit, SelectionExplicit, nil, names)
	}
	if variable != nil {
		return requireSelection(kind, *variable, SelectionEnvironmentVariable, nil, names)
	}
	if configured != nil {
		origin := configured.origin
		return requireSelection(kind, configured.name, SelectionDefault, &origin, names)
	}
	if goos != "" {
		if _, ok := names[goos]; ok {
			return &SelectionChoice{Name: goos, Basis: SelectionGOOS}, nil
		}
	}
	return nil, nil
}

func requireSelection(kind, name string, basis SelectionBasis, origin *ProvenanceOrigin, names map[string]struct{}) (*SelectionChoice, error) {
	if _, ok := names[name]; !ok {
		return nil, fmt.Errorf("selected %s %q from %s is not declared", kind, name, basis)
	}
	return &SelectionChoice{Name: name, Basis: basis, DefaultOrigin: origin}, nil
}

func collectDeclarationNames(root *lua.LTable, field string, out map[string]struct{}) {
	table, ok := root.RawGetString(field).(*lua.LTable)
	if !ok {
		return
	}
	table.ForEach(func(key, _ lua.LValue) {
		if name, ok := key.(lua.LString); ok {
			out[string(name)] = struct{}{}
		}
	})
}

func sourceLayerOrigin(graph *SourceGraph, source SourceNode, layer ProvenanceLayer, name string) ProvenanceOrigin {
	if layer == LayerInclude && canonicalIdentity(source.CanonicalPath) == canonicalIdentity(graph.Primary) {
		layer = LayerPrimary
		name = "primary"
	}
	return ProvenanceOrigin{
		Layer: layer, Name: name, RequestedSource: source.RequestedPath, CanonicalSource: source.CanonicalPath,
		AuthoredVersion: source.Document.AuthoredVersion, Version: source.Document.Version,
	}
}
