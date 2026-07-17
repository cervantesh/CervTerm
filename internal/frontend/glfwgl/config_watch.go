//go:build glfw

package glfwgl

import (
	"crypto/sha256"
	"os"
	"reflect"
	"sort"
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
	baseline    map[string]configFileObservation
	observed    map[string]configFileObservation
	initialized bool
	generation  uint64
	nextPoll    time.Time
	dirtySince  time.Time
}

func newConfigWatchState(paths ...string) configWatchState {
	w := configWatchState{}
	w.acknowledge(paths)
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
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func observeWatchPaths(paths []string) map[string]configFileObservation {
	observed := make(map[string]configFileObservation, len(paths))
	for _, path := range paths {
		observed[path] = fileObservation(path)
	}
	return observed
}

func (w *configWatchState) acknowledge(paths []string) {
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
