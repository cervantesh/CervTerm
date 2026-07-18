// Package background provides dependency-light background layer modeling,
// decoding, composition, and caching. It intentionally has no frontend or
// configuration dependencies.
package background

import (
	"fmt"
	"image/color"
	"math"
)

const (
	MaxLayers        = 8
	MaxImageLayers   = 4
	MinGradientStops = 2
	MaxGradientStops = 8
)

type Fit string

const (
	FitCover   Fit = "cover"
	FitContain Fit = "contain"
	FitStretch Fit = "stretch"
	FitNone    Fit = "none"
)

type HorizontalAlignment string

const (
	AlignLeft   HorizontalAlignment = "left"
	AlignCenter HorizontalAlignment = "center"
	AlignRight  HorizontalAlignment = "right"
)

type VerticalAlignment string

const (
	AlignTop    VerticalAlignment = "top"
	AlignMiddle VerticalAlignment = "center"
	AlignBottom VerticalAlignment = "bottom"
)

type Solid struct {
	Color color.RGBA
}

type GradientStop struct {
	Offset float64
	Color  color.RGBA
}

type LinearGradient struct {
	Angle float64
	Stops []GradientStop
}

type Image struct {
	Source     *Source
	Fit        Fit
	Horizontal HorizontalAlignment
	Vertical   VerticalAlignment
}

type Layer struct {
	Opacity        float64
	Solid          *Solid
	LinearGradient *LinearGradient
	Image          *Image
}

// NormalizeLayers validates a strict layer union and returns a detached copy.
// Gradient angles are normalized to [0, 360).
func NormalizeLayers(layers []Layer) ([]Layer, error) {
	if len(layers) > MaxLayers {
		return nil, fmt.Errorf("background layers: maximum is %d", MaxLayers)
	}

	normalized := make([]Layer, len(layers))
	images := 0
	for i, layer := range layers {
		if !finite(layer.Opacity) || layer.Opacity < 0 || layer.Opacity > 1 {
			return nil, fmt.Errorf("layer %d opacity: expected finite value in [0,1]", i)
		}
		variants := 0
		if layer.Solid != nil {
			variants++
		}
		if layer.LinearGradient != nil {
			variants++
		}
		if layer.Image != nil {
			variants++
		}
		if variants != 1 {
			return nil, fmt.Errorf("layer %d: expected exactly one layer kind", i)
		}

		normalized[i].Opacity = layer.Opacity
		switch {
		case layer.Solid != nil:
			solid := *layer.Solid
			normalized[i].Solid = &solid
		case layer.LinearGradient != nil:
			gradient := *layer.LinearGradient
			if !finite(gradient.Angle) {
				return nil, fmt.Errorf("layer %d gradient angle: expected finite value", i)
			}
			if len(gradient.Stops) < MinGradientStops || len(gradient.Stops) > MaxGradientStops {
				return nil, fmt.Errorf("layer %d gradient stops: expected %d..%d", i, MinGradientStops, MaxGradientStops)
			}
			gradient.Angle = normalizeAngle(gradient.Angle)
			gradient.Stops = append([]GradientStop(nil), gradient.Stops...)
			for stopIndex, stop := range gradient.Stops {
				if !finite(stop.Offset) || stop.Offset < 0 || stop.Offset > 1 {
					return nil, fmt.Errorf("layer %d gradient stop %d: expected finite offset in [0,1]", i, stopIndex)
				}
				if stopIndex > 0 && stop.Offset < gradient.Stops[stopIndex-1].Offset {
					return nil, fmt.Errorf("layer %d gradient stops: offsets must be ordered", i)
				}
			}
			normalized[i].LinearGradient = &gradient
		case layer.Image != nil:
			images++
			if images > MaxImageLayers {
				return nil, fmt.Errorf("background image layers: maximum is %d", MaxImageLayers)
			}
			imageLayer := *layer.Image
			if imageLayer.Source == nil {
				return nil, fmt.Errorf("layer %d image: source is required", i)
			}
			if !validFit(imageLayer.Fit) {
				return nil, fmt.Errorf("layer %d image fit: unsupported value", i)
			}
			if !validHorizontal(imageLayer.Horizontal) {
				return nil, fmt.Errorf("layer %d image horizontal alignment: unsupported value", i)
			}
			if !validVertical(imageLayer.Vertical) {
				return nil, fmt.Errorf("layer %d image vertical alignment: unsupported value", i)
			}
			normalized[i].Image = &imageLayer
		}
	}
	return normalized, nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func normalizeAngle(angle float64) float64 {
	angle = math.Mod(angle, 360)
	if angle < 0 {
		angle += 360
	}
	if angle == 0 {
		return 0
	}
	return angle
}

func validFit(fit Fit) bool {
	return fit == FitCover || fit == FitContain || fit == FitStretch || fit == FitNone
}

func validHorizontal(alignment HorizontalAlignment) bool {
	return alignment == AlignLeft || alignment == AlignCenter || alignment == AlignRight
}

func validVertical(alignment VerticalAlignment) bool {
	return alignment == AlignTop || alignment == AlignMiddle || alignment == AlignBottom
}
