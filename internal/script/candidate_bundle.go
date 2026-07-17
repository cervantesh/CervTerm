package script

import (
	"fmt"

	"cervterm/internal/config"
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
	evaluation, err := evaluateCandidate(path, options.SourceGraph, false)
	if err != nil {
		return nil, err
	}
	defer evaluation.close()
	bundle, err := buildCandidateBundleFromEvaluation(evaluation, base, options)
	if err != nil {
		return nil, err
	}
	return bundle, nil
}

// Config returns a detached copy of the validated candidate configuration.
func (b *CandidateBundle) Config() config.Config {
	if b == nil {
		return config.Config{}
	}
	return cloneCandidateConfig(b.config)
}

// CandidateActivation is a prevalidated, allocation-complete handle whose Commit
// method only borrows the runtime from the still-owning bundle.
type CandidateActivation struct {
	bundle    *CandidateBundle
	runtime   *Runtime
	committed bool
}

// PrepareActivation performs all activation checks before external publication.
func (b *CandidateBundle) PrepareActivation() (*CandidateActivation, error) {
	if b == nil || b.closed || b.runtime == nil {
		return nil, fmt.Errorf("candidate bundle is closed")
	}
	return &CandidateActivation{bundle: b, runtime: b.runtime}, nil
}

// Commit is mechanically infallible. The frontend must retain and exclusively
// own the associated CandidateBundle for at least as long as the returned runtime.
func (a *CandidateActivation) Commit() *Runtime {
	if a == nil || a.committed || a.bundle == nil || a.bundle.closed {
		return nil
	}
	a.committed = true
	runtime := a.runtime
	a.runtime = nil
	return runtime
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
