package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const tealOwnershipVersion = 1

type TealPublicationOptions struct {
	FaultInjector func(outputIndex int, step string) error
	BeforeCommit  func() error
}

type TealPublishedOutput struct {
	SourcePath   string
	PublishedLua string
	MarkerPath   string
	Adopted      bool
}

type TealPublicationResult struct {
	Outputs []TealPublishedOutput
}

type tealOwnershipMarker struct {
	Version int    `json:"version"`
	Source  string `json:"source"`
}

type fileSnapshot struct {
	exists   bool
	data     []byte
	mode     os.FileMode
	identity os.FileInfo
}

type tealPublishPlan struct {
	staged        StagedTeal
	markerPath    string
	stagedBytes   []byte
	markerBytes   []byte
	oldOutput     fileSnapshot
	oldMarker     fileSnapshot
	tempOutput    string
	tempMarker    string
	adopted       bool
	outputTouched bool
	markerTouched bool
}

func TealOwnershipMarkerPath(publishedLua string) string {
	return publishedLua + ".cervterm-generated.json"
}

// RemoveOwnedTealMarker removes only a valid marker belonging to source. It is
// used best-effort after legacy `tl gen`; foreign or malformed files are ignored.
func RemoveOwnedTealMarker(publishedLua, source string) (bool, error) {
	path := TealOwnershipMarkerPath(publishedLua)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var marker tealOwnershipMarker
	if err := json.Unmarshal(data, &marker); err != nil || marker.Version != tealOwnershipVersion || canonicalIdentity(marker.Source) != canonicalIdentity(source) {
		return false, nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	return true, nil
}

// LegacyTealTransition journals v2-owned artifacts while an authored-v1
// candidate is prepared. Commit disarms rollback after frontend activation.
type LegacyTealTransition struct {
	outputPath string
	markerPath string
	output     fileSnapshot
	marker     fileSnapshot
	committed  bool
}

// PrepareLegacyTealTransition returns nil when no valid marker owned by staged
// source exists; foreign and malformed markers remain outside CervTerm ownership.
func PrepareLegacyTealTransition(staged StagedTeal) (*LegacyTealTransition, error) {
	markerPath := TealOwnershipMarkerPath(staged.PublishedLua)
	marker, err := snapshotRegularFile(markerPath)
	if err != nil {
		return nil, err
	}
	if !marker.exists {
		return nil, nil
	}
	var ownership tealOwnershipMarker
	if err := json.Unmarshal(marker.data, &ownership); err != nil || ownership.Version != tealOwnershipVersion || canonicalIdentity(ownership.Source) != canonicalIdentity(staged.SourcePath) {
		return nil, nil
	}
	output, err := snapshotRegularFile(staged.PublishedLua)
	if err != nil {
		return nil, err
	}
	return &LegacyTealTransition{outputPath: staged.PublishedLua, markerPath: markerPath, output: output, marker: marker}, nil
}

func (t *LegacyTealTransition) Commit() {
	if t != nil {
		t.committed = true
	}
}

func (t *LegacyTealTransition) Rollback() error {
	if t == nil || t.committed {
		return nil
	}
	t.committed = true
	var failures []error
	if err := restoreTealSnapshot(t.outputPath, t.output); err != nil {
		failures = append(failures, err)
	}
	if err := restoreTealSnapshot(t.markerPath, t.marker); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

// PublishStagedTeal publishes all graph-owned staged Teal outputs as one
// journaled transaction. Callers must complete composition and final validation
// first. Any failure restores every already-touched output and marker.
func PublishStagedTeal(graph *SourceGraph, options TealPublicationOptions) (TealPublicationResult, error) {
	if graph == nil {
		return TealPublicationResult{}, fmt.Errorf("publish staged Teal: source graph is required")
	}
	if err := prepareTealPublicationDirectories(graph.StagedTeal); err != nil {
		return TealPublicationResult{}, err
	}
	plans := make([]*tealPublishPlan, 0, len(graph.StagedTeal))
	destinations := make(map[string]string, len(graph.StagedTeal))
	for _, staged := range graph.StagedTeal {
		identity := canonicalIdentity(staged.PublishedLua)
		if previous, exists := destinations[identity]; exists {
			cleanupTealTemps(plans)
			return TealPublicationResult{}, fmt.Errorf("duplicate Teal publication destination %q from %q and %q", staged.PublishedLua, previous, staged.SourcePath)
		}
		destinations[identity] = staged.SourcePath
		plan, err := prepareTealPublishPlan(graph, staged)
		if err != nil {
			cleanupTealTemps(plans)
			return TealPublicationResult{}, err
		}
		plans = append(plans, plan)
	}
	if options.BeforeCommit != nil {
		if err := options.BeforeCommit(); err != nil {
			cleanupTealTemps(plans)
			return TealPublicationResult{}, fmt.Errorf("before Teal publication commit: %w", err)
		}
	}
	for _, plan := range plans {
		if err := verifyTealPlanUnchanged(plan); err != nil {
			cleanupTealTemps(plans)
			return TealPublicationResult{}, err
		}
	}
	defer cleanupTealTemps(plans)
	for index, plan := range plans {
		if err := verifyTealSnapshot(plan.markerPath, plan.oldMarker); err != nil {
			return TealPublicationResult{}, rollbackTealPlans(plans, err)
		}
		if err := verifyTealSnapshot(plan.staged.PublishedLua, plan.oldOutput); err != nil {
			return TealPublicationResult{}, rollbackTealPlans(plans, err)
		}
		// Publish ownership first. Preparation permits this only when the old
		// output is absent, already owned, or byte-identical and adoptable, so a
		// process interruption cannot strand new bytes without recoverable ownership.
		plan.markerTouched = true
		if err := atomicReplaceFile(plan.tempMarker, plan.markerPath); err != nil {
			return TealPublicationResult{}, rollbackTealPlans(plans, fmt.Errorf("publish Teal ownership marker %q: %w", plan.markerPath, err))
		}
		plan.tempMarker = ""
		if err := injectTealPublicationFault(options, index, "marker"); err != nil {
			return TealPublicationResult{}, rollbackTealPlans(plans, err)
		}
		if err := verifyTealSnapshot(plan.staged.PublishedLua, plan.oldOutput); err != nil {
			return TealPublicationResult{}, rollbackTealPlans(plans, err)
		}
		plan.outputTouched = true
		if err := atomicReplaceFile(plan.tempOutput, plan.staged.PublishedLua); err != nil {
			return TealPublicationResult{}, rollbackTealPlans(plans, fmt.Errorf("publish Teal output %q: %w", plan.staged.PublishedLua, err))
		}
		plan.tempOutput = ""
		if err := injectTealPublicationFault(options, index, "output"); err != nil {
			return TealPublicationResult{}, rollbackTealPlans(plans, err)
		}
	}
	result := TealPublicationResult{Outputs: make([]TealPublishedOutput, 0, len(plans))}
	for _, plan := range plans {
		result.Outputs = append(result.Outputs, TealPublishedOutput{
			SourcePath: plan.staged.SourcePath, PublishedLua: plan.staged.PublishedLua,
			MarkerPath: plan.markerPath, Adopted: plan.adopted,
		})
	}
	return result, nil
}

func prepareTealPublishPlan(graph *SourceGraph, staged StagedTeal) (*tealPublishPlan, error) {
	stagedBytes, err := os.ReadFile(staged.EvaluationLua)
	if err != nil {
		return nil, fmt.Errorf("read staged Teal output %q: %w", staged.EvaluationLua, err)
	}
	publishedIdentity := canonicalIdentity(staged.PublishedLua)
	for _, dependency := range graph.Dependencies {
		if canonicalIdentity(dependency.Canonical) == publishedIdentity {
			return nil, fmt.Errorf("Teal output %q for %q collides with explicit %s dependency %q", staged.PublishedLua, staged.SourcePath, dependency.Kind, dependency.Requested)
		}
	}
	markerPath := TealOwnershipMarkerPath(staged.PublishedLua)
	oldOutput, err := snapshotRegularFile(staged.PublishedLua)
	if err != nil {
		return nil, err
	}
	oldMarker, err := snapshotRegularFile(markerPath)
	if err != nil {
		return nil, err
	}
	adopted := false
	if oldMarker.exists {
		var marker tealOwnershipMarker
		if err := json.Unmarshal(oldMarker.data, &marker); err != nil || marker.Version != tealOwnershipVersion || canonicalIdentity(marker.Source) != canonicalIdentity(staged.SourcePath) {
			return nil, fmt.Errorf("Teal output %q has an invalid or foreign ownership marker %q", staged.PublishedLua, markerPath)
		}
	} else if oldOutput.exists {
		if !bytes.Equal(oldOutput.data, stagedBytes) {
			return nil, fmt.Errorf("refusing to overwrite unowned Teal output %q for %q", staged.PublishedLua, staged.SourcePath)
		}
		adopted = true
	}
	markerBytes, err := json.Marshal(tealOwnershipMarker{Version: tealOwnershipVersion, Source: staged.SourcePath})
	if err != nil {
		return nil, err
	}
	markerBytes = append(markerBytes, '\n')
	outputMode := os.FileMode(0o600)
	if oldOutput.exists {
		outputMode = oldOutput.mode
	}
	markerMode := os.FileMode(0o600)
	if oldMarker.exists {
		markerMode = oldMarker.mode
	}
	tempOutput, err := writeTealTemp(staged.PublishedLua, stagedBytes, outputMode)
	if err != nil {
		return nil, err
	}
	tempMarker, err := writeTealTemp(markerPath, markerBytes, markerMode)
	if err != nil {
		_ = os.Remove(tempOutput)
		return nil, err
	}
	return &tealPublishPlan{
		staged: staged, markerPath: markerPath, stagedBytes: stagedBytes, markerBytes: markerBytes,
		oldOutput: oldOutput, oldMarker: oldMarker, tempOutput: tempOutput, tempMarker: tempMarker, adopted: adopted,
	}, nil
}

func prepareTealPublicationDirectories(staged []StagedTeal) error {
	directories := make(map[string]struct{})
	for _, output := range staged {
		directories[filepath.Dir(output.PublishedLua)] = struct{}{}
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	for directory := range directories {
		entries, err := os.ReadDir(directory)
		if err != nil {
			return fmt.Errorf("scan Teal publication directory %q: %w", directory, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasPrefix(entry.Name(), ".cervterm-publish-") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() && info.ModTime().Before(cutoff) {
				if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("remove stale Teal publication temp %q: %w", entry.Name(), err)
				}
			}
		}
	}
	return nil
}

func verifyTealPlanUnchanged(plan *tealPublishPlan) error {
	if err := verifyTealSnapshot(plan.staged.PublishedLua, plan.oldOutput); err != nil {
		return err
	}
	return verifyTealSnapshot(plan.markerPath, plan.oldMarker)
}

func verifyTealSnapshot(path string, expected fileSnapshot) error {
	current, err := snapshotRegularFile(path)
	if err != nil {
		return fmt.Errorf("publication path changed after preparation: %w", err)
	}
	if current.exists != expected.exists {
		return fmt.Errorf("publication path %q changed after preparation", path)
	}
	if !expected.exists {
		return nil
	}
	if !os.SameFile(current.identity, expected.identity) || current.mode != expected.mode || !bytes.Equal(current.data, expected.data) {
		return fmt.Errorf("publication path %q changed after preparation", path)
	}
	return nil
}

func snapshotRegularFile(path string) (fileSnapshot, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileSnapshot{}, nil
	}
	if err != nil {
		return fileSnapshot{}, fmt.Errorf("inspect publication path %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fileSnapshot{}, fmt.Errorf("publication path %q is not a regular file", path)
	}
	multiple, err := fileHasMultipleLinks(path, info)
	if err != nil {
		return fileSnapshot{}, fmt.Errorf("inspect publication path %q link count: %w", path, err)
	}
	if multiple {
		return fileSnapshot{}, fmt.Errorf("publication path %q has multiple hard links", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileSnapshot{}, fmt.Errorf("read publication path %q: %w", path, err)
	}
	return fileSnapshot{exists: true, data: data, mode: info.Mode(), identity: info}, nil
}

func writeTealTemp(destination string, data []byte, mode os.FileMode) (string, error) {
	directory := filepath.Dir(destination)
	file, err := os.CreateTemp(directory, ".cervterm-publish-*")
	if err != nil {
		return "", fmt.Errorf("create publication temp beside %q: %w", destination, err)
	}
	path := file.Name()
	cleanup := func(cause error) (string, error) {
		_ = file.Close()
		_ = os.Remove(path)
		return "", cause
	}
	if err := file.Chmod(mode); err != nil {
		return cleanup(err)
	}
	if _, err := file.Write(data); err != nil {
		return cleanup(err)
	}
	if err := file.Sync(); err != nil {
		return cleanup(err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func injectTealPublicationFault(options TealPublicationOptions, index int, step string) error {
	if options.FaultInjector == nil {
		return nil
	}
	if err := options.FaultInjector(index, step); err != nil {
		return fmt.Errorf("injected Teal publication failure after output %d %s: %w", index, step, err)
	}
	return nil
}

func rollbackTealPlans(plans []*tealPublishPlan, cause error) error {
	var rollbackErrors []error
	for index := len(plans) - 1; index >= 0; index-- {
		plan := plans[index]
		if plan.outputTouched {
			if err := restoreTealSnapshot(plan.staged.PublishedLua, plan.oldOutput); err != nil {
				rollbackErrors = append(rollbackErrors, err)
			}
		}
		if plan.markerTouched {
			if err := restoreTealSnapshot(plan.markerPath, plan.oldMarker); err != nil {
				rollbackErrors = append(rollbackErrors, err)
			}
		}
	}
	if len(rollbackErrors) > 0 {
		return fmt.Errorf("%w; rollback failed: %v", cause, errors.Join(rollbackErrors...))
	}
	return cause
}

func restoreTealSnapshot(path string, snapshot fileSnapshot) error {
	if !snapshot.exists {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("remove newly published path %q: %w", path, err)
		}
		if err := syncParentDirectory(path); err != nil {
			return fmt.Errorf("sync removal of newly published path %q: %w", path, err)
		}
		return nil
	}
	temp, err := writeTealTemp(path, snapshot.data, snapshot.mode)
	if err != nil {
		return err
	}
	defer os.Remove(temp)
	if err := atomicReplaceFile(temp, path); err != nil {
		return fmt.Errorf("restore publication path %q: %w", path, err)
	}
	return nil
}

func cleanupTealTemps(plans []*tealPublishPlan) {
	for _, plan := range plans {
		if plan.tempOutput != "" {
			_ = os.Remove(plan.tempOutput)
		}
		if plan.tempMarker != "" {
			_ = os.Remove(plan.tempMarker)
		}
	}
}
