package shape

import (
	"fmt"

	"cervterm/internal/fontdesc"
)

// NewPolicy validates every authored budget and clones fallback policy.
func NewPolicy(config PolicyConfig) (*Policy, error) {
	if config.Environment == (fontdesc.FontEnvironmentKey{}) {
		return nil, fmt.Errorf("font policy has zero environment key")
	}
	if len(config.Descriptors) > fontdesc.MaxPrimaryDescriptors {
		return nil, fmt.Errorf("primary descriptor count %d exceeds %d", len(config.Descriptors), fontdesc.MaxPrimaryDescriptors)
	}
	if len(config.Fallback) > fontdesc.MaxFallbackDescriptors {
		return nil, fmt.Errorf("fallback descriptor count %d exceeds %d", len(config.Fallback), fontdesc.MaxFallbackDescriptors)
	}
	if len(config.Rules) > fontdesc.MaxRules {
		return nil, fmt.Errorf("font rule count %d exceeds %d", len(config.Rules), fontdesc.MaxRules)
	}
	if err := fontdesc.ValidateCanonicalFeaturePayload(config.FeaturePayload); err != nil {
		return nil, fmt.Errorf("font policy features: %w", err)
	}
	if _, err := fontdesc.NewFontEnvironmentKey(fontdesc.FontEnvironmentInput{
		Descriptors: config.Descriptors, Fallback: config.Fallback, Rules: config.Rules, Features: config.FeaturePayload,
	}); err != nil {
		return nil, fmt.Errorf("font policy payload: %w", err)
	}
	policy := &Policy{
		resolver: config.Resolver, environment: config.Environment,
		descriptors: append([]fontdesc.Descriptor(nil), config.Descriptors...),
		fallback:    append([]fontdesc.Descriptor(nil), config.Fallback...),
		rules:       cloneRules(config.Rules), hooks: config.Hooks,
		inflight: make(map[ContentKey]*resolutionCall), resolved: make(map[ContentKey]Plan),
		loadFailed: make(map[fontdesc.ResolvedFaceKey]struct{}), closeDone: make(chan struct{}),
	}
	return policy, nil
}

func cloneRules(rules []fontdesc.Rule) []fontdesc.Rule {
	cloned := make([]fontdesc.Rule, len(rules))
	for index, rule := range rules {
		cloned[index] = rule
		cloned[index].Match.Styles = append([]fontdesc.Style(nil), rule.Match.Styles...)
		cloned[index].Match.Ranges = append([]fontdesc.RuneRange(nil), rule.Match.Ranges...)
	}
	return cloned
}

// Resolve applies rule, primary, authored fallback, and embedded order to one
// complete cluster. Same-key concurrent callers share one callback execution.
func (p *Policy) Resolve(request fontdesc.RequestedFaceStyle, content string) (Plan, bool) {
	if p == nil || content == "" || request > fontdesc.RequestedFaceStyleBoldItalic || p.closing.Load() {
		return Plan{}, false
	}
	key := ContentKey{Request: request, Content: content}
	if hit := p.last.Load(); hit != nil && hit.key == key {
		return hit.plan, true
	}
	p.mu.Lock()
	if p.closed || p.closing.Load() {
		p.mu.Unlock()
		return Plan{}, false
	}
	if selected, ok := p.resolved[key]; ok {
		p.mu.Unlock()
		return selected, true
	}
	if call := p.inflight[key]; call != nil {
		done := call.done
		p.mu.Unlock()
		<-done
		return call.plan, call.ok
	}
	call := &resolutionCall{done: make(chan struct{})}
	p.inflight[key] = call
	p.active.Add(1)
	p.mu.Unlock()

	selected, ok := p.resolveUncached(request, content)

	p.mu.Lock()
	if ok && !p.closed {
		p.rememberLocked(key, selected)
	}
	call.plan, call.ok = selected, ok
	delete(p.inflight, key)
	close(call.done)
	p.mu.Unlock()
	p.active.Done()
	return selected, ok
}

func (p *Policy) resolveUncached(request fontdesc.RequestedFaceStyle, content string) (Plan, bool) {
	requestedTarget, _ := (fontdesc.Descriptor{Family: "requested-style"}).EffectiveTarget(request)
	for index, rule := range p.rules {
		if !rule.Matches(content, requestedTarget) {
			continue
		}
		plans, err := p.resolver.DescriptorPlans(p.environment, []fontdesc.Descriptor{rule.Use}, request, fontdesc.SourceTierRule, uint32(index))
		if err == nil {
			if selected, ok := p.tryPlans(content, plans, nil); ok {
				return selected, true
			}
		}
	}

	primaryPlan, primaryOK := p.primary(request)
	if primaryOK && primaryPlan.Tier == fontdesc.SourceTierPrimary && p.primaryCovers(request, content) {
		return primaryPlan, true
	}
	primaryPlans, _ := p.resolver.DescriptorPlans(p.environment, p.descriptors, request, fontdesc.SourceTierPrimary, 0)
	var skip *fontdesc.ResolvedFaceKey
	if primaryOK {
		skip = &primaryPlan.ResolvedKey
	}
	if selected, ok := p.tryPlans(content, primaryPlans, skip); ok {
		return selected, true
	}
	fallbackPlans, _ := p.resolver.DescriptorPlans(p.environment, p.fallback, request, fontdesc.SourceTierFallback, 0)
	if selected, ok := p.tryPlans(content, fallbackPlans, nil); ok {
		return selected, true
	}
	if primaryOK && primaryPlan.Tier == fontdesc.SourceTierEmbedded {
		return primaryPlan, true
	}
	embedded, err := p.resolver.EmbeddedPlan(p.environment, request)
	if err != nil || p.failed(embedded.ResolvedKey) {
		return Plan{}, false
	}
	if p.loaded(embedded) || p.load(embedded) {
		return embedded, true
	}
	p.recordFailure(embedded.ResolvedKey)
	return Plan{}, false
}

