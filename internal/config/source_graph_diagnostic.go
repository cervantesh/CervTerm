package config

// SourceGraphDiagnostic is a detached, value-free description of an evaluated
// local source graph. It deliberately excludes hashes, source bytes, Lua
// values, Teal staging details, and other executable state.
type SourceGraphDiagnostic struct {
	Primary      string                       `json:"primary"`
	Sources      []SourceNodeDiagnostic       `json:"sources"`
	Edges        []SourceEdgeDiagnostic       `json:"edges"`
	Dependencies []SourceDependencyDiagnostic `json:"dependencies"`
}

// SourceNodeDiagnostic describes source identity and schema migration only.
type SourceNodeDiagnostic struct {
	RequestedPath    string   `json:"requested_path"`
	CanonicalPath    string   `json:"canonical_path"`
	SelectedPath     string   `json:"selected_path"`
	SelectedPaths    []string `json:"selected_paths"`
	AuthoredVersion  int      `json:"authored_version"`
	EffectiveVersion int      `json:"effective_version"`
}

// SourceEdgeDiagnostic describes one declarative include edge.
type SourceEdgeDiagnostic struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Requested string `json:"requested"`
}

// SourceDependencyDiagnostic describes one captured local Lua dependency
// without retaining its content hash.
type SourceDependencyDiagnostic struct {
	Kind      DependencyKind `json:"kind"`
	Requested string         `json:"requested"`
	Canonical string         `json:"canonical"`
	Selected  string         `json:"selected"`
}

// Diagnostic returns a detached diagnostic snapshot of the graph.
func (g *SourceGraph) Diagnostic() SourceGraphDiagnostic {
	if g == nil {
		return SourceGraphDiagnostic{}
	}
	diagnostic := SourceGraphDiagnostic{
		Primary:      g.Primary,
		Sources:      make([]SourceNodeDiagnostic, 0, len(g.Sources)),
		Edges:        make([]SourceEdgeDiagnostic, 0, len(g.Edges)),
		Dependencies: make([]SourceDependencyDiagnostic, 0, len(g.Dependencies)),
	}
	for _, source := range g.Sources {
		diagnostic.Sources = append(diagnostic.Sources, SourceNodeDiagnostic{
			RequestedPath: source.RequestedPath, CanonicalPath: source.CanonicalPath,
			SelectedPath: source.SelectedPath, SelectedPaths: append([]string(nil), source.SelectedPaths...),
			AuthoredVersion: source.Document.AuthoredVersion, EffectiveVersion: source.Document.Version,
		})
	}
	for _, edge := range g.Edges {
		diagnostic.Edges = append(diagnostic.Edges, SourceEdgeDiagnostic{
			From: edge.From, To: edge.To, Requested: edge.Requested,
		})
	}
	for _, dependency := range g.Dependencies {
		diagnostic.Dependencies = append(diagnostic.Dependencies, SourceDependencyDiagnostic{
			Kind: dependency.Kind, Requested: dependency.Requested,
			Canonical: dependency.Canonical, Selected: dependency.Selected,
		})
	}
	return diagnostic
}

// Clone returns a deep copy of the diagnostic's mutable slices.
func (d SourceGraphDiagnostic) Clone() SourceGraphDiagnostic {
	clone := d
	clone.Sources = append([]SourceNodeDiagnostic(nil), d.Sources...)
	for index := range clone.Sources {
		clone.Sources[index].SelectedPaths = append([]string(nil), d.Sources[index].SelectedPaths...)
	}
	clone.Edges = append([]SourceEdgeDiagnostic(nil), d.Edges...)
	clone.Dependencies = append([]SourceDependencyDiagnostic(nil), d.Dependencies...)
	return clone
}
