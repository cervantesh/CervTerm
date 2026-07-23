package fontdesc

import "testing"

var phase15FontEnvironment FontEnvironmentKey

func BenchmarkPhase15FontEnvironmentRebuild(b *testing.B) {
	input := FontEnvironmentInput{
		Descriptors: []Descriptor{{Family: "JetBrainsMono Nerd Font"}},
		Fallback: []Descriptor{
			{Family: "CaskaydiaCove Nerd Font"},
			{Family: "Cascadia Mono"},
			{Family: "Consolas"},
		},
		Rules: []Rule{
			{Match: RuleMatch{Class: SymbolClassEmoji}, Use: Descriptor{Family: "Segoe UI Emoji"}},
			{Match: RuleMatch{Class: SymbolClassCJK}, Use: Descriptor{Family: "Microsoft YaHei UI"}},
		},
		Features:      []byte("calt=1,clig=1,liga=1"),
		Metrics:       []byte("line-height=1.08,cell-width=1"),
		BaseSizeBits:  0x4029000000000000,
		PaneZoomBits:  0x3ff0000000000000,
		DPI:           96,
		RasterMode:    "auto",
		GammaBits:     0x3ff0000000000000,
		DarkeningBits: 0,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		key, err := NewFontEnvironmentKey(input)
		if err != nil {
			b.Fatal(err)
		}
		phase15FontEnvironment = key
	}
}
