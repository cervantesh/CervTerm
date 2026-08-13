//go:build glfw

package glfwgl

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"

	"cervterm/internal/config"
)

const (
	configPollInterval   = 250 * time.Millisecond
	configReloadDebounce = 200 * time.Millisecond
)

type configFileSignature struct {
	modTime int64
	size    int64
	hash    [sha256.Size]byte
}

type configFileObservation struct {
	signature configFileSignature
	exists    bool
}

type configWatchSnapshot struct {
	generation uint64
	files      map[string]configFileObservation
}

type configWatchState struct {
	paths            []string
	watchPaths       []string
	activePaths      []string
	activeWatchPaths []string
	failedPaths      []string
	failedWatchPaths []string
	baseline         map[string]configFileObservation
	observed         map[string]configFileObservation
	initialized      bool
	generation       uint64
	nextPoll         time.Time
	dirtySince       time.Time
}

func newConfigWatchState(paths ...string) configWatchState {
	w := configWatchState{}
	w.acknowledgeSuccess(paths)
	return w
}

func fileObservation(path string) configFileObservation {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return configFileObservation{}
	}
	hash, err := config.FileSourceWatchHash(path)
	if err != nil {
		return configFileObservation{}
	}
	return configFileObservation{exists: true, signature: configFileSignature{modTime: info.ModTime().UnixNano(), size: info.Size(), hash: hash}}
}

func fileSignature(path string) (configFileSignature, bool) {
	observation := fileObservation(path)
	return observation.signature, observation.exists
}

type normalizedWatchPaths struct {
	byIdentity      map[string][]string
	representatives []string
	originals       []string
}

