package background

import (
	"image/color"
	"math"
	"strings"
	"testing"
)

func TestNormalizeLayers(t *testing.T) {
	source := testSource(1, 1, color.RGBA{R: 0xff, A: 0xff}, "model")
	validGradient := &LinearGradient{Angle: -90, Stops: []GradientStop{
		{Offset: 0, Color: color.RGBA{A: 0xff}},
		{Offset: 1, Color: color.RGBA{R: 0xff, A: 0xff}},
	}}
	tests := []struct {
		name    string
		layers  []Layer
		wantErr string
	}{
		{name: "empty"},
		{name: "all variants", layers: []Layer{
			{Opacity: 1, Solid: &Solid{Color: color.RGBA{A: 0xff}}},
			{Opacity: 0.5, LinearGradient: validGradient},
			{Opacity: 0, Image: &Image{Source: source, Fit: FitCover, Horizontal: AlignCenter, Vertical: AlignMiddle}},
		}},
		{name: "too many layers", layers: make([]Layer, MaxLayers+1), wantErr: "maximum"},
		{name: "no variant", layers: []Layer{{Opacity: 1}}, wantErr: "exactly one"},
		{name: "multiple variants", layers: []Layer{{Opacity: 1, Solid: &Solid{}, LinearGradient: validGradient}}, wantErr: "exactly one"},
		{name: "nan opacity", layers: []Layer{{Opacity: math.NaN(), Solid: &Solid{}}}, wantErr: "opacity"},
		{name: "opacity high", layers: []Layer{{Opacity: 1.01, Solid: &Solid{}}}, wantErr: "opacity"},
		{name: "nonfinite angle", layers: []Layer{{Opacity: 1, LinearGradient: &LinearGradient{Angle: math.Inf(1), Stops: validGradient.Stops}}}, wantErr: "angle"},
		{name: "few stops", layers: []Layer{{Opacity: 1, LinearGradient: &LinearGradient{Stops: []GradientStop{{}}}}}, wantErr: "stops"},
		{name: "unordered stops", layers: []Layer{{Opacity: 1, LinearGradient: &LinearGradient{Stops: []GradientStop{{Offset: .8}, {Offset: .2}}}}}, wantErr: "ordered"},
		{name: "bad stop", layers: []Layer{{Opacity: 1, LinearGradient: &LinearGradient{Stops: []GradientStop{{Offset: 0}, {Offset: 2}}}}}, wantErr: "offset"},
		{name: "nil image", layers: []Layer{{Opacity: 1, Image: &Image{Fit: FitNone, Horizontal: AlignLeft, Vertical: AlignTop}}}, wantErr: "source"},
		{name: "bad fit", layers: []Layer{{Opacity: 1, Image: &Image{Source: source, Fit: "tile", Horizontal: AlignLeft, Vertical: AlignTop}}}, wantErr: "fit"},
		{name: "bad horizontal", layers: []Layer{{Opacity: 1, Image: &Image{Source: source, Fit: FitNone, Horizontal: "near", Vertical: AlignTop}}}, wantErr: "horizontal"},
		{name: "bad vertical", layers: []Layer{{Opacity: 1, Image: &Image{Source: source, Fit: FitNone, Horizontal: AlignLeft, Vertical: "near"}}}, wantErr: "vertical"},
	}

	fourImages := make([]Layer, MaxImageLayers+1)
	for index := range fourImages {
		fourImages[index] = Layer{Opacity: 1, Image: &Image{Source: source, Fit: FitNone, Horizontal: AlignLeft, Vertical: AlignTop}}
	}
	tests = append(tests, struct {
		name    string
		layers  []Layer
		wantErr string
	}{name: "too many images", layers: fourImages, wantErr: "image layers"})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized, err := NormalizeLayers(test.layers)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeLayers() error = %v", err)
			}
			if len(normalized) != len(test.layers) {
				t.Fatalf("len = %d, want %d", len(normalized), len(test.layers))
			}
		})
	}
}

func TestNormalizeLayersDetachesAndNormalizes(t *testing.T) {
	stops := []GradientStop{{Offset: 0}, {Offset: 1}}
	input := []Layer{{Opacity: 1, LinearGradient: &LinearGradient{Angle: 810, Stops: stops}}}
	normalized, err := NormalizeLayers(input)
	if err != nil {
		t.Fatal(err)
	}
	if normalized[0].LinearGradient.Angle != 90 {
		t.Fatalf("angle = %v, want 90", normalized[0].LinearGradient.Angle)
	}
	stops[0].Offset = .5
	input[0].LinearGradient.Angle = 12
	if normalized[0].LinearGradient.Stops[0].Offset != 0 || normalized[0].LinearGradient.Angle != 90 {
		t.Fatal("normalized layers alias input")
	}
}
