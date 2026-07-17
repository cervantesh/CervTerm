package fontdesc

import "testing"

func TestBudgetConstants(t *testing.T) {
	got := []int64{
		MaxPrimaryDescriptors, MaxFallbackDescriptors, MaxRules, MaxRangesPerRule,
		MaxTotalRanges, MaxEffectiveFeatures, MaxDescriptorPayloadBytes,
		MaxDiscoveryFiles, MaxDiscoveryFaces, MaxFacesPerFile, MaxParsedFaces,
		MaxParsedBytes, MaxRetainedContexts, MaxNegativeEntries,
	}
	want := []int64{32, 32, 128, 64, 2048, 64, 64 * 1024, 20_000, 65_536, 256, 128, 256 * 1024 * 1024, 64, 8192}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("budget %d = %d, want %d", i, got[i], want[i])
		}
	}
}
