package fontglyph

import (
	"image"
	"image/color"
	"testing"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

func TestDrawSegmentsProducesColoredPixels(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	segments := sfnt.Segments{
		{Op: sfnt.SegmentOpMoveTo, Args: [3]fixed.Point26_6{fixed.P(2, -12)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(14, -12)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(14, 0)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(2, 0)}},
	}
	drawSegments(img, segments, color.RGBA{R: 255, A: 255}, 0, 14, identityCOLRTransform())
	if !hasOpaquePixel(img) {
		t.Fatalf("drawSegments produced no visible pixels")
	}
	_, g, _, a := img.At(8, 8).RGBA()
	if a == 0 || g != 0 {
		t.Fatalf("expected red-ish pixel at center, got rgba=%#x %#x %#x %#x", img.RGBAAt(8, 8).R, img.RGBAAt(8, 8).G, img.RGBAAt(8, 8).B, img.RGBAAt(8, 8).A)
	}
}

func TestDrawSegmentsAppliesTransform(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 24, 16))
	segments := sfnt.Segments{
		{Op: sfnt.SegmentOpMoveTo, Args: [3]fixed.Point26_6{fixed.P(2, -12)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(8, -12)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(8, 0)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(2, 0)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(2, -12)}},
	}
	drawSegments(img, segments, color.RGBA{B: 255, A: 255}, 0, 14, translateCOLR(10, 0))
	if _, _, _, a := img.At(1, 1).RGBA(); a != 0 {
		t.Fatalf("expected untranslated region to stay transparent")
	}
	if _, _, _, a := img.At(14, 8).RGBA(); a == 0 {
		t.Fatalf("expected translated region to contain pixels")
	}
}

func TestDrawGradientSegmentsPaintsGradientInsideMask(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 24, 16))
	segments := sfnt.Segments{
		{Op: sfnt.SegmentOpMoveTo, Args: [3]fixed.Point26_6{fixed.P(2, -12)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(18, -12)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(18, 0)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(2, 0)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(2, -12)}},
	}
	layer := COLRLayer{
		Fill:      COLRFillLinearGradient,
		Transform: identityCOLRTransform(),
		LinearGradient: COLRLinearGradient{
			X0: 2, Y0: -6, X1: 18, Y1: -6,
			Stops: []COLRColorStop{
				{Offset: 0, Color: color.RGBA{R: 255, A: 255}},
				{Offset: 1, Color: color.RGBA{G: 255, A: 255}},
			},
		},
	}
	drawGradientSegments(img, segments, layer, 0, 14)
	left := img.RGBAAt(4, 8)
	right := img.RGBAAt(16, 8)
	if left.A == 0 || right.A == 0 {
		t.Fatalf("expected gradient to paint inside mask, left=%#v right=%#v", left, right)
	}
	if left.R <= left.G {
		t.Fatalf("expected left side to be red-dominant, got %#v", left)
	}
	if right.G <= right.R {
		t.Fatalf("expected right side to be green-dominant, got %#v", right)
	}
	if outside := img.RGBAAt(0, 0); outside.A != 0 {
		t.Fatalf("expected outside mask to stay transparent, got %#v", outside)
	}
}

func TestDrawGradientSegmentsPaintsRadialGradientInsideMask(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 24, 16))
	segments := sfnt.Segments{
		{Op: sfnt.SegmentOpMoveTo, Args: [3]fixed.Point26_6{fixed.P(2, -12)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(18, -12)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(18, 0)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(2, 0)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(2, -12)}},
	}
	layer := COLRLayer{
		Fill:      COLRFillRadialGradient,
		Transform: identityCOLRTransform(),
		RadialGradient: COLRRadialGradient{
			X0: 10, Y0: -6, Radius0: 0, X1: 10, Y1: -6, Radius1: 10,
			Stops: []COLRColorStop{
				{Offset: 0, Color: color.RGBA{R: 255, A: 255}},
				{Offset: 1, Color: color.RGBA{G: 255, A: 255}},
			},
		},
	}
	drawGradientSegments(img, segments, layer, 0, 14)
	center := img.RGBAAt(10, 8)
	edge := img.RGBAAt(16, 8)
	if center.A == 0 || edge.A == 0 {
		t.Fatalf("expected radial gradient to paint inside mask, center=%#v edge=%#v", center, edge)
	}
	if center.R <= center.G {
		t.Fatalf("expected center to be red-dominant, got %#v", center)
	}
	if edge.G <= edge.R {
		t.Fatalf("expected edge to be green-dominant, got %#v", edge)
	}
}

func TestDrawGradientSegmentsPaintsSweepGradientInsideMask(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 24, 16))
	segments := sfnt.Segments{
		{Op: sfnt.SegmentOpMoveTo, Args: [3]fixed.Point26_6{fixed.P(2, -12)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(18, -12)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(18, 0)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(2, 0)}},
		{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{fixed.P(2, -12)}},
	}
	layer := COLRLayer{
		Fill:      COLRFillSweepGradient,
		Transform: identityCOLRTransform(),
		SweepGradient: COLRSweepGradient{
			CenterX: 10, CenterY: -6, StartAngle: 0, EndAngle: 3.141592653589793,
			Stops: []COLRColorStop{
				{Offset: 0, Color: color.RGBA{R: 255, A: 255}},
				{Offset: 1, Color: color.RGBA{G: 255, A: 255}},
			},
		},
	}
	drawGradientSegments(img, segments, layer, 0, 14)
	right := img.RGBAAt(16, 8)
	left := img.RGBAAt(4, 8)
	if right.A == 0 || left.A == 0 {
		t.Fatalf("expected sweep gradient to paint inside mask, right=%#v left=%#v", right, left)
	}
	if right.R <= right.G {
		t.Fatalf("expected right side to be red-dominant, got %#v", right)
	}
	if left.G <= left.R {
		t.Fatalf("expected left side to be green-dominant, got %#v", left)
	}
}