func (p *Policy) tryPlans(content string, plans []Plan, skip *fontdesc.ResolvedFaceKey) (Plan, bool) {
	attempted := make(map[fontdesc.CanonicalFaceID]struct{}, len(plans))
	for _, plan := range plans {
		if skip != nil && plan.ResolvedKey == *skip {
			continue
		}
		if _, duplicate := attempted[plan.CanonicalFaceID]; duplicate {
			continue
		}
		attempted[plan.CanonicalFaceID] = struct{}{}
		if p.failed(plan.ResolvedKey) {
			continue
		}
		alreadyLoaded := p.loaded(plan)
		if !alreadyLoaded && !p.load(plan) {
			p.recordFailure(plan.ResolvedKey)
			continue
		}
		if p.covers(plan, content) {
			return plan, true
		}
		if !alreadyLoaded {
			p.discard(plan)
		}
	}
	return Plan{}, false
}

func (p *Policy) primary(request fontdesc.RequestedFaceStyle) (Plan, bool) {
	if p.hooks.Primary == nil {
		return Plan{}, false
	}
	return p.hooks.Primary(request)
}

func (p *Policy) primaryCovers(request fontdesc.RequestedFaceStyle, content string) bool {
	return p.hooks.PrimaryCovers != nil && p.hooks.PrimaryCovers(request, content)
}

func (p *Policy) loaded(plan Plan) bool {
	return p.hooks.Loaded != nil && p.hooks.Loaded(plan)
}

func (p *Policy) load(plan Plan) bool {
	return p.hooks.Load != nil && p.hooks.Load(plan)
}

func (p *Policy) covers(plan Plan, content string) bool {
	return p.hooks.Covers != nil && p.hooks.Covers(plan, content)
}

func (p *Policy) discard(plan Plan) {
	if p.hooks.Discard != nil {
		p.hooks.Discard(plan)
	}
}

func (p *Policy) failed(key fontdesc.ResolvedFaceKey) bool {
	p.mu.Lock()
	_, failed := p.loadFailed[key]
	p.mu.Unlock()
	return failed
}

func (p *Policy) recordFailure(key fontdesc.ResolvedFaceKey) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	if _, exists := p.loadFailed[key]; exists {
		return
	}
	if len(p.loadFailedRing) < fontdesc.MaxNegativeEntries {
		p.loadFailedRing = append(p.loadFailedRing, key)
	} else {
		victim := p.loadFailedRing[p.loadFailedNext]
		delete(p.loadFailed, victim)
		p.loadFailedRing[p.loadFailedNext] = key
		p.loadFailedNext = (p.loadFailedNext + 1) % len(p.loadFailedRing)
	}
	p.loadFailed[key] = struct{}{}
}

func (p *Policy) rememberLocked(key ContentKey, selected Plan) {
	if _, exists := p.resolved[key]; exists {
		p.resolved[key] = selected
		p.last.Store(&resolutionHit{key: key, plan: selected})
		return
	}
	if len(p.resolvedRing) < fontdesc.MaxNegativeEntries {
		p.resolvedRing = append(p.resolvedRing, key)
	} else {
		victim := p.resolvedRing[p.resolvedNext]
		delete(p.resolved, victim)
		p.resolvedRing[p.resolvedNext] = key
		p.resolvedNext = (p.resolvedNext + 1) % len(p.resolvedRing)
	}
	p.resolved[key] = selected
	p.last.Store(&resolutionHit{key: key, plan: selected})
}

// Stats returns detached bounded-cache accounting.
func (p *Policy) Stats() PolicyStats {
	if p == nil {
		return PolicyStats{Closed: true}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return PolicyStats{
		ResolvedEntries: len(p.resolved), LoadFailedEntries: len(p.loadFailed),
		InFlight: len(p.inflight), Closed: p.closed,
	}
}

// Close rejects new resolutions, waits for callbacks already in flight, then
// leaves no cached plan or authored slice reachable.
func (p *Policy) Close() {
	if p == nil {
		return
	}
	if !p.closing.CompareAndSwap(false, true) {
		<-p.closeDone
		return
	}
	p.mu.Lock()
	p.closed = true
	p.last.Store(nil)
	clear(p.resolved)
	p.resolvedRing = nil
	clear(p.loadFailed)
	p.loadFailedRing = nil
	p.mu.Unlock()

	p.active.Wait()

	p.mu.Lock()
	p.descriptors = nil
	p.fallback = nil
	p.rules = nil
	p.mu.Unlock()
	close(p.closeDone)
}
