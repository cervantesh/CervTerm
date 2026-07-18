package script

import (
	"testing"

	"cervterm/internal/config"
	"cervterm/internal/fontdesc"
)

func TestCloneCandidateConfigDetachesFontDescriptors(t *testing.T) {
	original := config.Defaults()
	original.Font.Descriptors = []fontdesc.Descriptor{{Family: "Original"}}
	original.Font.Fallback = []fontdesc.Descriptor{{Family: "Fallback"}}
	original.Font.Rules = []fontdesc.Rule{{Match: fontdesc.RuleMatch{Ranges: []fontdesc.RuneRange{{First: 1, Last: 2}}}, Use: fontdesc.Descriptor{Family: "Rule"}}}
	original.Font.Features = map[string]int{"ss01": 1}
	clone := cloneCandidateConfig(original)
	clone.Font.Descriptors[0].Family = "Mutated"
	clone.Font.Fallback[0].Family = "Mutated"
	clone.Font.Rules[0].Match.Ranges[0].First = 2
	clone.Font.Features["ss01"] = 2
	if original.Font.Descriptors[0].Family != "Original" {
		t.Fatal("candidate clone aliased font descriptors")
	}
	if original.Font.Fallback[0].Family != "Fallback" || original.Font.Rules[0].Match.Ranges[0].First != 1 {
		t.Fatal("candidate clone aliased font fallback/rules")
	}
	if original.Font.Features["ss01"] != 1 {
		t.Fatal("candidate clone aliased font features")
	}
}
