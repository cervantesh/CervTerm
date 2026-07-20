package fontdesc

import "testing"

func TestDescriptorNormalizationDefaultsAndPresence(t *testing.T) {
	d := (Descriptor{Family: "  JetBrainsMono   Nerd Font  ", CollectionIndex: SomeCollectionIndex(0)}).Normalized()
	if d.Family != "JetBrainsMono Nerd Font" || d.Weight != 400 || d.Style != StyleNormal || d.Stretch != 100 || d.AttributeMode != AttributeModeAugment {
		t.Fatalf("unexpected normalized descriptor: %+v", d)
	}
	if !d.CollectionIndex.Present || d.CollectionIndex.Value != 0 {
		t.Fatalf("explicit collection index zero lost: %+v", d.CollectionIndex)
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestDescriptorValidation(t *testing.T) {
	valid := Descriptor{Family: "Face"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("default descriptor rejected: %v", err)
	}
	for name, mutate := range map[string]func(*Descriptor){
		"family":              func(d *Descriptor) { d.Family = " " },
		"weight-low":          func(d *Descriptor) { d.Weight = 99 },
		"weight-high":         func(d *Descriptor) { d.Weight = 901 },
		"style":               func(d *Descriptor) { d.Style = "roman" },
		"stretch-low":         func(d *Descriptor) { d.Stretch = 49 },
		"stretch-high":        func(d *Descriptor) { d.Stretch = 201 },
		"attribute-mode":      func(d *Descriptor) { d.AttributeMode = "replace" },
		"collection-conflict": func(d *Descriptor) { d.CollectionFace = "Regular"; d.CollectionIndex = SomeCollectionIndex(0) },
		"collection-overflow": func(d *Descriptor) { d.CollectionIndex = SomeCollectionIndex(MaxFacesPerFile) },
	} {
		t.Run(name, func(t *testing.T) {
			d := valid
			mutate(&d)
			if err := d.Validate(); err == nil {
				t.Fatalf("invalid descriptor accepted: %+v", d)
			}
		})
	}
}
