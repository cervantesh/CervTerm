package action

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

type Category string

const (
	CategoryClipboard Category = "clipboard"
	CategorySearch    Category = "search"
	CategoryView      Category = "view"
	CategoryConfig    Category = "config"
	CategoryCommand   Category = "command"
	CategoryPane      Category = "pane"
	CategorySequence  Category = "sequence"
	CategoryScript    Category = "script"
)

type TargetRequirement string

const (
	TargetOptional TargetRequirement = "optional"
	TargetPane     TargetRequirement = "pane"
)

type TriggerPolicy struct {
	ConsumePress  bool
	ExecutePress  bool
	ConsumeRepeat bool
	ExecuteRepeat bool
}

var (
	pressAndRepeat = TriggerPolicy{ConsumePress: true, ExecutePress: true, ConsumeRepeat: true, ExecuteRepeat: true}
	pressOnly      = TriggerPolicy{ConsumePress: true, ExecutePress: true}
	callbackPolicy = TriggerPolicy{ConsumePress: true, ExecutePress: true, ConsumeRepeat: true}
)

type Descriptor struct {
	ID            ID
	Label         string
	Category      Category
	Target        TargetRequirement
	Serializable  bool
	Discoverable  bool
	TriggerPolicy TriggerPolicy
}

type codecOps struct {
	encode func(Action, *Codec, int, *codecBudget) (json.RawMessage, error)
	decode func(json.RawMessage, *Codec, int, *codecBudget) (Action, error)
}

type registration struct {
	descriptor Descriptor
	codec      codecOps
}

type Registry struct {
	ordered []Descriptor
	byID    map[ID]registration
}

func newRegistry(registrations ...registration) (*Registry, error) {
	registry := &Registry{
		ordered: make([]Descriptor, 0, len(registrations)),
		byID:    make(map[ID]registration, len(registrations)),
	}
	for _, item := range registrations {
		if err := validateRegistration(item); err != nil {
			return nil, err
		}
		id := item.descriptor.ID
		if _, exists := registry.byID[id]; exists {
			return nil, fmt.Errorf("duplicate action descriptor %q", id)
		}
		registry.byID[id] = item
		registry.ordered = append(registry.ordered, item.descriptor)
	}
	sort.Slice(registry.ordered, func(i, j int) bool { return registry.ordered[i].ID < registry.ordered[j].ID })
	return registry, nil
}

func validateRegistration(item registration) error {
	descriptor := item.descriptor
	if descriptor.ID == "" {
		return fmt.Errorf("action descriptor ID is required")
	}
	if descriptor.Label == "" {
		return fmt.Errorf("action descriptor %q label is required", descriptor.ID)
	}
	if descriptor.Category == "" {
		return fmt.Errorf("action descriptor %q category is required", descriptor.ID)
	}
	if descriptor.Target != TargetOptional && descriptor.Target != TargetPane {
		return fmt.Errorf("action descriptor %q target requirement %q is invalid", descriptor.ID, descriptor.Target)
	}
	policy := descriptor.TriggerPolicy
	if policy.ExecutePress && !policy.ConsumePress {
		return fmt.Errorf("action descriptor %q executes press without consuming it", descriptor.ID)
	}
	if policy.ExecuteRepeat && !policy.ConsumeRepeat {
		return fmt.Errorf("action descriptor %q executes repeat without consuming it", descriptor.ID)
	}
	if descriptor.Serializable && (item.codec.encode == nil || item.codec.decode == nil) {
		return fmt.Errorf("serializable action descriptor %q requires codec operations", descriptor.ID)
	}
	return nil
}

func (r *Registry) lookupRegistration(id ID) (registration, bool) {
	if r == nil {
		return registration{}, false
	}
	item, ok := r.byID[id]
	return item, ok
}

func (r *Registry) Lookup(id ID) (Descriptor, bool) {
	item, ok := r.lookupRegistration(id)
	return item.descriptor, ok
}

