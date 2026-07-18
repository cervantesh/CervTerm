package config

import (
	"fmt"
	"strings"
	"unicode"

	"cervterm/internal/fontdesc"

	lua "github.com/yuin/gopher-lua"
)

func validateDescriptorList(source, path string, value lua.LValue) error {
	table, ok := value.(*lua.LTable)
	if !ok {
		return typeError(source, path, KindDescriptorList, value)
	}
	if err := validateDenseArray(source, path, table); err != nil {
		return err
	}
	limit := fontdesc.MaxPrimaryDescriptors
	if path == "font.fallback" || strings.HasSuffix(path, ".fallback") {
		limit = fontdesc.MaxFallbackDescriptors
	}
	if table.Len() > limit {
		return documentError(source, path, "must contain at most %d entries, got %d", limit, table.Len())
	}
	for index := 1; index <= table.Len(); index++ {
		entryPath := fmt.Sprintf("%s[%d]", path, index)
		if _, err := parseDescriptorValue(source, entryPath, table.RawGetInt(index)); err != nil {
			return err
		}
	}
	return nil
}

func parseDescriptorValue(source, path string, value lua.LValue) (fontdesc.Descriptor, error) {
	entry, ok := value.(*lua.LTable)
	if !ok {
		return fontdesc.Descriptor{}, typeError(source, path, KindTable, value)
	}
	keys, err := strictStringKeys(source, path, entry)
	if err != nil {
		return fontdesc.Descriptor{}, err
	}
	for _, key := range keys {
		switch key {
		case "family", "collection_face", "collection_index", "weight", "style", "stretch", "attribute_mode":
		default:
			return fontdesc.Descriptor{}, documentError(source, joinPath(path, key), "unknown field")
		}
	}

	rawFamily := entry.RawGetString("family")
	family, ok := rawFamily.(lua.LString)
	if !ok {
		if rawFamily == lua.LNil {
			return fontdesc.Descriptor{}, documentError(source, path+".family", "is required")
		}
		return fontdesc.Descriptor{}, typeError(source, path+".family", KindString, rawFamily)
	}
	descriptor := fontdesc.Descriptor{Family: string(family)}
	if strings.TrimSpace(descriptor.Family) == "" {
		return fontdesc.Descriptor{}, documentError(source, path+".family", "must not be empty")
	}

	rawCollectionFace := entry.RawGetString("collection_face")
	collectionFacePresent := rawCollectionFace != lua.LNil
	if collectionFacePresent {
		face, ok := rawCollectionFace.(lua.LString)
		if !ok {
			return fontdesc.Descriptor{}, typeError(source, path+".collection_face", KindString, rawCollectionFace)
		}
		descriptor.CollectionFace = string(face)
		if strings.TrimSpace(descriptor.CollectionFace) == "" {
			return fontdesc.Descriptor{}, documentError(source, path+".collection_face", "must not be empty")
		}
	}
	if raw := entry.RawGetString("collection_index"); raw != lua.LNil {
		if err := validateInteger(source, path+".collection_index", raw); err != nil {
			return fontdesc.Descriptor{}, err
		}
		parsed := int(raw.(lua.LNumber))
		if parsed < 0 || parsed >= fontdesc.MaxFacesPerFile {
			return fontdesc.Descriptor{}, documentError(source, path+".collection_index", "must be between 0 and %d", fontdesc.MaxFacesPerFile-1)
		}
		if collectionFacePresent {
			return fontdesc.Descriptor{}, documentError(source, path+".collection_index", "cannot be set with collection_face")
		}
		descriptor.CollectionIndex = fontdesc.SomeCollectionIndex(uint32(parsed))
	}
	if raw := entry.RawGetString("weight"); raw != lua.LNil {
		if err := validateInteger(source, path+".weight", raw); err != nil {
			return fontdesc.Descriptor{}, err
		}
		descriptor.Weight = int(raw.(lua.LNumber))
		if descriptor.Weight < 100 || descriptor.Weight > 900 {
			return fontdesc.Descriptor{}, documentError(source, path+".weight", "must be between 100 and 900")
		}
	}
	if raw := entry.RawGetString("style"); raw != lua.LNil {
		style, ok := raw.(lua.LString)
		if !ok {
			return fontdesc.Descriptor{}, typeError(source, path+".style", KindString, raw)
		}
		descriptor.Style = fontdesc.Style(style)
		switch descriptor.Style {
		case fontdesc.StyleNormal, fontdesc.StyleItalic, fontdesc.StyleOblique:
		default:
			return fontdesc.Descriptor{}, documentError(source, path+".style", "must be normal, italic, or oblique")
		}
	}
	if raw := entry.RawGetString("stretch"); raw != lua.LNil {
		if err := validateInteger(source, path+".stretch", raw); err != nil {
			return fontdesc.Descriptor{}, err
		}
		descriptor.Stretch = int(raw.(lua.LNumber))
		if descriptor.Stretch < 50 || descriptor.Stretch > 200 {
			return fontdesc.Descriptor{}, documentError(source, path+".stretch", "must be between 50 and 200")
		}
	}
	if raw := entry.RawGetString("attribute_mode"); raw != lua.LNil {
		mode, ok := raw.(lua.LString)
		if !ok {
			return fontdesc.Descriptor{}, typeError(source, path+".attribute_mode", KindString, raw)
		}
		descriptor.AttributeMode = fontdesc.AttributeMode(mode)
		switch descriptor.AttributeMode {
		case fontdesc.AttributeModeAugment, fontdesc.AttributeModeFixed:
		default:
			return fontdesc.Descriptor{}, documentError(source, path+".attribute_mode", "must be augment or fixed")
		}
	}
	normalized, err := descriptor.Normalize()
	if err != nil {
		return fontdesc.Descriptor{}, documentError(source, path, "%v", err)
	}
	return normalized, nil
}

