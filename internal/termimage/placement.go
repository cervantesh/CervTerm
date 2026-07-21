package termimage

func NewPlacement(spec PlacementSpec, resource ResourceRef, width, height uint32) (Placement, error) {
	validated, err := ValidatePlacementSpec(spec, width, height)
	if err != nil {
		return Placement{}, err
	}
	if resource.Image == 0 || resource.Generation == 0 {
		return Placement{}, ErrInvalidID
	}
	return Placement{
		ID: validated.ID, Resource: resource, Anchor: validated.Anchor,
		Cols: validated.Cols, Rows: validated.Rows, Crop: validated.Crop,
		Z: validated.Z, Opacity: validated.Opacity,
	}, nil
}

func ValidatePlacementSpec(spec PlacementSpec, resourceWidth, resourceHeight uint32) (PlacementSpec, error) {
	if spec.ID == 0 || spec.Anchor.Row < 0 || spec.Cols == 0 || spec.Rows == 0 ||
		spec.Cols > HardPlacementSpan || spec.Rows > HardPlacementSpan {
		return PlacementSpec{}, ErrInvalidPlacement
	}
	validated := spec
	if spec.Crop != nil {
		crop := *spec.Crop
		if crop.Width == 0 || crop.Height == 0 || crop.X > resourceWidth || crop.Y > resourceHeight ||
			crop.Width > resourceWidth-crop.X || crop.Height > resourceHeight-crop.Y {
			return PlacementSpec{}, ErrInvalidCrop
		}
		validated.Crop = &crop
	}
	return validated, nil
}

func ValidateDeleteSelector(selector DeleteSelector) (DeleteSelector, error) {
	modes := 0
	if selector.All {
		modes++
	}
	if selector.Placement != nil {
		modes++
	}
	if selector.Image != nil {
		modes++
	}
	if selector.UnderCursor {
		modes++
	}
	if modes != 1 || (selector.Placement != nil && selector.CurrentScreen) {
		return DeleteSelector{}, ErrInvalidSelector
	}
	validated := selector
	if selector.Placement != nil {
		if *selector.Placement == 0 {
			return DeleteSelector{}, ErrInvalidSelector
		}
		id := *selector.Placement
		validated.Placement = &id
	}
	if selector.Image != nil {
		if *selector.Image == 0 {
			return DeleteSelector{}, ErrInvalidSelector
		}
		id := *selector.Image
		validated.Image = &id
	}
	return validated, nil
}
