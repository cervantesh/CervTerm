package config

import (
	"fmt"
	"math"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

const (
	MaxBackgroundLayers      = 8
	MaxBackgroundImageLayers = 4
	MaxBackgroundColors      = 8
)

type BackgroundConfig struct {
	Layers []BackgroundLayer
}

type BackgroundLayer struct {
	Kind            string
	Opacity         float64
	Color           string
	Colors          []string
	Angle           float64
	Path            string
	Fit             string
	HorizontalAlign string
	VerticalAlign   string
}

func cloneBackgroundLayers(layers []BackgroundLayer) []BackgroundLayer {
	if layers == nil {
		return nil
	}
	cloned := append([]BackgroundLayer(nil), layers...)
	for index := range cloned {
		cloned[index].Colors = append([]string(nil), layers[index].Colors...)
	}
	return cloned
}

func backgroundLayerListField(table *lua.LTable, key string, fallback []BackgroundLayer) []BackgroundLayer {
	value := table.RawGetString(key)
	list, ok := value.(*lua.LTable)
	if !ok {
		return fallback
	}
	layers := make([]BackgroundLayer, 0, list.Len())
	for index := 1; index <= list.Len(); index++ {
		item, ok := list.RawGetInt(index).(*lua.LTable)
		if !ok {
			continue
		}
		layer := BackgroundLayer{
			Kind: stringField(item, "kind", ""), Opacity: numberField(item, "opacity", 1),
			Angle: numberField(item, "angle", 0), Fit: stringField(item, "fit", "cover"),
			HorizontalAlign: stringField(item, "horizontal_align", "center"), VerticalAlign: stringField(item, "vertical_align", "center"),
			Color: stringField(item, "color", ""), Path: stringField(item, "path", ""),
		}
		if colors, ok := item.RawGetString("colors").(*lua.LTable); ok {
			layer.Colors = make([]string, 0, colors.Len())
			for colorIndex := 1; colorIndex <= colors.Len(); colorIndex++ {
				if value, ok := colors.RawGetInt(colorIndex).(lua.LString); ok {
					layer.Colors = append(layer.Colors, string(value))
				}
			}
		}
		layers = append(layers, layer)
	}
	return layers
}

func validateBackgroundLayers(layers []BackgroundLayer) error {
	if len(layers) > MaxBackgroundLayers {
		return fmt.Errorf("background.layers must contain at most %d entries", MaxBackgroundLayers)
	}
	images := 0
	for index, layer := range layers {
		path := fmt.Sprintf("background.layers[%d]", index+1)
		if math.IsNaN(layer.Opacity) || math.IsInf(layer.Opacity, 0) || layer.Opacity < 0 || layer.Opacity > 1 {
			return fmt.Errorf("%s.opacity must be a finite number between 0 and 1", path)
		}
		switch layer.Kind {
		case "solid":
			if !isHexColor(layer.Color) {
				return fmt.Errorf("%s.color must be #RRGGBB or #RRGGBBAA", path)
			}
		case "linear_gradient":
			if math.IsNaN(layer.Angle) || math.IsInf(layer.Angle, 0) {
				return fmt.Errorf("%s.angle must be finite", path)
			}
			if len(layer.Colors) < 2 || len(layer.Colors) > MaxBackgroundColors {
				return fmt.Errorf("%s.colors must contain 2..%d entries", path, MaxBackgroundColors)
			}
			for colorIndex, value := range layer.Colors {
				if !isHexColor(value) {
					return fmt.Errorf("%s.colors[%d] must be #RRGGBB or #RRGGBBAA", path, colorIndex+1)
				}
			}
		case "image":
			images++
			if images > MaxBackgroundImageLayers {
				return fmt.Errorf("background.layers may contain at most %d images", MaxBackgroundImageLayers)
			}
			if strings.TrimSpace(layer.Path) == "" {
				return fmt.Errorf("%s.path must not be empty", path)
			}
			if layer.Fit != "cover" && layer.Fit != "contain" && layer.Fit != "stretch" && layer.Fit != "none" {
				return fmt.Errorf("%s.fit is unsupported", path)
			}
			if layer.HorizontalAlign != "left" && layer.HorizontalAlign != "center" && layer.HorizontalAlign != "right" {
				return fmt.Errorf("%s.horizontal_align is unsupported", path)
			}
			if layer.VerticalAlign != "top" && layer.VerticalAlign != "center" && layer.VerticalAlign != "bottom" {
				return fmt.Errorf("%s.vertical_align is unsupported", path)
			}
		default:
			return fmt.Errorf("%s.kind must be solid, linear_gradient, or image", path)
		}
	}
	return nil
}
