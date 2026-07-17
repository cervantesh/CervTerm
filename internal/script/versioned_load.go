package script

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"cervterm/internal/config"

	lua "github.com/yuin/gopher-lua"
)

// VersionedSource is the result of one source evaluation. Authored v1 owns a
// legacy Runtime; authored v2 owns a CandidateBundle pending frontend commit.
type VersionedSource struct {
	Config           config.Config
	Runtime          *Runtime
	Candidate        *CandidateBundle
	LegacyTransition *config.LegacyTealTransition
	WatchPaths       []string
	WatchHashes      map[string][32]byte
	AuthoredVersion  int
}

type candidateEvaluation struct {
	state    *lua.LState
	graph    *config.SourceGraph
	timers   *timerTable
	statuses *statusTable
	overlays *overlayStore
}

func evaluateCandidate(path string, options config.SourceGraphOptions, allowLegacy bool) (*candidateEvaluation, error) {
	state := lua.NewState(lua.Options{SkipOpenLibs: false})
	timers := &timerTable{}
	statuses := &statusTable{}
	overlays := &overlayStore{}
	state.PreloadModule("cervterm", func(state *lua.LState) int {
		state.Push(buildModule(state, timers, statuses, overlays))
		return 1
	})
	var graph *config.SourceGraph
	var err error
	if allowLegacy {
		graph, err = config.BuildVersionedSourceGraph(state, path, options)
	} else {
		graph, err = config.BuildSourceGraph(state, path, options)
	}
	if err != nil {
		state.Close()
		return nil, err
	}
	return &candidateEvaluation{state: state, graph: graph, timers: timers, statuses: statuses, overlays: overlays}, nil
}

func (e *candidateEvaluation) close() {
	if e == nil {
		return
	}
	if e.graph != nil {
		_ = e.graph.Close()
		e.graph = nil
	}
	if e.state != nil {
		e.state.Close()
		e.state = nil
	}
}

func validateEvaluatedScripting(graph *config.SourceGraph) error {
	for _, source := range graph.Sources {
		if _, err := loadBindings(source.Document.Root); err != nil {
			return fmt.Errorf("%s: %w", source.CanonicalPath, err)
		}
		if _, err := loadEvents(source.Document.Root); err != nil {
			return fmt.Errorf("%s: %w", source.CanonicalPath, err)
		}
	}
	return nil
}

func buildCandidateBundleFromEvaluation(evaluation *candidateEvaluation, base config.Config, options CandidateOptions) (*CandidateBundle, error) {
	if err := validateEvaluatedScripting(evaluation.graph); err != nil {
		return nil, err
	}
	composition, err := config.ComposeSourceGraph(evaluation.state, evaluation.graph, options.Composition)
	if err != nil {
		return nil, err
	}
	resolved := config.FromDocument(cloneCandidateConfig(base), composition.Document)
	if err := resolved.Validate(); err != nil {
		return nil, err
	}
	bindings, err := loadBindings(composition.Document.Root)
	if err != nil {
		return nil, err
	}
	events, err := loadEvents(composition.Document.Root)
	if err != nil {
		return nil, err
	}
	runtime := &Runtime{
		state: evaluation.state, bindings: bindings, events: events, timers: evaluation.timers,
		statuses: evaluation.statuses, overlays: evaluation.overlays, dispatchTimeout: time.Second,
	}
	bundle := &CandidateBundle{
		config: cloneCandidateConfig(resolved), runtime: runtime,
		graph: evaluation.graph, composition: composition,
	}
	evaluation.state, evaluation.graph = nil, nil
	return bundle, nil
}

// LoadVersioned evaluates the selected source exactly once and dispatches by
// authored schema version. Omitted config_version remains the untouched v1
// single-source path; only explicit v2 produces a composed candidate bundle.
func LoadVersioned(path string, base config.Config, options CandidateOptions) (VersionedSource, error) {
	evaluation, err := evaluateCandidate(path, options.SourceGraph, true)
	if err != nil {
		return VersionedSource{}, err
	}
	defer evaluation.close()
	primary, ok := evaluation.graph.PrimaryNode()
	if !ok {
		return VersionedSource{}, fmt.Errorf("evaluated source graph has no primary document")
	}
	if primary.Document.AuthoredVersion == 2 {
		bundle, err := buildCandidateBundleFromEvaluation(evaluation, base, options)
		if err != nil {
			return VersionedSource{}, err
		}
		return VersionedSource{Config: bundle.Config(), Candidate: bundle, WatchPaths: bundle.WatchPaths(), WatchHashes: bundle.WatchHashes(), AuthoredVersion: 2}, nil
	}
	if err := validateEvaluatedScripting(evaluation.graph); err != nil {
		return VersionedSource{}, err
	}
	resolved := config.FromDocument(cloneCandidateConfig(base), primary.Document)
	if err := resolved.Validate(); err != nil {
		return VersionedSource{}, err
	}
	bindings, err := loadBindings(primary.Document.Root)
	if err != nil {
		return VersionedSource{}, err
	}
	events, err := loadEvents(primary.Document.Root)
	if err != nil {
		return VersionedSource{}, err
	}
	var legacyTransition *config.LegacyTealTransition
	if strings.HasSuffix(strings.ToLower(path), ".tl") {
		if len(evaluation.graph.StagedTeal) == 1 {
			legacyTransition, err = config.PrepareLegacyTealTransition(evaluation.graph.StagedTeal[0])
			if err != nil {
				return VersionedSource{}, err
			}
		}
		rollbackTransition := func(cause error) error {
			if legacyTransition == nil {
				return cause
			}
			return errors.Join(cause, legacyTransition.Rollback())
		}
		generated, err := config.GenerateTeal(path)
		if err != nil {
			return VersionedSource{}, rollbackTransition(err)
		}
		// Cleanly cross back to marker-free v1 only when a valid marker from this
		// source exists. A failed removal restores the journal before returning.
		if _, removeErr := config.RemoveOwnedTealMarker(generated, path); removeErr != nil && legacyTransition != nil {
			return VersionedSource{}, rollbackTransition(removeErr)
		}
	}
	watchPaths := watchPathsForGraph(evaluation.graph)
	watchHashes := watchHashesForGraph(evaluation.graph)
	_ = evaluation.graph.Close()
	evaluation.graph = nil
	runtime := &Runtime{
		state: evaluation.state, bindings: bindings, events: events, timers: evaluation.timers,
		statuses: evaluation.statuses, overlays: evaluation.overlays, dispatchTimeout: time.Second,
	}
	evaluation.state = nil
	return VersionedSource{Config: cloneCandidateConfig(resolved), Runtime: runtime, LegacyTransition: legacyTransition, WatchPaths: watchPaths, WatchHashes: watchHashes, AuthoredVersion: 1}, nil
}
