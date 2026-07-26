package shape

import (
	"sync"
	"sync/atomic"

	"cervterm/internal/fontdesc"
)

// ContentKey is the bounded resolution-cache identity.
type ContentKey struct {
	Request fontdesc.RequestedFaceStyle
	Content string
}

// PolicyHooks keep platform/raster construction in the root facade. Every hook
// is invoked without Policy.mu held.
type PolicyHooks struct {
	Primary       func(fontdesc.RequestedFaceStyle) (Plan, bool)
	PrimaryCovers func(fontdesc.RequestedFaceStyle, string) bool
	Loaded        func(Plan) bool
	Load          func(Plan) bool
	Covers        func(Plan, string) bool
	Discard       func(Plan)
}

// PolicyConfig is cloned by NewPolicy so authored slices cannot be mutated
// after installation.
type PolicyConfig struct {
	Resolver       Resolver
	Environment    fontdesc.FontEnvironmentKey
	Descriptors    []fontdesc.Descriptor
	Fallback       []fontdesc.Descriptor
	Rules          []fontdesc.Rule
	FeaturePayload []byte
	Hooks          PolicyHooks
}

type resolutionCall struct {
	done chan struct{}
	plan Plan
	ok   bool
}

type resolutionHit struct {
	key  ContentKey
	plan Plan
}

// Policy owns lazy fallback ordering plus bounded positive and load-failure
// caches. It owns no parsed face or native/raster resource.
type Policy struct {
	resolver    Resolver
	environment fontdesc.FontEnvironmentKey
	descriptors []fontdesc.Descriptor
	fallback    []fontdesc.Descriptor
	rules       []fontdesc.Rule
	hooks       PolicyHooks

	last           atomic.Pointer[resolutionHit]
	mu             sync.Mutex
	active         sync.WaitGroup
	inflight       map[ContentKey]*resolutionCall
	resolved       map[ContentKey]Plan
	resolvedRing   []ContentKey
	resolvedNext   int
	loadFailed     map[fontdesc.ResolvedFaceKey]struct{}
	loadFailedRing []fontdesc.ResolvedFaceKey
	loadFailedNext int
	closing        atomic.Bool
	closeDone      chan struct{}
	closed         bool
}

// PolicyStats is a detached cache/accounting snapshot.
type PolicyStats struct {
	ResolvedEntries   int
	LoadFailedEntries int
	InFlight          int
	Closed            bool
}
