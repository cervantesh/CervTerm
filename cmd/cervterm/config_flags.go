package main

import (
	"flag"
	"fmt"
	"runtime"
	"strings"

	"cervterm/internal/config"
	"cervterm/internal/script"
)

type optionalFlagValue struct {
	value string
	set   bool
}

func (v *optionalFlagValue) String() string { return v.value }
func (v *optionalFlagValue) Set(value string) error {
	v.value, v.set = value, true
	return nil
}

type repeatedFlagValues []string

func (v *repeatedFlagValues) String() string { return fmt.Sprintf("<%d override(s)>", len(*v)) }
func (v *repeatedFlagValues) Set(value string) error {
	*v = append(*v, value)
	return nil
}

type frontendStartupFlags struct {
	safeFonts bool
}

func registerFrontendStartupFlags(flags *flag.FlagSet) *frontendStartupFlags {
	values := &frontendStartupFlags{}
	flags.BoolVar(&values.safeFonts, "safe-fonts", false, "force the embedded Go Mono startup font")
	return values
}

type compositionFlags struct {
	environment optionalFlagValue
	profile     optionalFlagValue
	overrides   repeatedFlagValues
}

func registerCompositionFlags(flags *flag.FlagSet) *compositionFlags {
	values := &compositionFlags{}
	flags.Var(&values.environment, "environment", "select a named v2 environment (env: CERVTERM_ENV)")
	flags.Var(&values.profile, "profile", "select a named v2 profile (env: CERVTERM_PROFILE)")
	flags.Var(&values.overrides, "config-override", "typed v2 PATH=VALUE override; repeat to apply left-to-right")
	return values
}

type explainConfigFlags struct {
	all    bool
	fields repeatedFlagValues
}

func registerExplainConfigFlags(flags *flag.FlagSet) *explainConfigFlags {
	values := &explainConfigFlags{}
	flags.BoolVar(&values.all, "explain-config", false, "print resolved v2 configuration with provenance and exit")
	flags.Var(&values.fields, "explain-config-field", "resolved v2 field path to explain; repeat to filter")
	return values
}

func (f *explainConfigFlags) requested() bool {
	return f != nil && (f.all || len(f.fields) != 0)
}

type environmentLookup func(string) (string, bool)

func (f *compositionFlags) candidateOptions(args []string, lookup environmentLookup) (script.CandidateOptions, error) {
	selection := config.SelectionOptions{GOOS: runtime.GOOS}
	if f.environment.set {
		value := f.environment.value
		selection.EnvironmentOverride = &value
	}
	if value, ok := lookup("CERVTERM_ENV"); ok {
		selection.EnvironmentVariableValue = &value
	}
	if f.profile.set {
		value := f.profile.value
		selection.ProfileOverride = &value
	}
	if value, ok := lookup("CERVTERM_PROFILE"); ok {
		selection.ProfileVariableValue = &value
	}
	indexes := configOverrideArgumentIndexes(args)
	if len(indexes) != len(f.overrides) {
		return script.CandidateOptions{}, fmt.Errorf("resolve --config-override arguments: parsed %d flags but found %d argument positions", len(f.overrides), len(indexes))
	}
	overrides := make([]config.CLIOverride, 0, len(f.overrides))
	for index, raw := range f.overrides {
		path, value, ok := strings.Cut(raw, "=")
		if !ok {
			return script.CandidateOptions{}, fmt.Errorf("--config-override argument %d: expected PATH=VALUE", indexes[index])
		}
		overrides = append(overrides, config.CLIOverride{ArgumentIndex: indexes[index], Path: path, Value: value})
	}
	return script.CandidateOptions{Composition: config.CompositionOptions{Selection: selection, CLIOverrides: overrides}}, nil
}

func (f *compositionFlags) explicitlyRequested() bool {
	return f.environment.set || f.profile.set || len(f.overrides) != 0
}

func configOverrideArgumentIndexes(args []string) []int {
	indexes := make([]int, 0)
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" {
			break
		}
		if argument == "-config-override" || argument == "--config-override" {
			indexes = append(indexes, index+1)
			index++
			continue
		}
		if strings.HasPrefix(argument, "-config-override=") || strings.HasPrefix(argument, "--config-override=") {
			indexes = append(indexes, index+1)
		}
	}
	return indexes
}

func closeVersionedSource(loaded *script.VersionedSource) {
	if loaded == nil {
		return
	}
	if loaded.Candidate != nil {
		loaded.Candidate.Close()
		loaded.Candidate = nil
	} else if loaded.Runtime != nil {
		loaded.Runtime.Close()
		loaded.Runtime = nil
	}
	if loaded.LegacyTransition != nil {
		_ = loaded.LegacyTransition.Rollback()
		loaded.LegacyTransition = nil
	}
}

func validateCompositionTarget(flags *compositionFlags, sourcePath string, authoredVersion int) error {
	if flags == nil || !flags.explicitlyRequested() {
		return nil
	}
	if sourcePath == "" {
		return fmt.Errorf("--environment, --profile, and --config-override require an explicit config_version=2 source")
	}
	if authoredVersion != 0 && authoredVersion != 2 {
		return fmt.Errorf("--environment, --profile, and --config-override require config_version=2")
	}
	return nil
}
