package background

import (
	"image/color"
	"testing"
)

func TestComposeStraightAlphaSourceOver(t *testing.T) {
	layers := []Layer{{Opacity: 1, Solid: &Solid{Color: color.RGBA{R: 255, A: 128}}}}
	got, err := Compose(1, 1, color.RGBA{B: 255, A: 255}, layers, NewBudget())
	if err != nil {
		t.Fatal(err)
	}
	if want := (color.RGBA{R: 128, B: 127, A: 255}); got.RGBAAt(0, 0) != want {
		t.Fatalf("source-over = %#v, want %#v", got.RGBAAt(0, 0), want)
	}
}

func TestComposeGradientAndImagePlacement(t *testing.T) {
	gradient := Layer{Opacity: 1, LinearGradient: &LinearGradient{Angle: 0, Stops: []GradientStop{
		{Offset: 0, Color: color.RGBA{A: 255}},
		{Offset: 1, Color: color.RGBA{R: 255, A: 255}},
	}}}
	got, err := Compose(3, 1, color.RGBA{}, []Layer{gradient}, NewBudget())
	if err != nil {
		t.Fatal(err)
	}
	if got.RGBAAt(0, 0).R != 0 || got.RGBAAt(1, 0).R != 128 || got.RGBAAt(2, 0).R != 255 {
		t.Fatalf("gradient pixels = %v %v %v", got.RGBAAt(0, 0), got.RGBAAt(1, 0), got.RGBAAt(2, 0))
	}

	source := testSource(1, 1, color.RGBA{G: 255, A: 255}, "placement")
	imageLayer := Layer{Opacity: 1, Image: &Image{Source: source, Fit: FitNone, Horizontal: AlignRight, Vertical: AlignBottom}}
	placed, err := Compose(2, 2, color.RGBA{A: 255}, []Layer{imageLayer}, NewBudget())
	if err != nil {
		t.Fatal(err)
	}
	if placed.RGBAAt(1, 1).G != 255 || placed.RGBAAt(0, 0).G != 0 {
		t.Fatalf("image placement wrong: %#v %#v", placed.RGBAAt(1, 1), placed.RGBAAt(0, 0))
	}
}

func TestComposeBudgetsAndClosedSource(t *testing.T) {
	if _, err := Compose(10000, 10000, color.RGBA{}, nil, NewBudget()); err == nil {
		t.Fatal("expected output CPU budget rejection")
	}
	source := testSource(1, 1, color.RGBA{A: 255}, "closed")
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	layer := Layer{Opacity: 1, Image: &Image{Source: source, Fit: FitNone, Horizontal: AlignLeft, Vertical: AlignTop}}
	if _, err := Compose(1, 1, color.RGBA{}, []Layer{layer}, NewBudget()); err == nil {
		t.Fatal("expected closed-source rejection")
	}
}
