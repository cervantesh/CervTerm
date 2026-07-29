package mux

import (
	"strings"
	"testing"
)

func TestL302TypedStructuralMutationFixturesFailClosed(t *testing.T) {
	fixtures := map[string]struct {
		source string
		want   []string
	}{
		"atomic mutator aliases through closure": {
			source: `package mux
import "sync/atomic"
type Mux struct{ n atomic.Uint64 }
func bad(m *Mux) {
	store := m.n.Store
	alias := store
	invoke := func() { alias(1) }
	invoke()
}`,
			want: []string{"bad"},
		},
		"function and closure capability locators": {
			source: `package mux
type Mux struct{ n int }
type Owner struct{ mux *Mux }
type processCommandCapability interface{ Shutdown() error }
func locateMux(m *Mux) *Mux { return m }
func locateOwner(o *Owner) *Owner { return o }
func locateProcess(p processCommandCapability) processCommandCapability { return p }
func badClosure(m *Mux) {
	locate := func() *Mux { return m }
	_ = locate
}`,
			want: []string{"locateMux", "locateOwner", "locateProcess", "badClosure"},
		},
		"call return and closure locators": {
			source: `package mux
type Mux struct{ n int }
func locate(m *Mux) *Mux { return m }
func badCall(m *Mux) { locate(m).n++ }
func badClosure(m *Mux) {
	locate := func() *Mux { return m }
	locate().n++
}`,
			want: []string{"locate", "badCall", "badClosure"},
		},
		"chained interface generic aliases": {
			source: `package mux
type Mux struct{ n int }
type box[T any] struct{ value T }
func bad(m *Mux) {
	wrapped := box[any]{value: box[*Mux]{value: m}}
	inner := wrapped.value.(box[*Mux])
	first := inner.value
	second := first
	third := second
	third.n++
}`,
			want: []string{"bad"},
		},
	}
	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			result := l302AnalyzeTypedFixture(t, fixture.source)
			for _, function := range fixture.want {
				if !result.writes[function] {
					t.Fatalf("%s escaped go/types-aware mutation analysis; writes=%v locators=%v unresolved=%v", function, result.writes, result.locators, result.unresolved)
				}
			}
			if len(result.unresolved) != 0 {
				t.Fatalf("typed fixture left unresolved forms: %s", strings.Join(result.unresolved, "; "))
			}
		})
	}
}

func TestL302TypedProductionMutationAnalysisResolvesAndAllowsOnlyExplicitPaths(t *testing.T) {
	result := l302TypedProductionAnalysis(t)
	if len(result.unresolved) != 0 {
		t.Fatalf("production mutation analysis did not resolve all bounded forms: %s", strings.Join(result.unresolved, "; "))
	}
	if len(result.locators) != 0 {
		t.Fatalf("non-allowlisted production capability locators: %v", result.locators)
	}
	if len(result.writes) == 0 {
		t.Fatal("typed production mutation inventory is empty")
	}
}

func TestL302TypedProductionMutationAnalysisFailsClosedOnUnresolvedCalls(t *testing.T) {
	const source = `package mux
	type Mux struct{ n int }
	func bad(m *Mux) { mystery(m).n++ }`
	result := l302AnalyzeUnresolvedProductionFixture(t, source)
	if !result.writes["bad"] {
		t.Fatalf("unresolved production mutation call was accepted: %#v", result)
	}
	if len(result.unresolved) == 0 {
		t.Fatal("unresolved production mutation call was not reported")
	}
	if len(l302CapabilityAllowlist) != 2 || !l302CapabilityAllowlist["internal/mux/mux.go:newMux"] || !l302CapabilityAllowlist["internal/mux/owner.go:NewOwner"] {
		t.Fatalf("capability constructor allowlist is not the exact bounded set: %v", l302CapabilityAllowlist)
	}
	if len(l302DetachedReadAllowlist) == 0 || len(l302DetachedReadAllowlist) > 12 {
		t.Fatalf("detached read allowlist is not small and explicit: %v", l302DetachedReadAllowlist)
	}
}
