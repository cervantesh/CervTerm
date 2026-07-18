package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"cervterm/internal/fontdesc"

	lua "github.com/yuin/gopher-lua"
)

func TestV2FontFallbackAndRulesDecodeCanonicalValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	writeTestFile(t, path, `return {config_version=2,font={
 fallback={{family=" Fallback  Mono ",weight=500,style="oblique"}},
 rules={{match={weight={min=400,max=700},styles={"italic","normal"},stretch={min=90,max=110},ranges={{first=0xE010,last=0xE020},{first=0xE000,last=0xE018}},class="nerd_font"},use={family=" Rule  Face ",attribute_mode="fixed"}}}
}}`)
	cfg, err := LoadLua(path, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Font.Fallback) != 1 || cfg.Font.Fallback[0].Family != "Fallback Mono" || cfg.Font.Fallback[0].Weight != 500 || cfg.Font.Fallback[0].Style != fontdesc.StyleOblique {
		t.Fatalf("fallback=%#v", cfg.Font.Fallback)
	}
	if len(cfg.Font.Rules) != 1 {
		t.Fatalf("rules=%#v", cfg.Font.Rules)
	}
	rule := cfg.Font.Rules[0]
	if rule.Use.Family != "Rule Face" || rule.Use.AttributeMode != fontdesc.AttributeModeFixed || rule.Match.Class != fontdesc.SymbolClassNerdFont {
		t.Fatalf("rule=%#v", rule)
	}
	if len(rule.Match.Styles) != 2 || rule.Match.Styles[0] != fontdesc.StyleNormal || len(rule.Match.Ranges) != 1 || rule.Match.Ranges[0] != (fontdesc.RuneRange{First: 0xE000, Last: 0xE020}) {
		t.Fatalf("canonical match=%#v", rule.Match)
	}
}

func TestV1IgnoresAdvancedFallbackAndRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	writeTestFile(t, path, `return {font={fallback={{family="Ignored"}},rules={{match={class="emoji"},use={family="Ignored"}}}}}`)
	cfg, err := LoadLua(path, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Font.Fallback != nil || cfg.Font.Rules != nil {
		t.Fatalf("v1 advanced font config leaked: %#v", cfg.Font)
	}
}

