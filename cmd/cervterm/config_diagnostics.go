package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"cervterm/internal/config"
	"cervterm/internal/script"
)

const configExplanationFormatVersion = 1

type configDiagnosticOptions struct {
	ConfigPath       string
	Candidate        script.CandidateOptions
	Fields           []string
	DisableDiscovery bool
}

type configDiagnosticReport struct {
	SourcePath      string
	AuthoredVersion int
	Config          config.Config
	Selection       config.SelectionResult
	Diagnostic      config.ConfigDiagnostic
	Graph           config.SourceGraphDiagnostic
	Composition     bool
}

type configDiagnosticTargetError struct{ message string }

func (e configDiagnosticTargetError) Error() string { return e.message }

func loadConfigDiagnostic(opts configDiagnosticOptions, requireV2 bool) (configDiagnosticReport, func(), error) {
	path := strings.TrimSpace(opts.ConfigPath)
	if path == "" && !opts.DisableDiscovery {
		path = config.DiscoverPath()
	}
	if path == "" {
		if requireV2 || opts.Candidate.RequiresVersion2() {
			return configDiagnosticReport{}, func() {}, configDiagnosticTargetError{message: "config explanation requires an explicit config_version=2 source"}
		}
		return configDiagnosticReport{Config: config.Defaults()}, func() {}, nil
	}
	candidate := opts.Candidate.Clone()
	candidate.DiagnosticOnly = true
	loaded, err := script.LoadVersioned(path, config.Defaults(), candidate)
	if err != nil {
		return configDiagnosticReport{}, func() {}, err
	}
	cleanup := func() { closeVersionedSource(&loaded) }
	report := configDiagnosticReport{SourcePath: path, AuthoredVersion: loaded.AuthoredVersion, Config: loaded.Config.Clone()}
	if loaded.AuthoredVersion != 2 || loaded.Candidate == nil {
		if requireV2 || opts.Candidate.RequiresVersion2() {
			cleanup()
			return configDiagnosticReport{}, func() {}, configDiagnosticTargetError{message: "config explanation requires config_version=2"}
		}
		return report, cleanup, nil
	}
	report.Composition = true
	report.Selection = loaded.Candidate.Selection()
	report.Graph = loaded.Candidate.SourceGraphDiagnostic()
	report.Diagnostic, err = loaded.Candidate.ConfigDiagnostic(opts.Fields)
	if err != nil {
		cleanup()
		var pathErr config.ConfigDiagnosticPathError
		if errors.As(err, &pathErr) {
			return configDiagnosticReport{}, func() {}, configDiagnosticTargetError{message: fmt.Sprintf("unknown config field %q", pathErr.Path)}
		}
		return configDiagnosticReport{}, func() {}, err
	}
	return report, cleanup, nil
}

func runExplainConfig(opts configDiagnosticOptions) int {
	return runExplainConfigTo(os.Stdout, os.Stderr, opts)
}

func runExplainConfigTo(stdout, stderr io.Writer, opts configDiagnosticOptions) int {
	report, cleanup, err := loadConfigDiagnostic(opts, true)
	if err != nil {
		fmt.Fprintf(stderr, "config explanation error: %v\n", err)
		var target configDiagnosticTargetError
		if errors.As(err, &target) {
			return 2
		}
		return 1
	}
	defer cleanup()
	renderConfigDiagnostic(stdout, report, "CervTerm config explanation")
	return 0
}

func renderConfigDiagnostic(out io.Writer, report configDiagnosticReport, heading string) {
	fmt.Fprintln(out, heading)
	fmt.Fprintf(out, "format-version: %d\n", configExplanationFormatVersion)
	fmt.Fprintf(out, "source: %s\n", strconv.Quote(report.SourcePath))
	fmt.Fprintf(out, "schema: authored=%d effective=%d\n", report.AuthoredVersion, report.Diagnostic.ConfigVersion)
	renderSelection(out, report.Selection)
	fmt.Fprintln(out, "sources:")
	for _, source := range report.Graph.Sources {
		fmt.Fprintf(out, "  - requested=%s canonical=%s version=%d->%d\n", strconv.Quote(source.RequestedPath), strconv.Quote(source.CanonicalPath), source.AuthoredVersion, source.EffectiveVersion)
	}
	fmt.Fprintln(out, "edges:")
	for _, edge := range report.Graph.Edges {
		fmt.Fprintf(out, "  - %s -> %s requested=%s\n", strconv.Quote(edge.From), strconv.Quote(edge.To), strconv.Quote(edge.Requested))
	}
	fmt.Fprintln(out, "dependencies:")
	for _, dependency := range report.Graph.Dependencies {
		fmt.Fprintf(out, "  - kind=%s requested=%s selected=%s canonical=%s\n", dependency.Kind, strconv.Quote(dependency.Requested), strconv.Quote(dependency.Selected), strconv.Quote(dependency.Canonical))
	}
	fmt.Fprintln(out, "fields:")
	for _, field := range report.Diagnostic.Fields {
		fmt.Fprintf(out, "  %s = %s [scope=%s", field.Metadata.Path, field.Value, field.Metadata.ApplyScope)
		if field.Metadata.Sensitive {
			fmt.Fprint(out, " sensitive=true")
		}
		fmt.Fprintln(out, "]")
		for _, record := range field.Provenance {
			fmt.Fprintf(out, "    provenance-path: %s\n", record.Path)
			fmt.Fprintf(out, "    winner: %s\n", renderProvenanceOrigin(record.Winner))
			for _, origin := range record.Overwritten {
				fmt.Fprintf(out, "    overwritten: %s\n", renderProvenanceOrigin(origin))
			}
		}
	}
}

func renderSelection(out io.Writer, selection config.SelectionResult) {
	fmt.Fprintln(out, "selection:")
	if selection.Environment == nil {
		fmt.Fprintln(out, "  environment: none")
	} else {
		fmt.Fprintf(out, "  environment: %s [%s]\n", strconv.Quote(selection.Environment.Name), selection.Environment.Basis)
	}
	if selection.Profile == nil {
		fmt.Fprintln(out, "  profile: none")
	} else {
		fmt.Fprintf(out, "  profile: %s [%s]\n", strconv.Quote(selection.Profile.Name), selection.Profile.Basis)
	}
}

func renderProvenanceOrigin(origin config.ProvenanceOrigin) string {
	parts := []string{"layer=" + string(origin.Layer)}
	if origin.Name != "" {
		parts = append(parts, "name="+strconv.Quote(origin.Name))
	}
	if origin.RequestedSource != "" {
		parts = append(parts, "requested="+strconv.Quote(origin.RequestedSource))
	}
	if origin.CanonicalSource != "" {
		parts = append(parts, "canonical="+strconv.Quote(origin.CanonicalSource))
	}
	if origin.AuthoredVersion != 0 || origin.Version != 0 {
		parts = append(parts, fmt.Sprintf("version=%d->%d", origin.AuthoredVersion, origin.Version))
	}
	if origin.HasCLIArgumentIndex {
		parts = append(parts, fmt.Sprintf("argument=%d", origin.CLIArgumentIndex))
	}
	if origin.HasConfigScopeID {
		parts = append(parts, "scope="+origin.ConfigScopeID.String())
	}
	return strings.Join(parts, " ")
}