func validateFontRuleList(source, path string, value lua.LValue) error {
	table, ok := value.(*lua.LTable)
	if !ok {
		return typeError(source, path, KindFontRuleList, value)
	}
	if err := validateDenseArray(source, path, table); err != nil {
		return err
	}
	if table.Len() > fontdesc.MaxRules {
		return documentError(source, path, "must contain at most %d entries, got %d", fontdesc.MaxRules, table.Len())
	}
	totalRanges := 0
	for index := 1; index <= table.Len(); index++ {
		entryPath := fmt.Sprintf("%s[%d]", path, index)
		rule, _, err := parseFontRuleValue(source, entryPath, table.RawGetInt(index))
		if err != nil {
			return err
		}
		normalized, err := rule.Normalize()
		if err != nil {
			return documentError(source, entryPath, "%v", err)
		}
		totalRanges += len(normalized.Match.Ranges)
		if totalRanges > fontdesc.MaxTotalRanges {
			return documentError(source, path, "must contain at most %d normalized ranges", fontdesc.MaxTotalRanges)
		}
	}
	return nil
}

func parseFontRuleValue(source, path string, value lua.LValue) (fontdesc.Rule, int, error) {
	entry, ok := value.(*lua.LTable)
	if !ok {
		return fontdesc.Rule{}, 0, typeError(source, path, KindTable, value)
	}
	keys, err := strictStringKeys(source, path, entry)
	if err != nil {
		return fontdesc.Rule{}, 0, err
	}
	for _, key := range keys {
		if key != "match" && key != "use" {
			return fontdesc.Rule{}, 0, documentError(source, joinPath(path, key), "unknown field")
		}
	}
	rawMatch := entry.RawGetString("match")
	matchTable, ok := rawMatch.(*lua.LTable)
	if !ok {
		if rawMatch == lua.LNil {
			return fontdesc.Rule{}, 0, documentError(source, path+".match", "is required")
		}
		return fontdesc.Rule{}, 0, typeError(source, path+".match", KindTable, rawMatch)
	}
	match, rawRanges, err := parseFontRuleMatch(source, path+".match", matchTable)
	if err != nil {
		return fontdesc.Rule{}, 0, err
	}
	rawUse := entry.RawGetString("use")
	if rawUse == lua.LNil {
		return fontdesc.Rule{}, 0, documentError(source, path+".use", "is required")
	}
	use, err := parseDescriptorValue(source, path+".use", rawUse)
	if err != nil {
		return fontdesc.Rule{}, 0, err
	}
	return fontdesc.Rule{Match: match, Use: use}, rawRanges, nil
}