func TestComposedV1IgnoresMalformedAdvancedFallbackRules(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"legacy.lua"}}`)
	writeGraphLua(t, dir, "legacy.lua", `return {font={fallback="ignored",rules="ignored"}}`)
	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if cfg.Font.Fallback != nil || cfg.Font.Rules != nil || composition.Document.Has("font.fallback") || composition.Document.Has("font.rules") {
		t.Fatalf("composed v1 advanced font fields leaked: %#v %#v", cfg.Font, composition.Document.Present)
	}
}

func TestV2FontRuleValidationPaths(t *testing.T) {
	entries := []struct{ name, rules, want string }{
		{"not list", `"bad"`, "font.rules"},
		{"too many", ruleList(fontdesc.MaxRules + 1), "at most"},
		{"entry", `{1}`, "font.rules[1]"},
		{"unknown", `{{wat=true,match={class="emoji"},use={family="F"}}}`, "font.rules[1].wat"},
		{"missing match", `{{use={family="F"}}}`, "font.rules[1].match"},
		{"empty match", `{{match={},use={family="F"}}}`, "at least one predicate"},
		{"unknown match", `{{match={wat=true},use={family="F"}}}`, "font.rules[1].match.wat"},
		{"class", `{{match={class="icons"},use={family="F"}}}`, "invalid symbol class"},
		{"range missing", `{{match={ranges={{first=1}}},use={family="F"}}}`, ".last"},
		{"range reverse", `{{match={ranges={{first=2,last=1}}},use={family="F"}}}`, "invalid Unicode range"},
		{"surrogate", `{{match={ranges={{first=0xD800,last=0xD800}}},use={family="F"}}}`, "Unicode scalar"},
		{"weight", `{{match={weight={min=99,max=400}},use={family="F"}}}`, ".weight.min"},
		{"styles", `{{match={styles={"normal","normal"}},use={family="F"}}}`, "duplicate match style"},
		{"missing use", `{{match={class="emoji"}}}`, ".use"},
		{"invalid use", `{{match={class="emoji"},use={family="F",unknown=true}}}`, ".use.unknown"},
		{"normalized range count", ruleWithRanges(fontdesc.MaxRangesPerRule+1, false), "range count"},
	}
	for _, test := range entries {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cervterm.lua")
			writeTestFile(t, path, fmt.Sprintf(`return {config_version=2,font={rules=%s}}`, test.rules))
			_, err := LoadLua(path, Defaults())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestV2FontRuleRangeLimitUsesNormalizedIntervals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	writeTestFile(t, path, fmt.Sprintf(`return {config_version=2,font={rules=%s}}`, ruleWithRanges(fontdesc.MaxRangesPerRule+1, true)))
	cfg, err := LoadLua(path, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Font.Rules) != 1 || len(cfg.Font.Rules[0].Match.Ranges) != 1 {
		t.Fatalf("normalized adjacent ranges = %#v", cfg.Font.Rules)
	}
}

func TestComposeFallbackAndRulesReplaceAndUnsetAtomically(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"low.lua","replace.lua"}}`)
	writeGraphLua(t, dir, "low.lua", `return {config_version=2,font={fallback={{family="A"},{family="B"}},rules={{match={class="emoji"},use={family="A"}}}}}`)
	writeGraphLua(t, dir, "replace.lua", `return {config_version=2,font={fallback={{family="C"}},rules={{match={class="cjk"},use={family="C"}}}}}`)
	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if len(cfg.Font.Fallback) != 1 || cfg.Font.Fallback[0].Family != "C" || len(cfg.Font.Rules) != 1 || cfg.Font.Rules[0].Match.Class != fontdesc.SymbolClassCJK {
		t.Fatalf("composed font config=%#v", cfg.Font)
	}
	assertProvenanceLayers(t, composition.Provenance, "font.fallback", []ProvenanceLayer{LayerInclude, LayerInclude})
	assertProvenanceLayers(t, composition.Provenance, "font.rules", []ProvenanceLayer{LayerInclude, LayerInclude})

	writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"low.lua","reset.lua"}}`)
	writeGraphLua(t, dir, "reset.lua", `return {config_version=2,font={fallback=unset,rules=unset}}`)
	state2, graph2, composition2 := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{})
	defer state2.Close()
	defer graph2.Close()
	cfg = FromDocument(Defaults(), composition2.Document)
	if cfg.Font.Fallback != nil || cfg.Font.Rules != nil {
		t.Fatalf("unset font config=%#v", cfg.Font)
	}
	for _, path := range []string{"font.fallback", "font.rules"} {
		if record, ok := composition2.Provenance.Lookup(path); !ok || !record.Tombstone {
			t.Fatalf("unset provenance %s=%#v ok=%v", path, record, ok)
		}
	}
}

func TestConfigCloneDetachesFallbackRules(t *testing.T) {
	cfg := Defaults()
	cfg.Font.Fallback = []fontdesc.Descriptor{{Family: "Fallback"}}
	cfg.Font.Rules = []fontdesc.Rule{{Match: fontdesc.RuleMatch{Styles: []fontdesc.Style{fontdesc.StyleNormal}, Ranges: []fontdesc.RuneRange{{First: 1, Last: 2}}}, Use: fontdesc.Descriptor{Family: "Rule"}}}
	clone := cfg.Clone()
	clone.Font.Fallback[0].Family = "Mutated"
	clone.Font.Rules[0].Match.Styles[0] = fontdesc.StyleItalic
	clone.Font.Rules[0].Match.Ranges[0].First = 2
	if cfg.Font.Fallback[0].Family != "Fallback" || cfg.Font.Rules[0].Match.Styles[0] != fontdesc.StyleNormal || cfg.Font.Rules[0].Match.Ranges[0].First != 1 {
		t.Fatal("fallback/rule slices aliased")
	}
}

func TestDiagnoseFallbackRulesUsesCanonicalAtomicValues(t *testing.T) {
	cfg := Defaults()
	cfg.Font.Fallback = []fontdesc.Descriptor{{Family: "Fallback"}}
	cfg.Font.Rules = []fontdesc.Rule{{Match: fontdesc.RuleMatch{Class: fontdesc.SymbolClassPowerline, Ranges: []fontdesc.RuneRange{{First: 0xE000, Last: 0xE010}}}, Use: fontdesc.Descriptor{Family: "Rule"}}}
	diagnostic, err := DiagnoseConfig(cfg, Provenance{}, []string{"font.fallback", "font.rules"})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostic.Fields) != 2 || !strings.Contains(diagnostic.Fields[0].Value, `"family":"Fallback"`) || !strings.Contains(diagnostic.Fields[1].Value, `"class":"powerline"`) || !strings.Contains(diagnostic.Fields[1].Value, `"first":57344`) || !strings.Contains(diagnostic.Fields[1].Value, `"family":"Rule"`) {
		t.Fatalf("diagnostic=%#v", diagnostic.Fields)
	}
}

func TestCLIFontFallbackRulesAreTypedAtomicAndProvenanced(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2,font={fallback={{family="Low"}},rules={{match={class="emoji"},use={family="Low"}}}}}`)
	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{CLIOverrides: []CLIOverride{
		{Path: "font.fallback", Value: `[{"family":"High"}]`, ArgumentIndex: 1},
		{Path: "font.rules", Value: `[{"match":{"ranges":[{"first":57344,"last":57599}]},"use":{"family":"Icons"}}]`, ArgumentIndex: 2},
	}})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if len(cfg.Font.Fallback) != 1 || cfg.Font.Fallback[0].Family != "High" || len(cfg.Font.Rules) != 1 || cfg.Font.Rules[0].Use.Family != "Icons" {
		t.Fatalf("CLI font config=%#v", cfg.Font)
	}
	for _, path := range []string{"font.fallback", "font.rules"} {
		record, ok := composition.Provenance.Lookup(path)
		if !ok || record.Winner.Layer != LayerCLI {
			t.Fatalf("CLI provenance %s=%#v ok=%v", path, record, ok)
		}
	}
	state2 := lua.NewState()
	defer state2.Close()
	resolved, err := resolveCLIOverridePath("font.rules")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := decodeCLIOverrideValue(state2, resolved, `[{"match":{"class":"emoji"},"use":null}]`); err == nil {
		t.Fatal("CLI null rule result accepted")
	}
}

func ruleList(count int) string {
	parts := make([]string, count)
	for index := range parts {
		parts[index] = `{match={class="emoji"},use={family="F"}}`
	}
	return `{` + strings.Join(parts, `,`) + `}`
}

func ruleWithRanges(count int, adjacent bool) string {
	parts := make([]string, count)
	for index := range parts {
		first := 1 + index*2
		if adjacent {
			first = 1 + index
		}
		parts[index] = fmt.Sprintf(`{first=%d,last=%d}`, first, first)
	}
	return `{{match={ranges={` + strings.Join(parts, `,`) + `}},use={family="F"}}}`
}