func normalizeWatchPaths(paths []string) normalizedWatchPaths {
	result := normalizedWatchPaths{byIdentity: make(map[string][]string, len(paths))}
	for _, path := range paths {
		if path == "" {
			continue
		}
		identity := watchPathIdentity(path)
		originals := result.byIdentity[identity]
		duplicate := false
		for _, original := range originals {
			if original == path {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		if len(originals) == 0 {
			result.representatives = append(result.representatives, path)
		}
		result.byIdentity[identity] = append(originals, path)
	}
	for _, originals := range result.byIdentity {
		result.originals = append(result.originals, originals...)
	}
	sort.Strings(result.originals)
	sort.Strings(result.representatives)
	return result
}

// watchPathIdentity coalesces parent-directory aliases (Windows 8.3 short
// names, and OS-level symlinks such as macOS's /var -> /private/var and
// /tmp -> /private/tmp) without resolving the final path component. Keeping
// that component intact is required so two declarative symlink aliases
// remain independently watched for retargeting. Case-folding is Windows-only
// since other supported platforms have case-sensitive filesystems.
//
// normalizeWatchPaths groups aliases by this identity while retaining every
// original path. That lets stable OS aliases share an identity without losing
// a user-created alias whose parent directory is later retargeted.
//
// Also note: EvalSymlinks fails for a parent directory that does not exist
// yet, in which case no canonicalization happens and aliases stay distinct.
func watchPathIdentity(path string) string {
	clean := filepath.Clean(path)
	if absolute, err := filepath.Abs(clean); err == nil {
		clean = absolute
	}
	directory := filepath.Dir(clean)
	if canonicalDirectory, err := filepath.EvalSymlinks(directory); err == nil {
		clean = filepath.Join(canonicalDirectory, filepath.Base(clean))
	}
	clean = filepath.Clean(clean)
	if runtime.GOOS == "windows" {
		return strings.ToLower(clean)
	}
	return clean
}

func watchExpectations(paths []string) []config.SourceWatchExpectation {
	normalized := normalizeWatchPaths(paths).originals
	expectations := make([]config.SourceWatchExpectation, 0, len(normalized))
	for _, path := range normalized {
		expectations = append(expectations, config.SourceWatchExpectation{Path: path})
	}
	return expectations
}

func observeWatchPaths(paths []string) map[string]configFileObservation {
	observed := make(map[string]configFileObservation, len(paths))
	for _, path := range paths {
		observed[path] = fileObservation(path)
	}
	return observed
}

func (w *configWatchState) acknowledge(paths []string) { w.acknowledgeSuccess(paths) }

func (w *configWatchState) acknowledgeSuccess(paths []string) {
	normalized := normalizeWatchPaths(paths)
	w.activePaths = normalized.representatives
	w.activeWatchPaths = normalized.originals
	w.failedPaths = nil
	w.failedWatchPaths = nil
	w.installPaths(w.activeWatchPaths)
}

// acknowledgeFailure replaces the latest failure-only set while preserving the
// last successful graph. It returns whether the failure set changed.
func (w *configWatchState) acknowledgeFailure(expectations []config.SourceWatchExpectation) bool {
	failed := make([]string, 0, len(expectations))
	for _, expectation := range expectations {
		failed = append(failed, expectation.Path)
	}
	normalized := normalizeWatchPaths(failed)
	changed := !reflect.DeepEqual(normalized.originals, w.failedWatchPaths)
	w.failedPaths = normalized.representatives
	w.failedWatchPaths = normalized.originals
	union := append(append([]string(nil), w.activeWatchPaths...), w.failedWatchPaths...)
	w.installPaths(union)
	return changed
}

func (w *configWatchState) installPaths(paths []string) {
	normalized := normalizeWatchPaths(paths)
	w.paths = normalized.representatives
	w.watchPaths = normalized.originals
	w.baseline = observeWatchPaths(w.watchPaths)
	w.observed = cloneWatchObservations(w.baseline)
	w.initialized = len(w.watchPaths) > 0
	w.generation++
	w.dirtySince = time.Time{}
}

func cloneWatchObservations(source map[string]configFileObservation) map[string]configFileObservation {
	clone := make(map[string]configFileObservation, len(source))
	for path, observation := range source {
		clone[path] = observation
	}
	return clone
}

func (w *configWatchState) snapshot() configWatchSnapshot {
	return configWatchSnapshot{generation: w.generation, files: observeWatchPaths(w.watchPaths)}
}

func (w *configWatchState) changedSince(snapshot configWatchSnapshot) bool {
	return !reflect.DeepEqual(snapshot.files, observeWatchPaths(mapsKeys(snapshot.files)))
}

func watchHashesChanged(hashes map[string][32]byte) bool {
	for path, expected := range hashes {
		observation := fileObservation(path)
		if !observation.exists || observation.signature.hash != expected {
			return true
		}
	}
	return false
}

func configWatchSnapshotsDiffer(left, right configWatchSnapshot) bool {
	return left.generation != right.generation || !reflect.DeepEqual(left.files, right.files)
}

func mapsKeys(values map[string]configFileObservation) []string {
	paths := make([]string, 0, len(values))
	for path := range values {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// poll reports one debounced change across the complete active source graph.
// Missing files are observations too, so deletion/rename triggers a reload.
func (w *configWatchState) poll(now time.Time) bool {
	if len(w.watchPaths) == 0 || now.Before(w.nextPoll) {
		return false
	}
	w.nextPoll = now.Add(configPollInterval)
	current := observeWatchPaths(w.watchPaths)
	if !w.initialized {
		w.baseline, w.observed, w.initialized = current, cloneWatchObservations(current), true
		return false
	}
	if !reflect.DeepEqual(current, w.observed) {
		w.observed = current
		if reflect.DeepEqual(current, w.baseline) {
			w.dirtySince = time.Time{}
		} else {
			w.dirtySince = now
		}
		return false
	}
	if !w.dirtySince.IsZero() && now.Sub(w.dirtySince) >= configReloadDebounce {
		w.baseline = cloneWatchObservations(current)
		w.dirtySince = time.Time{}
		w.generation++
		return true
	}
	return false
}
