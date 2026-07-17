package script

import (
	"fmt"
	"time"

	"cervterm/internal/config"

	lua "github.com/yuin/gopher-lua"
)

// CandidateOptions supplies pure composition and graph-building inputs.
type CandidateOptions struct {
	Composition config.CompositionOptions
	SourceGraph config.SourceGraphOptions
}

// CandidateBundle owns every resource created for one validated composed v2
// candidate until a future atomic frontend transfer or Close. Like Runtime, its
// lifecycle methods are main-thread-only and must not be called concurrently.
type CandidateBundle struct {
	config      config.Config
	runtime     *Runtime
	graph       *config.SourceGraph
	composition config.Composition
	publication config.TealPublicationResult
	published   bool
	closed      bool
}

// BuildCandidateBundle builds and validates a composed v2 candidate without
// mutating active application state or publishing staged Teal outputs.
func BuildCandidateBundle(path string, base config.Config, options CandidateOptions) (*CandidateBundle, error) {
	state := lua.NewState(lua.Options{SkipOpenLibs: false})
	timers := &timerTable{}
	statuses := &statusTable{}
	overlays := &overlayStore{}
	state.PreloadModule("cervterm", func(state *lua.LState) int {
		state.Push(buildModule(state, timers, statuses, overlays))
		return 1
	})
	graph, err := config.BuildSourceGraph(state, path, options.SourceGraph)
	if err != nil {
		state.Close()
		return nil, err
	}
	fail := func(err error) (*CandidateBundle, error) {
		_ = graph.Close()
		state.Close()
		return nil, err
	}
	// V1 partials retain legacy fail-fast scripting surfaces even when a later
	// layer would replace them; validate every source before effective merge.
	for _, source := range graph.Sources {
		if _, err := loadBindings(source.Document.Root); err != nil {
			return fail(fmt.Errorf("%s: %w", source.CanonicalPath, err))
		}
		if _, err := loadEvents(source.Document.Root); err != nil {
			return fail(fmt.Errorf("%s: %w", source.CanonicalPath, err))
		}
	}
	composition, err := config.ComposeSourceGraph(state, graph, options.Composition)
	if err != nil {
		return fail(err)
	}
	resolved := config.FromDocument(cloneCandidateConfig(base), composition.Document)
	if err := resolved.Validate(); err != nil {
		return fail(err)
	}
	bindings, err := loadBindings(composition.Document.Root)
	if err != nil {
		return fail(err)
	}
	events, err := loadEvents(composition.Document.Root)
	if err != nil {
		return fail(err)
	}
	runtime := &Runtime{
		state: state, bindings: bindings, events: events, timers: timers, statuses: statuses,
		overlays: overlays, dispatchTimeout: time.Second,
	}
	return &CandidateBundle{config: cloneCandidateConfig(resolved), runtime: runtime, graph: graph, composition: composition}, nil
}

// Config returns a detached copy of the validated candidate configuration.
func (b *CandidateBundle) Config() config.Config {
	if b == nil {
		return config.Config{}
	}
	return cloneCandidateConfig(b.config)
}

// Provenance returns detached effective provenance records.
func (b *CandidateBundle) Provenance() []config.ProvenanceRecord {
	if b == nil {
		return nil
	}
	return b.composition.Provenance.Records()
}

// Selection returns a detached copy of candidate selection decisions.
func (b *CandidateBundle) Selection() config.SelectionResult {
	if b == nil {
		return config.SelectionResult{}
	}
	return cloneSelectionResult(b.composition.Selection)
}

// Dependencies returns a detached copy of resolved local dependencies.
func (b *CandidateBundle) Dependencies() []config.SourceDependency {
	if b == nil || b.graph == nil {
		return nil
	}
	return append([]config.SourceDependency(nil), b.graph.Dependencies...)
}

// PublishTeal performs the deferred transactional Teal publication once. A
// failed attempt leaves the candidate owned and retryable.
func (b *CandidateBundle) PublishTeal(options config.TealPublicationOptions) (config.TealPublicationResult, error) {
	if b == nil || b.closed || b.graph == nil {
		return config.TealPublicationResult{}, fmt.Errorf("candidate bundle is closed")
	}
	if b.published {
		return cloneTealPublicationResult(b.publication), nil
	}
	result, err := config.PublishStagedTeal(b.graph, options)
	if err != nil {
		return config.TealPublicationResult{}, err
	}
	b.publication = cloneTealPublicationResult(result)
	b.published = true
	return cloneTealPublicationResult(result), nil
}

// Close releases the candidate runtime and staging graph exactly once.
func (b *CandidateBundle) Close() {
	if b == nil || b.closed {
		return
	}
	b.closed = true
	if b.runtime != nil {
		b.runtime.Close()
		b.runtime = nil
	}
	if b.graph != nil {
		_ = b.graph.Close()
		b.graph = nil
	}
}

func cloneSelectionResult(selection config.SelectionResult) config.SelectionResult {
	clone := func(choice *config.SelectionChoice) *config.SelectionChoice {
		if choice == nil {
			return nil
		}
		copied := *choice
		if choice.DefaultOrigin != nil {
			origin := *choice.DefaultOrigin
			copied.DefaultOrigin = &origin
		}
		return &copied
	}
	return config.SelectionResult{Environment: clone(selection.Environment), Profile: clone(selection.Profile)}
}

func cloneCandidateConfig(value config.Config) config.Config {
	value.Shell.Args = append([]string(nil), value.Shell.Args...)
	if value.Shell.Env != nil {
		environment := make(map[string]string, len(value.Shell.Env))
		for key, entry := range value.Shell.Env {
			environment[key] = entry
		}
		value.Shell.Env = environment
	}
	return value
}

func cloneTealPublicationResult(result config.TealPublicationResult) config.TealPublicationResult {
	result.Outputs = append([]config.TealPublishedOutput(nil), result.Outputs...)
	return result
}
