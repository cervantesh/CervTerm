//go:build glfw

package glfwgl

import (
	"bytes"

	"cervterm/internal/fontglyph"
)

type atlasContextAdmission struct {
	key       atlasFontKey
	context   *atlasFontContext
	victimKey atlasFontKey
	victim    *atlasFontContext
	pins      map[atlasFontKey]struct{}
	owned     bool
}

// useSpec selects an existing raster context or transactionally admits one.
func (a *glyphAtlas) useSpec(spec fontglyph.Spec, textGamma, textDarken float64) (int, int, int, bool) {
	admission, ok := a.prepareSpecWithPins(spec, textGamma, textDarken, a.pinnedContexts)
	if !ok {
		return 0, 0, 0, false
	}
	a.commitContextAdmission(admission)
	return admission.context.cellW, admission.context.cellH, admission.context.baseline, true
}

func (a *glyphAtlas) usePinnedSpec(spec fontglyph.Spec, textGamma, textDarken float64, pins map[atlasFontKey]struct{}) (int, int, int, bool) {
	admission, ok := a.prepareSpecWithPins(spec, textGamma, textDarken, pins)
	if !ok {
		return 0, 0, 0, false
	}
	a.commitContextAdmission(admission)
	return admission.context.cellW, admission.context.cellH, admission.context.baseline, true
}

func (a *glyphAtlas) prepareSpecWithPins(spec fontglyph.Spec, textGamma, textDarken float64, pins map[atlasFontKey]struct{}) (*atlasContextAdmission, bool) {
	if a == nil || a.closed || len(pins) > maxAtlasFontContexts {
		return nil, false
	}
	proposed := cloneContextPins(pins)
	key, err := a.fontKey(spec, textGamma, textDarken)
	if err != nil {
		return nil, false
	}
	if ctx := a.contexts[key]; ctx != nil {
		return &atlasContextAdmission{key: key, context: ctx, pins: proposed}, true
	}
	var victimKey atlasFontKey
	var victim *atlasFontContext
	if len(a.contexts) >= maxAtlasFontContexts {
		victimKey, victim = a.inactiveLRU(proposed)
		if victim == nil {
			return nil, false
		}
	}
	model := a.modelForSpec(spec)
	ctx, err := makeAtlasFontContextWithModel(spec, textGamma, textDarken, model, a.backendFactory)
	if err != nil {
		return nil, false
	}
	return &atlasContextAdmission{key: key, context: ctx, victimKey: victimKey, victim: victim, pins: proposed, owned: true}, true
}

func (a *glyphAtlas) commitContextAdmission(admission *atlasContextAdmission) {
	if admission.owned {
		if admission.victim != nil {
			a.removeContext(admission.victimKey, admission.context.backend)
		}
		a.contexts[admission.key] = admission.context
		admission.owned = false
	}
	a.pinnedContexts = admission.pins
	a.activateContext(admission.context)
	a.prewarmASCII()
}

func (a *glyphAtlas) abortContextAdmission(admission *atlasContextAdmission) {
	if admission == nil || !admission.owned {
		return
	}
	admission.owned = false
	for _, retained := range a.contexts {
		if sameAtlasBackend(admission.context.backend, retained.backend) {
			return
		}
	}
	admission.context.backend.Close()
}

func (a *glyphAtlas) inactiveLRU(pins map[atlasFontKey]struct{}) (atlasFontKey, *atlasFontContext) {
	var selectedKey atlasFontKey
	var selected *atlasFontContext
	for key, ctx := range a.contexts {
		if _, pinned := pins[key]; pinned {
			continue
		}
		if selected == nil || ctx.lastUsed < selected.lastUsed || (ctx.lastUsed == selected.lastUsed && atlasFontKeyLess(key, selectedKey)) {
			selectedKey, selected = key, ctx
		}
	}
	return selectedKey, selected
}

