package background

import (
	"fmt"
	"image"
	"image/color"
	"math"
)

// Compose returns a freshly allocated straight-alpha RGBA surface. It starts
// with base and applies normalized authored layers in order using source-over.
// The returned image never aliases a decoded source.
func Compose(width, height int, base color.RGBA, layers []Layer, budget *Budget) (*image.RGBA, error) {
	normalized, err := NormalizeLayers(layers)
	if err != nil {
		return nil, err
	}
	outputBytes, err := checkedRGBABytes(width, height)
	if err != nil {
		return nil, err
	}

	sources := make([]*Source, 0, MaxImageLayers)
	for i, layer := range normalized {
		if layer.Image == nil {
			continue
		}
		source := layer.Image.Source
		if source.closed || source.rgba == nil {
			return nil, fmt.Errorf("layer %d image: source is closed", i)
		}
		sources = append(sources, source)
	}
	if err := budget.reserveComposition(sources, outputBytes); err != nil {
		return nil, err
	}

	destination := image.NewRGBA(image.Rect(0, 0, width, height))
	fillRGBA(destination, base)
	if width == 0 || height == 0 {
		return destination, nil
	}
	for _, layer := range normalized {
		switch {
		case layer.Solid != nil:
			composeSolid(destination, layer.Solid.Color, layer.Opacity)
		case layer.LinearGradient != nil:
			composeGradient(destination, *layer.LinearGradient, layer.Opacity)
		case layer.Image != nil:
			composeImage(destination, *layer.Image, layer.Opacity)
		}
	}
	return destination, nil
}

func fillRGBA(destination *image.RGBA, value color.RGBA) {
	bounds := destination.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		offset := destination.PixOffset(bounds.Min.X, y)
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			destination.Pix[offset+0] = value.R
			destination.Pix[offset+1] = value.G
			destination.Pix[offset+2] = value.B
			destination.Pix[offset+3] = value.A
			offset += 4
		}
	}
}

func composeSolid(destination *image.RGBA, value color.RGBA, opacity float64) {
	source := scaleAlpha(value, opacity)
	bounds := destination.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			blendPixel(destination, x, y, source)
		}
	}
}

func composeGradient(destination *image.RGBA, gradient LinearGradient, opacity float64) {
	bounds := destination.Bounds()
	radians := gradient.Angle * math.Pi / 180
	dx, dy := math.Cos(radians), math.Sin(radians)
	maxX := float64(bounds.Dx() - 1)
	maxY := float64(bounds.Dy() - 1)
	projections := [4]float64{0, dx * maxX, dy * maxY, dx*maxX + dy*maxY}
	minimum, maximum := projections[0], projections[0]
	for _, projection := range projections[1:] {
		minimum = math.Min(minimum, projection)
		maximum = math.Max(maximum, projection)
	}
	span := maximum - minimum
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			t := 0.5
			if span > 0 {
				t = (dx*float64(x) + dy*float64(y) - minimum) / span
			}
			source := scaleAlpha(interpolateGradient(gradient.Stops, t), opacity)
			blendPixel(destination, bounds.Min.X+x, bounds.Min.Y+y, source)
		}
	}
}

func interpolateGradient(stops []GradientStop, position float64) color.RGBA {
	if position <= stops[0].Offset {
		return stops[0].Color
	}
	last := stops[len(stops)-1]
	if position >= last.Offset {
		return last.Color
	}
	for index := 1; index < len(stops); index++ {
		right := stops[index]
		if position > right.Offset {
			continue
		}
		left := stops[index-1]
		span := right.Offset - left.Offset
		if span == 0 {
			return right.Color
		}
		amount := (position - left.Offset) / span
		return color.RGBA{
			R: interpolateByte(left.Color.R, right.Color.R, amount),
			G: interpolateByte(left.Color.G, right.Color.G, amount),
			B: interpolateByte(left.Color.B, right.Color.B, amount),
			A: interpolateByte(left.Color.A, right.Color.A, amount),
		}
	}
	return last.Color
}

