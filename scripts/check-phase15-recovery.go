//go:build ignore

// check-phase15-recovery runs the exact recovery/redaction qualification suites.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

type recoverySuite struct {
	name  string
	pkg   string
	tags  string
	tests []string
}

func main() {
	race := flag.Bool("race", false, "run the focused suites with the race detector")
	flag.Parse()

	suites := []recoverySuite{
		{
			name: "logging recovery", pkg: "./internal/applog",
			tests: []string{"TestSetupAppendsToLogFile", "TestSetupFailureIsClassifiedWithoutPrivatePath", "TestFormatPanicReportRedactsValuesContextsAndSourcePaths", "TestSafePanicContextAndClass"},
		},
		{
			name: "layout storage recovery", pkg: "./internal/layoutstate",
			tests: []string{"TestStoreInvalidDocumentsAreTypedAndRedacted", "TestStoreReplacementFailurePreservesOldFileAndCleansTemp", "TestStorePrecommitFaultsPreserveOldFileAndCleanTemp"},
		},
		{
			name: "config, restore, and transient image recovery", pkg: "./internal/frontend/glfwgl", tags: "glfw",
			tests: []string{
				"TestReloadFailurePreservesConfigAndRuntime",
				"TestRuntimeScopeSurvivesReloadRejectsInvalidCandidateAndClears",
				"TestBackgroundGPUPreparationFailureRollsBackWholeReload",
				"TestLoadConfiguredRestorePlanIsOptInAndUsesConfiguredStore",
				"TestRestoreLoadPolicyTurnsInvalidStateIntoFreshValueFreeFallback",
				"TestResetFailedRestoreFrontendClearsAbandonedAppearance",
				"TestTerminalImageActivationFactoryErrorClosesCandidateAndRollsBackStartupMux",
				"TestTerminalImageActivationCommitFailureClosesCacheThenMux",
				"TestTerminalImageActivationRestorePreparationAndBindFailuresCloseEveryCache",
				"TestTerminalImageCacheRetriesSameGenerationOnFixedScheduleAndThenStops",
				"TestTerminalImageCacheClosesWithCurrentContextBeforeRendererAcrossProjectionLifecycles",
				"TestTerminalImageCacheCloseIsIdempotentAndDeterministic",
			},
		},
	}
	for _, suite := range suites {
		if err := runRecoverySuite(suite, *race); err != nil {
			fmt.Fprintf(os.Stderr, "Phase 15 recovery gate failed in %s: %v\n", suite.name, err)
			os.Exit(1)
		}
	}
	fmt.Println("Phase 15 recovery and redaction gates passed")
}

func runRecoverySuite(suite recoverySuite, race bool) error {
	pattern := "^(" + strings.Join(suite.tests, "|") + ")$"
	base := []string{"test"}
	if suite.tags != "" {
		base = append(base, "-tags", suite.tags)
	}
	if race {
		base = append(base, "-race")
	}
	listArgs := append(append([]string(nil), base...), suite.pkg, "-list", pattern)
	list := exec.Command("go", listArgs...)
	list.Env = os.Environ()
	output, err := list.CombinedOutput()
	if err != nil {
		return fmt.Errorf("list tests: %w\n%s", err, output)
	}
	listed := make(map[string]struct{}, len(suite.tests))
	for _, line := range bytes.Split(output, []byte{'\n'}) {
		name := strings.TrimSpace(string(line))
		if strings.HasPrefix(name, "Test") {
			listed[name] = struct{}{}
		}
	}
	for _, name := range suite.tests {
		if _, exists := listed[name]; !exists {
			return fmt.Errorf("required test %s did not match -list output", name)
		}
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return fmt.Errorf("invalid generated test pattern: %w", err)
	}
	runArgs := append(append([]string(nil), base...), suite.pkg, "-run", pattern, "-count=1")
	fmt.Printf("==> %s: go %s\n", suite.name, strings.Join(runArgs, " "))
	cmd := exec.Command("go", runArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Run()
}
