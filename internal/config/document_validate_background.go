package config

import (
	"fmt"
	"math"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

func validateBackgroundLayerList(source, path string, value lua.LValue) error {
	list, ok := value.(*lua.LTable)
	if !ok {
		return typeError(source, path, KindBackgroundLayerList, value)
	}
	if err := validateDenseArray(source, path, list); err != nil {
		return err
	}
	if list.Len() > MaxBackgroundLayers {
		return documentError(source, path, "must contain at most %d entries", MaxBackgroundLayers)
	}
	images := 0
	for index := 1; index <= list.Len(); index++ {
		itemPath := fmt.Sprintf("%s[%d]", path, index)
		item, ok := list.RawGetInt(index).(*lua.LTable)
		if !ok {
			return typeError(source, itemPath, KindTable, list.RawGetInt(index))
		}
		keys, err := strictStringKeys(source, itemPath, item)
		if err != nil {
			return err
		}
		kindValue := item.RawGetString("kind")
		kind, ok := kindValue.(lua.LString)
		if !ok {
			return typeError(source, itemPath+".kind", KindString, kindValue)
		}
		allowed := map[string]bool{"kind": true, "opacity": true}
		switch string(kind) {
		case "solid":
			allowed["color"] = true
		case "linear_gradient":
			allowed["colors"], allowed["angle"] = true, true
		case "image":
			images++
			allowed["path"], allowed["fit"], allowed["horizontal_align"], allowed["vertical_align"] = true, true, true, true
		default:
			return documentError(source, itemPath+".kind", "must be solid, linear_gradient, or image")
		}
		for _, key := range keys {
			if !allowed[key] {
				return documentError(source, itemPath+"."+key, "unknown field")
			}
		}
		if opacity := item.RawGetString("opacity"); opacity != lua.LNil {
			number, ok := opacity.(lua.LNumber)
			if !ok {
				return typeError(source, itemPath+".opacity", KindNumber, opacity)
			}
			parsed := float64(number)
			if math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 || parsed > 1 {
				return documentError(source, itemPath+".opacity", "must be finite and between 0 and 1")
			}
		}
		switch string(kind) {
		case "solid":
			if err := validateBackgroundColorValue(source, itemPath+".color", item.RawGetString("color")); err != nil {
				return err
			}
		case "linear_gradient":
			colors := item.RawGetString("colors")
			if err := validateStringList(source, itemPath+".colors", colors); err != nil {
				return err
			}
			colorList := colors.(*lua.LTable)
			if colorList.Len() < 2 || colorList.Len() > MaxBackgroundColors {
				return documentError(source, itemPath+".colors", "must contain 2..%d entries", MaxBackgroundColors)
			}
			for colorIndex := 1; colorIndex <= colorList.Len(); colorIndex++ {
				if err := validateBackgroundColorValue(source, fmt.Sprintf("%s.colors[%d]", itemPath, colorIndex), colorList.RawGetInt(colorIndex)); err != nil {
					return err
				}
			}
			if angle := item.RawGetString("angle"); angle != lua.LNil {
				number, ok := angle.(lua.LNumber)
				if !ok || math.IsNaN(float64(number)) || math.IsInf(float64(number), 0) {
					return documentError(source, itemPath+".angle", "must be a finite number")
				}
			}
		case "image":
			pathValue, ok := item.RawGetString("path").(lua.LString)
			if !ok || strings.TrimSpace(string(pathValue)) == "" {
				return documentError(source, itemPath+".path", "must be a non-empty string")
			}
			for _, option := range []struct {
				field string
				valid map[string]bool
			}{
				{"fit", map[string]bool{"cover": true, "contain": true, "stretch": true, "none": true}},
				{"horizontal_align", map[string]bool{"left": true, "center": true, "right": true}},
				{"vertical_align", map[string]bool{"top": true, "center": true, "bottom": true}},
			} {
				field, valid := option.field, option.valid
				fieldValue := item.RawGetString(field)
				if fieldValue == lua.LNil {
					continue
				}
				text, ok := fieldValue.(lua.LString)
				if !ok || !valid[string(text)] {
					return documentError(source, itemPath+"."+field, "has an unsupported value")
				}
			}
		}
	}
	if images > MaxBackgroundImageLayers {
		return documentError(source, path, "may contain at most %d images", MaxBackgroundImageLayers)
	}
	return nil
}

func validateBackgroundColorValue(source, path string, value lua.LValue) error {
	text, ok := value.(lua.LString)
	if !ok {
		return typeError(source, path, KindString, value)
	}
	if !isHexColor(string(text)) {
		return documentError(source, path, "must be #RRGGBB or #RRGGBBAA")
	}
	return nil
}
