package script

import (
	"testing"

	"cervterm/internal/config"
	"cervterm/internal/fontdesc"
)

func TestCloneCandidateConfigDetachesFontDescriptors(t *testing.T) {
	original := config.Defaults()
	original.Font.Descriptors = []fontdesc.Descriptor{{Family: "Original"}}
	clone := cloneCandidateConfig(original)
	clone.Font.Descriptors[0].Family = "Mutated"
	if original.Font.Descriptors[0].Family != "Original" {
		t.Fatal("candidate clone aliased font descriptors")
	}
}
