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

// watchPathIdentity coalesces parent-directory aliases (Windows 8.3 short
// names, and OS-level symlinks such as macOS's /var -> /private/var and
// /tmp -> /private/tmp) without resolving the final path component. Keeping
// that component intact is required so two declarative symlink aliases
// remain independently watched for retargeting. Case-folding is Windows-only
// since other supported platforms have case-sensitive filesystems.
//
// Known limitation (pre-existing on Windows, now shared by every platform):
// collapsing the parent directory is deliberately lossy. Two *distinct*
// symlinked directories that resolve to the same real directory (/a -> /real
// and /b -> /real, each containing config.lua) produce one identity, so
// normalizeWatchPaths keeps only whichever alias it saw first and drops the
// other. If the dropped alias is later retargeted to a different directory,
// that retarget is not observed. The guarantee this function does uphold is
// the final-component one exercised by
// TestWatchHashesKeepEveryDeclarativeSymlinkAlias: sibling symlink *files*
// in the same directory stay independently watched. The motivating macOS
// case (/var, /tmp) is unaffected because those system symlinks never
// retarget. Fixing this properly means tracking every original path per
// canonical identity rather than deduplicating to one; that is tracked as a
// separate improvement rather than folded into the macOS support pass.
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