func atlasFontKeyLess(left, right atlasFontKey) bool {
	if comparison := bytes.Compare(left.environment[:], right.environment[:]); comparison != 0 {
		return comparison < 0
	}
	if left.family != right.family {
		return left.family < right.family
	}
	if left.sizeBits != right.sizeBits {
		return left.sizeBits < right.sizeBits
	}
	if left.dpiBits != right.dpiBits {
		return left.dpiBits < right.dpiBits
	}
	return left.textRaster < right.textRaster
}

func (a *glyphAtlas) removeContext(key atlasFontKey, replacement fontglyph.Backend) {
	ctx := a.contexts[key]
	if ctx == nil {
		return
	}
	delete(a.contexts, key)
	if replacement != nil && sameAtlasBackend(ctx.backend, replacement) {
		return
	}
	for _, retained := range a.contexts {
		if sameAtlasBackend(ctx.backend, retained.backend) {
			return
		}
	}
	ctx.backend.Close()
}

// retainContexts commits the visible pin set and keeps inactive contexts in a
// bounded LRU. It rejects oversized visible sets without mutation.
func (a *glyphAtlas) retainContexts(keep map[atlasFontKey]struct{}) bool {
	if a == nil || a.closed || len(keep) > maxAtlasFontContexts {
		return false
	}
	proposed := cloneContextPins(keep)
	for len(a.contexts) > maxAtlasFontContexts {
		key, victim := a.inactiveLRU(proposed)
		if victim == nil {
			return false
		}
		a.removeContext(key, nil)
	}
	a.pinnedContexts = proposed
	return true
}

func cloneContextPins(source map[atlasFontKey]struct{}) map[atlasFontKey]struct{} {
	cloned := make(map[atlasFontKey]struct{}, len(source))
	for key := range source {
		cloned[key] = struct{}{}
	}
	return cloned
}

type atlasPreparedContextInstall struct {
	prepared map[atlasFontKey]*atlasFontContext
	pins     map[atlasFontKey]struct{}
	victims  []atlasFontKey
}

func (a *glyphAtlas) prepareContextInstall(prepared map[atlasFontKey]*atlasFontContext, pins map[atlasFontKey]struct{}) (*atlasPreparedContextInstall, bool) {
	if a == nil || a.closed || len(pins) > maxAtlasFontContexts {
		return nil, false
	}
	missing := 0
	for key := range prepared {
		if a.contexts[key] == nil {
			missing++
		}
	}
	removeCount := max(0, len(a.contexts)+missing-maxAtlasFontContexts)
	plan := &atlasPreparedContextInstall{prepared: prepared, pins: cloneContextPins(pins), victims: make([]atlasFontKey, 0, removeCount)}
	temporaryPins := cloneContextPins(pins)
	for key := range prepared {
		temporaryPins[key] = struct{}{}
	}
	for index := 0; index < removeCount; index++ {
		key, victim := a.inactiveLRU(temporaryPins)
		if victim == nil {
			return nil, false
		}
		plan.victims = append(plan.victims, key)
		temporaryPins[key] = struct{}{}
	}
	return plan, true
}

func (a *glyphAtlas) commitContextInstall(plan *atlasPreparedContextInstall) {
	for _, key := range plan.victims {
		var replacement fontglyph.Backend
		if victim := a.contexts[key]; victim != nil {
			for _, prepared := range plan.prepared {
				if sameAtlasBackend(victim.backend, prepared.backend) {
					replacement = prepared.backend
					break
				}
			}
		}
		a.removeContext(key, replacement)
	}
	for key, ctx := range plan.prepared {
		if existing := a.contexts[key]; existing != nil {
			if !sameAtlasBackend(existing.backend, ctx.backend) {
				ctx.backend.Close()
			}
			continue
		}
		a.contexts[key] = ctx
	}
	a.pinnedContexts = cloneContextPins(plan.pins)
}

func (a *glyphAtlas) installPreparedContexts(prepared map[atlasFontKey]*atlasFontContext, pins map[atlasFontKey]struct{}) bool {
	plan, ok := a.prepareContextInstall(prepared, pins)
	if !ok {
		return false
	}
	a.commitContextInstall(plan)
	return true
}
