//go:build glfw

package glfwgl

import (
	"time"

	"cervterm/internal/script"
)

type keyTableMode uint8

const (
	keyTableInactive keyTableMode = iota
	keyTableLeader
	keyTableNamed
)

type keyTableState struct {
	mode     keyTableMode
	table    string
	deadline time.Time
	origin   uint64
}

type keyTableResult struct {
	consume bool
	binding *script.Binding
	origin  uint64
}

func (s *keyTableState) cancel() {
	*s = keyTableState{}
}

func (s keyTableState) active(now time.Time) bool {
	return s.mode != keyTableInactive && (s.deadline.IsZero() || now.Before(s.deadline))
}

func (s *keyTableState) step(set script.BindingSet, spec script.Spec, repeat bool, now time.Time, origin uint64) keyTableResult {
	if s.mode != keyTableInactive && !s.active(now) {
		s.cancel()
	}

	if s.mode == keyTableInactive {
		if set.Leader == nil || spec != set.Leader.Spec {
			return keyTableResult{}
		}
		s.mode = keyTableLeader
		s.deadline = now.Add(time.Duration(set.Leader.TimeoutMS) * time.Millisecond)
		s.origin = origin
		return keyTableResult{consume: true, origin: origin}
	}

	capturedOrigin := s.origin
	if set.Leader != nil && spec == set.Leader.Spec {
		// Key-repeat events for the leader remain inside the transient mode and
		// never leak to root bindings, built-ins, character input, or the PTY.
		return keyTableResult{consume: true, origin: capturedOrigin}
	}
	if spec.Key == "escape" && spec.Mods == 0 {
		s.cancel()
		return keyTableResult{consume: true, origin: capturedOrigin}
	}

	bindings := set.Root
	var table script.KeyTable
	if s.mode == keyTableNamed {
		var ok bool
		table, ok = set.Table(s.table)
		if !ok {
			s.cancel()
			return keyTableResult{consume: true, origin: capturedOrigin}
		}
		bindings = table.Bindings
	}
	binding, ok := findExactBinding(bindings, spec)
	if !ok {
		s.cancel()
		return keyTableResult{consume: true, origin: capturedOrigin}
	}
	if binding.ToTable != "" {
		next, ok := set.Table(binding.ToTable)
		if !ok {
			s.cancel()
			return keyTableResult{consume: true, origin: capturedOrigin}
		}
		s.mode = keyTableNamed
		s.table = next.Name
		s.deadline = now.Add(time.Duration(next.TimeoutMS) * time.Millisecond)
		return keyTableResult{consume: true, origin: capturedOrigin}
	}

	result := keyTableResult{consume: true, binding: &binding, origin: capturedOrigin}
	if s.mode == keyTableLeader || table.OneShot {
		s.cancel()
	} else {
		s.deadline = now.Add(time.Duration(table.TimeoutMS) * time.Millisecond)
	}
	_ = repeat // action trigger policy is applied by the frontend dispatcher.
	return result
}

func findExactBinding(bindings []script.Binding, spec script.Spec) (script.Binding, bool) {
	for _, binding := range bindings {
		if binding.Spec == spec {
			return binding, true
		}
	}
	return script.Binding{}, false
}
