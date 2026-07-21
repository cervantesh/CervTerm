package termimage

import (
	"math"
	"testing"
)

func TestPlacementValidationAndCropCopy(t *testing.T) {
	crop := PixelRect{X: 1, Y: 2, Width: 3, Height: 4}
	spec := PlacementSpec{ID: 1, Anchor: CellAnchor{Row: 0, Col: 2}, Cols: 1, Rows: 256, Crop: &crop, Opacity: 255}
	validated, err := ValidatePlacementSpec(spec, 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	crop.X = 9
	if validated.Crop == nil || validated.Crop.X != 1 {
		t.Fatal("crop pointer was retained")
	}
	placement, err := NewPlacement(validated, ResourceRef{Image: 1, Generation: 2}, 10, 10)
	if err != nil || placement.Resource.Generation != 2 {
		t.Fatalf("placement=%#v err=%v", placement, err)
	}
}

func TestPlacementValidationRejectsBoundsAndOverflow(t *testing.T) {
	valid := PlacementSpec{ID: 1, Cols: 1, Rows: 1}
	for name, mutate := range map[string]func(*PlacementSpec){
		"zero id":         func(s *PlacementSpec) { s.ID = 0 },
		"negative row":    func(s *PlacementSpec) { s.Anchor.Row = -1 },
		"zero cols":       func(s *PlacementSpec) { s.Cols = 0 },
		"wide span":       func(s *PlacementSpec) { s.Cols = HardPlacementSpan + 1 },
		"tall span":       func(s *PlacementSpec) { s.Rows = HardPlacementSpan + 1 },
		"zero crop":       func(s *PlacementSpec) { s.Crop = &PixelRect{} },
		"crop x overflow": func(s *PlacementSpec) { s.Crop = &PixelRect{X: math.MaxUint32, Width: 2, Height: 1} },
		"crop y overflow": func(s *PlacementSpec) { s.Crop = &PixelRect{Y: math.MaxUint32, Width: 1, Height: 2} },
	} {
		t.Run(name, func(t *testing.T) {
			spec := valid
			mutate(&spec)
			if _, err := ValidatePlacementSpec(spec, 10, 10); err == nil {
				t.Fatal("invalid placement accepted")
			}
		})
	}
}

func TestDeleteSelectorTruthTableAndCopiesIDs(t *testing.T) {
	placementID := PlacementID(1)
	imageID := ImageID(2)
	valid := []DeleteSelector{{All: true}, {Placement: &placementID}, {Image: &imageID}, {UnderCursor: true}, {All: true, CurrentScreen: true, DeleteResource: true}}
	for _, selector := range valid {
		validated, err := ValidateDeleteSelector(selector)
		if err != nil {
			t.Fatalf("valid selector %#v: %v", selector, err)
		}
		if selector.Placement != nil {
			*selector.Placement = 9
			if *validated.Placement != 1 {
				t.Fatal("placement ID pointer retained")
			}
		}
	}
	zeroPlacement := PlacementID(0)
	zeroImage := ImageID(0)
	invalid := []DeleteSelector{{}, {All: true, UnderCursor: true}, {Placement: &placementID, Image: &imageID}, {Placement: &placementID, CurrentScreen: true}, {Placement: &zeroPlacement}, {Image: &zeroImage}}
	for _, selector := range invalid {
		if _, err := ValidateDeleteSelector(selector); err == nil {
			t.Fatalf("invalid selector accepted: %#v", selector)
		}
	}
}

func FuzzPlacementCropValidation(f *testing.F) {
	f.Add(uint64(1), int64(0), uint32(0), uint32(1), uint32(1), uint32(0), uint32(0), uint32(1), uint32(1))
	f.Fuzz(func(t *testing.T, id uint64, row int64, col, cols, rows, x, y, width, height uint32) {
		spec := PlacementSpec{ID: PlacementID(id), Anchor: CellAnchor{Row: row, Col: col}, Cols: uint16(cols), Rows: uint16(rows), Crop: &PixelRect{X: x, Y: y, Width: width, Height: height}}
		validated, err := ValidatePlacementSpec(spec, 4096, 4096)
		if err == nil {
			if _, err = NewPlacement(validated, ResourceRef{Image: 1, Generation: 1}, 4096, 4096); err != nil {
				t.Fatalf("validated placement rejected: %v", err)
			}
		}
	})
}

func FuzzDeleteSelectorTruthTable(f *testing.F) {
	f.Add(byte(1), uint64(1), uint64(1))
	f.Fuzz(func(t *testing.T, bits byte, placementValue, imageValue uint64) {
		placement, image := PlacementID(placementValue), ImageID(imageValue)
		selector := DeleteSelector{All: bits&1 != 0, UnderCursor: bits&2 != 0, CurrentScreen: bits&4 != 0, DeleteResource: bits&8 != 0}
		if bits&16 != 0 {
			selector.Placement = &placement
		}
		if bits&32 != 0 {
			selector.Image = &image
		}
		validated, err := ValidateDeleteSelector(selector)
		if err == nil {
			if _, secondErr := ValidateDeleteSelector(validated); secondErr != nil {
				t.Fatalf("validated selector rejected: %v", secondErr)
			}
		}
	})
}
