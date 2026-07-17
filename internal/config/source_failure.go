package config

import (
	"errors"
	"path/filepath"
	"sort"
)

// SourceWatchExpectation identifies one value-free local filesystem path whose
// existence or content may make a failed source-graph candidate load succeed.
type SourceWatchExpectation struct {
	Path string
}

// SourceGraphFailureError preserves a source-graph failure together with a
// detached, normalized set of filesystem watch expectations.
type SourceGraphFailureError struct {
	err          error
	expectations []SourceWatchExpectation
}

func (e *SourceGraphFailureError) Error() string { return e.err.Error() }

func (e *SourceGraphFailureError) Unwrap() error { return e.err }

// Expectations returns a detached copy of the failure's watch expectations.
func (e *SourceGraphFailureError) Expectations() []SourceWatchExpectation {
	if e == nil {
		return nil
	}
	return append([]SourceWatchExpectation(nil), e.expectations...)
}

// SourceGraphFailureExpectations returns structured filesystem evidence from a
// failed source-graph build. It never derives paths by parsing error text.
func SourceGraphFailureExpectations(err error) []SourceWatchExpectation {
	var failure *SourceGraphFailureError
	if !errors.As(err, &failure) {
		return nil
	}
	return failure.Expectations()
}

func wrapSourceGraphFailure(err error, groups ...[]SourceWatchExpectation) error {
	if err == nil {
		return nil
	}
	var existing *SourceGraphFailureError
	if errors.As(err, &existing) {
		return err
	}
	byIdentity := make(map[string]SourceWatchExpectation)
	for _, group := range groups {
		for _, expectation := range group {
			if expectation.Path == "" {
				continue
			}
			absolute, absErr := filepath.Abs(expectation.Path)
			if absErr != nil {
				continue
			}
			cleaned := filepath.Clean(absolute)
			byIdentity[canonicalIdentity(cleaned)] = SourceWatchExpectation{Path: cleaned}
		}
	}
	expectations := make([]SourceWatchExpectation, 0, len(byIdentity))
	for _, expectation := range byIdentity {
		expectations = append(expectations, expectation)
	}
	sort.Slice(expectations, func(i, j int) bool { return expectations[i].Path < expectations[j].Path })
	return &SourceGraphFailureError{err: err, expectations: expectations}
}
