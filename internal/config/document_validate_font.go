package config

import (
	"fmt"
	"strings"

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
	if table.Len() > fontdesc.MaxPrimaryDescriptors {
		return documentError(source, path, "must contain at most %d entries, got %d", fontdesc.MaxPrimaryDescriptors, table.Len())
	}
	for index := 1; index <= table.Len(); index++ {
		entryPath := fmt.Sprintf("%s[%d]", path, index)
		rawEntry := table.RawGetInt(index)
		entry, ok := rawEntry.(*lua.LTable)
		if !ok {
			return typeError(source, entryPath, KindTable, rawEntry)
		}
		keys, err := strictStringKeys(source, entryPath, entry)
		if err != nil {
			return err
		}
		for _, key := range keys {
			switch key {
			case "family", "collection_face", "collection_index", "weight", "style", "stretch", "attribute_mode":
			default:
				return documentError(source, joinPath(entryPath, key), "unknown field")
			}
		}

		rawFamily := entry.RawGetString("family")
		family, ok := rawFamily.(lua.LString)
		if !ok {
			if rawFamily == lua.LNil {
				return documentError(source, entryPath+".family", "is required")
			}
			return typeError(source, entryPath+".family", KindString, rawFamily)
		}
		descriptor := fontdesc.Descriptor{Family: string(family)}
		if strings.TrimSpace(descriptor.Family) == "" {
			return documentError(source, entryPath+".family", "must not be empty")
		}

		rawCollectionFace := entry.RawGetString("collection_face")
		collectionFacePresent := rawCollectionFace != lua.LNil
		if collectionFacePresent {
			face, ok := rawCollectionFace.(lua.LString)
			if !ok {
				return typeError(source, entryPath+".collection_face", KindString, rawCollectionFace)
			}
			descriptor.CollectionFace = string(face)
			if strings.TrimSpace(descriptor.CollectionFace) == "" {
				return documentError(source, entryPath+".collection_face", "must not be empty")
			}
		}
		if raw := entry.RawGetString("collection_index"); raw != lua.LNil {
			if err := validateInteger(source, entryPath+".collection_index", raw); err != nil {
				return err
			}
			parsed := int(raw.(lua.LNumber))
			if parsed < 0 || parsed >= fontdesc.MaxFacesPerFile {
				return documentError(source, entryPath+".collection_index", "must be between 0 and %d", fontdesc.MaxFacesPerFile-1)
			}
			if collectionFacePresent {
				return documentError(source, entryPath+".collection_index", "cannot be set with collection_face")
			}
			descriptor.CollectionIndex = fontdesc.SomeCollectionIndex(uint32(parsed))
		}
		if raw := entry.RawGetString("weight"); raw != lua.LNil {
			if err := validateInteger(source, entryPath+".weight", raw); err != nil {
				return err
			}
			descriptor.Weight = int(raw.(lua.LNumber))
			if descriptor.Weight < 100 || descriptor.Weight > 900 {
				return documentError(source, entryPath+".weight", "must be between 100 and 900")
			}
		}
		if raw := entry.RawGetString("style"); raw != lua.LNil {
			style, ok := raw.(lua.LString)
			if !ok {
				return typeError(source, entryPath+".style", KindString, raw)
			}
			descriptor.Style = fontdesc.Style(style)
			switch descriptor.Style {
			case fontdesc.StyleNormal, fontdesc.StyleItalic, fontdesc.StyleOblique:
			default:
				return documentError(source, entryPath+".style", "must be normal, italic, or oblique")
			}
		}
		if raw := entry.RawGetString("stretch"); raw != lua.LNil {
			if err := validateInteger(source, entryPath+".stretch", raw); err != nil {
				return err
			}
			descriptor.Stretch = int(raw.(lua.LNumber))
			if descriptor.Stretch < 50 || descriptor.Stretch > 200 {
				return documentError(source, entryPath+".stretch", "must be between 50 and 200")
			}
		}
		if raw := entry.RawGetString("attribute_mode"); raw != lua.LNil {
			mode, ok := raw.(lua.LString)
			if !ok {
				return typeError(source, entryPath+".attribute_mode", KindString, raw)
			}
			descriptor.AttributeMode = fontdesc.AttributeMode(mode)
			switch descriptor.AttributeMode {
			case fontdesc.AttributeModeAugment, fontdesc.AttributeModeFixed:
			default:
				return documentError(source, entryPath+".attribute_mode", "must be augment or fixed")
			}
		}
		if _, err := descriptor.Normalize(); err != nil {
			return documentError(source, entryPath, "%v", err)
		}
	}
	return nil
}