func interpolateByte(left, right uint8, amount float64) uint8 {
	return uint8(math.Round(float64(left) + (float64(right)-float64(left))*amount))
}

func composeImage(destination *image.RGBA, layer Image, opacity float64) {
	source := layer.Source.rgba
	sourceBounds := source.Bounds()
	destinationBounds := destination.Bounds()
	target := fittedRectangle(destinationBounds, sourceBounds.Dx(), sourceBounds.Dy(), layer)
	visible := target.Intersect(destinationBounds)
	if visible.Empty() || target.Dx() <= 0 || target.Dy() <= 0 {
		return
	}
	for y := visible.Min.Y; y < visible.Max.Y; y++ {
		sourceY := sourceBounds.Min.Y + int((int64(y-target.Min.Y)*int64(sourceBounds.Dy()))/int64(target.Dy()))
		if sourceY >= sourceBounds.Max.Y {
			sourceY = sourceBounds.Max.Y - 1
		}
		for x := visible.Min.X; x < visible.Max.X; x++ {
			sourceX := sourceBounds.Min.X + int((int64(x-target.Min.X)*int64(sourceBounds.Dx()))/int64(target.Dx()))
			if sourceX >= sourceBounds.Max.X {
				sourceX = sourceBounds.Max.X - 1
			}
			blendPixel(destination, x, y, scaleAlpha(source.RGBAAt(sourceX, sourceY), opacity))
		}
	}
}

func fittedRectangle(destination image.Rectangle, sourceWidth, sourceHeight int, layer Image) image.Rectangle {
	width, height := sourceWidth, sourceHeight
	switch layer.Fit {
	case FitStretch:
		width, height = destination.Dx(), destination.Dy()
	case FitCover, FitContain:
		xScale := float64(destination.Dx()) / float64(sourceWidth)
		yScale := float64(destination.Dy()) / float64(sourceHeight)
		scale := math.Max(xScale, yScale)
		if layer.Fit == FitContain {
			scale = math.Min(xScale, yScale)
		}
		width = max(1, int(math.Round(float64(sourceWidth)*scale)))
		height = max(1, int(math.Round(float64(sourceHeight)*scale)))
	}
	x := alignedOffset(destination.Min.X, destination.Dx(), width, layer.Horizontal)
	y := alignedVerticalOffset(destination.Min.Y, destination.Dy(), height, layer.Vertical)
	return image.Rect(x, y, x+width, y+height)
}

func alignedOffset(origin, available, size int, alignment HorizontalAlignment) int {
	switch alignment {
	case AlignLeft:
		return origin
	case AlignRight:
		return origin + available - size
	default:
		return origin + (available-size)/2
	}
}

func alignedVerticalOffset(origin, available, size int, alignment VerticalAlignment) int {
	switch alignment {
	case AlignTop:
		return origin
	case AlignBottom:
		return origin + available - size
	default:
		return origin + (available-size)/2
	}
}

func scaleAlpha(value color.RGBA, opacity float64) color.RGBA {
	value.A = uint8(math.Round(float64(value.A) * opacity))
	return value
}

func blendPixel(destination *image.RGBA, x, y int, source color.RGBA) {
	offset := destination.PixOffset(x, y)
	sourceAlpha := float64(source.A) / 255
	destinationAlpha := float64(destination.Pix[offset+3]) / 255
	outputAlpha := sourceAlpha + destinationAlpha*(1-sourceAlpha)
	if outputAlpha == 0 {
		for channel := 0; channel < 4; channel++ {
			destination.Pix[offset+channel] = 0
		}
		return
	}
	for channel, sourceValue := range []uint8{source.R, source.G, source.B} {
		destinationValue := float64(destination.Pix[offset+channel])
		output := (float64(sourceValue)*sourceAlpha + destinationValue*destinationAlpha*(1-sourceAlpha)) / outputAlpha
		destination.Pix[offset+channel] = uint8(math.Round(output))
	}
	destination.Pix[offset+3] = uint8(math.Round(outputAlpha * 255))
}