// Describe returns instance metadata. A labeled callback becomes discoverable
// without changing the registry's runtime-local, non-serializable contract.
func (r *Registry) Describe(action Action) (Descriptor, error) {
	id, err := actionIdentity(action)
	if err != nil {
		return Descriptor{}, err
	}
	descriptor, ok := r.Lookup(id)
	if !ok {
		return Descriptor{}, fmt.Errorf("action %q is not registered", id)
	}
	if callback, ok := action.(Callback); ok && callback.Label != "" {
		descriptor.Label = callback.Label
		descriptor.Discoverable = true
	}
	return descriptor, nil
}

func (r *Registry) Descriptors() []Descriptor {
	if r == nil {
		return nil
	}
	out := make([]Descriptor, len(r.ordered))
	copy(out, r.ordered)
	return out
}

func metadata(id ID, label string, category Category, target TargetRequirement, serializable, discoverable bool, policy TriggerPolicy) Descriptor {
	return Descriptor{ID: id, Label: label, Category: category, Target: target, Serializable: serializable, Discoverable: discoverable, TriggerPolicy: policy}
}

var (
	defaultRegistryOnce sync.Once
	defaultRegistry     *Registry
)

func mustRegistry(registrations ...registration) *Registry {
	registry, err := newRegistry(registrations...)
	if err != nil {
		panic(err)
	}
	return registry
}

func DefaultRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = mustRegistry(
			registration{descriptor: metadata(IDCopySelection, "Copy selection", CategoryClipboard, TargetPane, true, true, pressAndRepeat), codec: simpleCodec(CopySelection{})},
			registration{descriptor: metadata(IDPasteClipboard, "Paste clipboard", CategoryClipboard, TargetPane, true, true, pressAndRepeat), codec: simpleCodec(PasteClipboard{})},
			registration{descriptor: metadata(IDToggleSearch, "Toggle search", CategorySearch, TargetPane, true, true, pressAndRepeat), codec: simpleCodec(ToggleSearch{})},
			registration{descriptor: metadata(IDActivateCommandPalette, "Activate command palette", CategoryCommand, TargetOptional, true, true, pressOnly), codec: simpleCodec(ActivateCommandPalette{})},
			registration{descriptor: metadata(IDActivateQuickSelect, "Activate quick select", CategoryCommand, TargetPane, true, true, pressOnly), codec: simpleCodec(ActivateQuickSelect{})},
			registration{descriptor: metadata(IDToggleStats, "Toggle statistics", CategoryView, TargetOptional, true, true, pressOnly), codec: simpleCodec(ToggleStats{})},
			registration{descriptor: metadata(IDScroll, "Scroll", CategoryView, TargetPane, true, true, pressAndRepeat), codec: scrollCodec},
			registration{descriptor: metadata(IDZoom, "Zoom", CategoryView, TargetPane, true, true, pressAndRepeat), codec: zoomCodec},
			registration{descriptor: metadata(IDReloadConfig, "Reload configuration", CategoryConfig, TargetOptional, true, true, pressOnly), codec: simpleCodec(ReloadConfig{})},
			registration{descriptor: metadata(IDSplitPane, "Split pane", CategoryPane, TargetPane, true, true, pressAndRepeat), codec: splitPaneCodec},
			registration{descriptor: metadata(IDFocusPane, "Focus pane", CategoryPane, TargetPane, true, true, pressAndRepeat), codec: focusPaneCodec},
			registration{descriptor: metadata(IDClosePane, "Close pane", CategoryPane, TargetPane, true, true, pressAndRepeat), codec: simpleCodec(ClosePane{})},
			registration{descriptor: metadata(IDResizePane, "Resize pane", CategoryPane, TargetPane, true, true, pressAndRepeat), codec: resizePaneCodec},
			registration{descriptor: metadata(IDSwapPane, "Swap pane", CategoryPane, TargetPane, true, true, callbackPolicy), codec: swapPaneCodec},
			registration{descriptor: metadata(IDMovePane, "Move pane", CategoryPane, TargetPane, true, true, callbackPolicy), codec: movePaneCodec},
			registration{descriptor: metadata(IDMultiple, "Run multiple actions", CategorySequence, TargetOptional, true, true, callbackPolicy), codec: multipleCodec},
			registration{descriptor: metadata(IDCallback, "Lua callback", CategoryScript, TargetPane, false, false, callbackPolicy)},
		)
	})
	return defaultRegistry
}
