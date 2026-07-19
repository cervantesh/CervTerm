package action

import (
	"reflect"
	"strings"
	"testing"
)

func descriptor(id ID) Descriptor {
	return Descriptor{
		ID: id, Label: string(id), Category: CategoryView, Target: TargetOptional,
		Serializable: false, Discoverable: true,
		TriggerPolicy: TriggerPolicy{ConsumePress: true, ExecutePress: true},
	}
}

func TestRegistrySortsAndCopiesDescriptors(t *testing.T) {
	registry, err := newRegistry(
		registration{descriptor: descriptor("z")},
		registration{descriptor: descriptor("a")},
		registration{descriptor: descriptor("m")},
	)
	if err != nil {
		t.Fatal(err)
	}
	got := registry.Descriptors()
	want := []ID{"a", "m", "z"}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("Descriptors()[%d].ID = %q, want %q", i, got[i].ID, id)
		}
	}
	got[0].Label = "mutated"
	again := registry.Descriptors()
	if again[0].Label == "mutated" {
		t.Fatal("Descriptors returned registry-owned storage")
	}
	if _, ok := registry.Lookup("m"); !ok {
		t.Fatal("Lookup did not find registered action")
	}
	if _, ok := registry.Lookup("missing"); ok {
		t.Fatal("Lookup found unregistered action")
	}
}

func TestRegistryRejectsInvalidDescriptors(t *testing.T) {
	executesWithoutConsume := descriptor("bad-policy")
	executesWithoutConsume.TriggerPolicy = TriggerPolicy{ExecuteRepeat: true}
	badTarget := descriptor("bad-target")
	badTarget.Target = "domain"

	missingCodec := descriptor("missing-codec")
	missingCodec.Serializable = true
	tests := []struct {
		name        string
		descriptors []Descriptor
		want        string
	}{
		{name: "duplicate", descriptors: []Descriptor{descriptor("same"), descriptor("same")}, want: "duplicate"},
		{name: "missing id", descriptors: []Descriptor{{Label: "label", Category: CategoryView, Target: TargetOptional}}, want: "ID"},
		{name: "missing label", descriptors: []Descriptor{{ID: "id", Category: CategoryView, Target: TargetOptional}}, want: "label"},
		{name: "missing category", descriptors: []Descriptor{{ID: "id", Label: "label", Target: TargetOptional}}, want: "category"},
		{name: "target", descriptors: []Descriptor{badTarget}, want: "target requirement"},
		{name: "policy", descriptors: []Descriptor{executesWithoutConsume}, want: "without consuming"},
		{name: "codec", descriptors: []Descriptor{missingCodec}, want: "codec operations"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registrations := make([]registration, len(tt.descriptors))
			for i, descriptor := range tt.descriptors {
				registrations[i] = registration{descriptor: descriptor}
			}
			_, err := newRegistry(registrations...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("newRegistry() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestDefaultRegistryContract(t *testing.T) {
	wantIDs := []ID{
		IDActivateCommandPalette, IDActivateLaunchMenu, IDActivateQuickSelect, IDActivateTab, IDActivateTabRelative, IDActivateTabSwitcher, IDCallback, IDClosePane, IDCloseTab, IDCloseWindow, IDCopySelection,
		IDFocusPane, IDFocusWindow, IDMovePane, IDMovePaneToTab, IDMovePaneToWindow, IDMoveTab, IDMoveTabToWindow, IDMultiple, IDNewTab, IDNewWindow, IDPasteClipboard, IDReloadConfig, IDRenameTab, IDResizePane, IDScroll, IDSplitPane, IDSwapPane, IDToggleSearch, IDToggleStats, IDZoom,
	}
	descriptors := DefaultRegistry().Descriptors()
	gotIDs := make([]ID, len(descriptors))
	for i, item := range descriptors {
		gotIDs[i] = item.ID
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("default IDs = %v, want %v", gotIDs, wantIDs)
	}

	callback, _ := DefaultRegistry().Lookup(IDCallback)
	if callback.Serializable || callback.Discoverable {
		t.Fatalf("callback metadata = %#v", callback)
	}
	if callback.TriggerPolicy != callbackPolicy {
		t.Fatalf("callback policy = %#v, want %#v", callback.TriggerPolicy, callbackPolicy)
	}
	reload, _ := DefaultRegistry().Lookup(IDReloadConfig)
	if reload.TriggerPolicy.ConsumeRepeat || reload.TriggerPolicy.ExecuteRepeat {
		t.Fatalf("reload repeat policy = %#v", reload.TriggerPolicy)
	}
	palette, _ := DefaultRegistry().Lookup(IDActivateCommandPalette)
	if !palette.Discoverable || palette.Target != TargetOptional || palette.TriggerPolicy != pressOnly {
		t.Fatalf("palette metadata = %#v", palette)
	}
	quick, _ := DefaultRegistry().Lookup(IDActivateQuickSelect)
	if !quick.Discoverable || quick.Target != TargetPane || quick.TriggerPolicy != pressOnly {
		t.Fatalf("quick select metadata = %#v", quick)
	}
	launch, _ := DefaultRegistry().Lookup(IDActivateLaunchMenu)
	if !launch.Discoverable || launch.Target != TargetPane || launch.TriggerPolicy != pressOnly {
		t.Fatalf("launch metadata = %#v", launch)
	}
	zoom, _ := DefaultRegistry().Lookup(IDZoom)
	if !zoom.TriggerPolicy.ConsumeRepeat || !zoom.TriggerPolicy.ExecuteRepeat {
		t.Fatalf("zoom repeat policy = %#v", zoom.TriggerPolicy)
	}
	resize, _ := DefaultRegistry().Lookup(IDResizePane)
	if !resize.TriggerPolicy.ConsumeRepeat || !resize.TriggerPolicy.ExecuteRepeat {
		t.Fatalf("resize repeat policy = %#v", resize.TriggerPolicy)
	}
	for _, id := range []ID{IDSwapPane, IDMovePane} {
		descriptor, _ := DefaultRegistry().Lookup(id)
		if !descriptor.TriggerPolicy.ConsumeRepeat || descriptor.TriggerPolicy.ExecuteRepeat {
			t.Fatalf("%s repeat policy = %#v", id, descriptor.TriggerPolicy)
		}
	}
}

func TestRegistryDescribesLabeledCallbacks(t *testing.T) {
	unlabeled, err := DefaultRegistry().Describe(Callback{BindingIndex: 1})
	if err != nil {
		t.Fatal(err)
	}
	if unlabeled.Discoverable {
		t.Fatal("unlabeled callback should remain hidden")
	}
	labeled, err := DefaultRegistry().Describe(Callback{BindingIndex: 1, Label: "Open project"})
	if err != nil {
		t.Fatal(err)
	}
	if !labeled.Discoverable || labeled.Label != "Open project" {
		t.Fatalf("labeled callback descriptor = %#v", labeled)
	}
	base, _ := DefaultRegistry().Lookup(IDCallback)
	if base.Discoverable || base.Label != "Lua callback" {
		t.Fatalf("instance description mutated registry: %#v", base)
	}
}
