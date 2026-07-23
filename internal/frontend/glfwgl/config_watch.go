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
	paths       []string
	activePaths []string
	failedPaths []string
	baseline    map[string]configFileObservation
	observed    map[string]configFileObservation
	initialized bool
	generation  uint64
	nextPoll    time.Time
	dirtySince  time.Time
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

func normalizeWatchPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		identity := watchPathIdentity(path)
		if _, ok := seen[identity]; ok {
			continue
		}
		seen[identity] = struct{}{}
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

// watchPathIdentity coalesces Windows case and 8.3 parent-directory aliases
// without resolving the final path component. Keeping that component intact is
// required so two declarative symlink aliases remain independently watched for
// retargeting.
func watchPathIdentity(path string) string {
	clean := filepath.Clean(path)
	if runtime.GOOS != "windows" {
		return clean
	}
	if absolute, err := filepath.Abs(clean); err == nil {
		clean = absolute
	}
	directory := filepath.Dir(clean)
	if canonicalDirectory, err := filepath.EvalSymlinks(directory); err == nil {
		clean = filepath.Join(canonicalDirectory, filepath.Base(clean))
	}
	return strings.ToLower(filepath.Clean(clean))
}

func watchExpectations(paths []string) []config.SourceWatchExpectation {
	normalized := normalizeWatchPaths(paths)
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
	w.activePaths = normalizeWatchPaths(paths)
	w.failedPaths = nil
	w.installPaths(w.activePaths)
}

// acknowledgeFailure replaces the latest failure-only set while preserving the
// last successful graph. It returns whether the failure set changed.
func (w *configWatchState) acknowledgeFailure(expectations []config.SourceWatchExpectation) bool {
	failed := make([]string, 0, len(expectations))
	for _, expectation := range expectations {
		failed = append(failed, expectation.Path)
	}
	failed = normalizeWatchPaths(failed)
	changed := !reflect.DeepEqual(failed, w.failedPaths)
	w.failedPaths = failed
	union := append(append([]string(nil), w.activePaths...), w.failedPaths...)
	w.installPaths(union)
	return changed
}

func (w *configWatchState) installPaths(paths []string) {
	w.paths = normalizeWatchPaths(paths)
	w.baseline = observeWatchPaths(w.paths)
	w.observed = cloneWatchObservations(w.baseline)
	w.initialized = len(w.paths) > 0
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
	return configWatchSnapshot{generation: w.generation, files: observeWatchPaths(w.paths)}
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
	if len(w.paths) == 0 || now.Before(w.nextPoll) {
		return false
	}
	w.nextPoll = now.Add(configPollInterval)
	current := observeWatchPaths(w.paths)
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