func parseFontRuleMatch(source, path string, table *lua.LTable) (fontdesc.RuleMatch, int, error) {
	keys, err := strictStringKeys(source, path, table)
	if err != nil {
		return fontdesc.RuleMatch{}, 0, err
	}
	for _, key := range keys {
		switch key {
		case "weight", "styles", "stretch", "ranges", "class":
		default:
			return fontdesc.RuleMatch{}, 0, documentError(source, joinPath(path, key), "unknown field")
		}
	}
	var match fontdesc.RuleMatch
	if raw := table.RawGetString("weight"); raw != lua.LNil {
		value, err := parseRuleIntegerRange(source, path+".weight", raw, 100, 900)
		if err != nil {
			return fontdesc.RuleMatch{}, 0, err
		}
		match.Weight = value
	}
	if raw := table.RawGetString("stretch"); raw != lua.LNil {
		value, err := parseRuleIntegerRange(source, path+".stretch", raw, 50, 200)
		if err != nil {
			return fontdesc.RuleMatch{}, 0, err
		}
		match.Stretch = value
	}
	if raw := table.RawGetString("styles"); raw != lua.LNil {
		styles, ok := raw.(*lua.LTable)
		if !ok {
			return fontdesc.RuleMatch{}, 0, typeError(source, path+".styles", KindStringList, raw)
		}
		if err := validateDenseArray(source, path+".styles", styles); err != nil {
			return fontdesc.RuleMatch{}, 0, err
		}
		match.Styles = make([]fontdesc.Style, styles.Len())
		for index := range match.Styles {
			value, ok := styles.RawGetInt(index + 1).(lua.LString)
			if !ok {
				return fontdesc.RuleMatch{}, 0, typeError(source, fmt.Sprintf("%s.styles[%d]", path, index+1), KindString, styles.RawGetInt(index+1))
			}
			match.Styles[index] = fontdesc.Style(value)
		}
	}
	if raw := table.RawGetString("class"); raw != lua.LNil {
		value, ok := raw.(lua.LString)
		if !ok {
			return fontdesc.RuleMatch{}, 0, typeError(source, path+".class", KindString, raw)
		}
		match.Class = fontdesc.SymbolClass(value)
	}
	rawRangeCount := 0
	if raw := table.RawGetString("ranges"); raw != lua.LNil {
		ranges, ok := raw.(*lua.LTable)
		if !ok {
			return fontdesc.RuleMatch{}, 0, typeError(source, path+".ranges", KindTable, raw)
		}
		if err := validateDenseArray(source, path+".ranges", ranges); err != nil {
			return fontdesc.RuleMatch{}, 0, err
		}
		rawRangeCount = ranges.Len()
		match.Ranges = make([]fontdesc.RuneRange, ranges.Len())
		for index := range match.Ranges {
			entryPath := fmt.Sprintf("%s.ranges[%d]", path, index+1)
			rawEntry := ranges.RawGetInt(index + 1)
			entry, ok := rawEntry.(*lua.LTable)
			if !ok {
				return fontdesc.RuleMatch{}, 0, typeError(source, entryPath, KindTable, rawEntry)
			}
			keys, err := strictStringKeys(source, entryPath, entry)
			if err != nil {
				return fontdesc.RuleMatch{}, 0, err
			}
			for _, key := range keys {
				if key != "first" && key != "last" {
					return fontdesc.RuleMatch{}, 0, documentError(source, joinPath(entryPath, key), "unknown field")
				}
			}
			first, err := requiredUnicodeScalar(source, entryPath+".first", entry.RawGetString("first"))
			if err != nil {
				return fontdesc.RuleMatch{}, 0, err
			}
			last, err := requiredUnicodeScalar(source, entryPath+".last", entry.RawGetString("last"))
			if err != nil {
				return fontdesc.RuleMatch{}, 0, err
			}
			match.Ranges[index] = fontdesc.RuneRange{First: first, Last: last}
		}
	}
	return match, rawRangeCount, nil
}

func parseRuleIntegerRange(source, path string, value lua.LValue, minimum, maximum int) (fontdesc.OptionalIntRange, error) {
	table, ok := value.(*lua.LTable)
	if !ok {
		return fontdesc.OptionalIntRange{}, typeError(source, path, KindTable, value)
	}
	keys, err := strictStringKeys(source, path, table)
	if err != nil {
		return fontdesc.OptionalIntRange{}, err
	}
	for _, key := range keys {
		if key != "min" && key != "max" {
			return fontdesc.OptionalIntRange{}, documentError(source, joinPath(path, key), "unknown field")
		}
	}
	minValue, err := requiredBoundedInteger(source, path+".min", table.RawGetString("min"), minimum, maximum)
	if err != nil {
		return fontdesc.OptionalIntRange{}, err
	}
	maxValue, err := requiredBoundedInteger(source, path+".max", table.RawGetString("max"), minimum, maximum)
	if err != nil {
		return fontdesc.OptionalIntRange{}, err
	}
	if minValue > maxValue {
		return fontdesc.OptionalIntRange{}, documentError(source, path, "min must not exceed max")
	}
	return fontdesc.OptionalIntRange{Min: minValue, Max: maxValue, Present: true}, nil
}

func requiredBoundedInteger(source, path string, value lua.LValue, minimum, maximum int) (int, error) {
	if value == lua.LNil {
		return 0, documentError(source, path, "is required")
	}
	if err := validateInteger(source, path, value); err != nil {
		return 0, err
	}
	parsed := int(value.(lua.LNumber))
	if parsed < minimum || parsed > maximum {
		return 0, documentError(source, path, "must be between %d and %d", minimum, maximum)
	}
	return parsed, nil
}

func requiredUnicodeScalar(source, path string, value lua.LValue) (rune, error) {
	parsed, err := requiredBoundedInteger(source, path, value, 0, unicode.MaxRune)
	if err != nil {
		return 0, err
	}
	if parsed >= 0xD800 && parsed <= 0xDFFF {
		return 0, documentError(source, path, "must be a Unicode scalar value")
	}
	return rune(parsed), nil
}
