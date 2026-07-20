package script

import (
	"errors"
	"path/filepath"
	"sort"

	"cervterm/internal/config"
)

// VersionedLoadError preserves a versioned-load failure and the detached,
// value-free filesystem paths whose existence or content may permit recovery.
type VersionedLoadError struct {
	err          error
	expectations []config.SourceWatchExpectation
}

func (e *VersionedLoadError) Error() string { return e.err.Error() }

func (e *VersionedLoadError) Unwrap() error { return e.err }

// Expectations returns a detached copy of the failure's watch expectations.
func (e *VersionedLoadError) Expectations() []config.SourceWatchExpectation {
	if e == nil {
		return nil
	}
	return append([]config.SourceWatchExpectation(nil), e.expectations...)
}

// FailedWatchExpectations returns structured filesystem evidence from a failed
// versioned load. It never derives paths by parsing error text.
func FailedWatchExpectations(err error) []config.SourceWatchExpectation {
	var failure *VersionedLoadError
	if errors.As(err, &failure) {
		return failure.Expectations()
	}
	return config.SourceGraphFailureExpectations(err)
}

func wrapVersionedLoadError(err error, paths []string, inherited []config.SourceWatchExpectation) error {
	if err == nil {
		return nil
	}
	var existing *VersionedLoadError
	if errors.As(err, &existing) {
		return err
	}
	byPath := make(map[string]config.SourceWatchExpectation, len(paths)+len(inherited))
	add := func(path string) {
		if path == "" {
			return
		}
		absolute, absErr := filepath.Abs(path)
		if absErr != nil {
			return
		}
		cleaned := filepath.Clean(absolute)
		byPath[cleaned] = config.SourceWatchExpectation{Path: cleaned}
	}
	for _, path := range paths {
		add(path)
	}
	for _, expectation := range inherited {
		add(expectation.Path)
	}
	expectations := make([]config.SourceWatchExpectation, 0, len(byPath))
	for _, expectation := range byPath {
		expectations = append(expectations, expectation)
	}
	sort.Slice(expectations, func(i, j int) bool { return expectations[i].Path < expectations[j].Path })
	return &VersionedLoadError{err: err, expectations: expectations}
}
