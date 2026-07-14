package script

import (
	"reflect"
	"testing"

	"cervterm/internal/config"
)

func TestStatusSegmentsOrderedMutations(t *testing.T) {
	path := writeScriptConfig(t, `local cervterm = require("cervterm")
return { keys = {
  { key = "a", action = function() cervterm.status("clock", "12:00") end },
  { key = "b", action = function() cervterm.status("branch", "main") end },
  { key = "c", action = function() cervterm.status("clock", "12:01") end },
  { key = "d", action = function() cervterm.status("clock", "12:01") end },
  { key = "e", action = function() cervterm.status("clock", "") end },
} }`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()

	host := &fakeHost{}
	steps := []struct {
		name    string
		binding int
		want    []string
		wantSeq int
		mutates bool
	}{
		{"register first", 0, []string{"12:00"}, 1, true},
		{"register second", 1, []string{"12:00", "main"}, 2, true},
		{"replace in place", 2, []string{"12:01", "main"}, 3, true},
		{"identical no-op", 3, []string{"12:01", "main"}, 3, false},
		{"remove with empty text", 4, []string{"main"}, 4, true},
	}
	lastMutationSeq := 0
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			if err := rt.Dispatch(step.binding, host); err != nil {
				t.Fatalf("Dispatch failed: %v", err)
			}
			if got := rt.StatusSegments(); !reflect.DeepEqual(got, step.want) {
				t.Fatalf("StatusSegments() = %#v, want %#v", got, step.want)
			}
			if got := rt.StatusSeq(); got != step.wantSeq {
				t.Fatalf("StatusSeq() = %d, want %d", got, step.wantSeq)
			}
			if step.mutates && step.wantSeq <= lastMutationSeq {
				t.Fatalf("mutation sequence %d did not increase from %d", step.wantSeq, lastMutationSeq)
			}
			if !step.mutates && step.wantSeq != lastMutationSeq {
				t.Fatalf("no-op sequence changed from %d to %d", lastMutationSeq, step.wantSeq)
			}
			lastMutationSeq = step.wantSeq
		})
	}
}
