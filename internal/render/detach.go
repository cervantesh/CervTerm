package render

import (
	"cervterm/internal/core"
	"cervterm/internal/termimage"
)

// DetachedSnapshot returns a recursively detached public value without retaining
// the reusable capture scratch owned by source.
func DetachedSnapshot(source Snapshot) Snapshot {
	result := source
	if source.Cells != nil {
		result.Cells = make([]core.Cell, len(source.Cells))
		for index, cell := range source.Cells {
			result.Cells[index] = core.NewCellWithCombining(cell.Rune, cell.Attr, cell.CloneCombining()...)
			result.Cells[index].WideContinuation = cell.WideContinuation
			result.Cells[index].HyperlinkID = cell.HyperlinkID
			result.Cells[index].SemanticKind = cell.SemanticKind
		}
	}
	if source.Wrapped != nil {
		result.Wrapped = append([]bool(nil), source.Wrapped...)
	}
	if source.Hyperlinks != nil {
		result.Hyperlinks = append([]core.Hyperlink(nil), source.Hyperlinks...)
	}
	if source.SemanticZones != nil {
		result.SemanticZones = append([]core.SemanticZone(nil), source.SemanticZones...)
	}
	if source.Images != nil {
		result.Images = make([]ImagePlacement, len(source.Images))
		copy(result.Images, source.Images)
		cropCount := 0
		for _, image := range source.Images {
			if image.Placement.Crop != nil {
				cropCount++
			}
		}
		crops := make([]termimage.PixelRect, cropCount)
		cropIndex := 0
		for index := range result.Images {
			if source.Images[index].Placement.Crop != nil {
				crops[cropIndex] = *source.Images[index].Placement.Crop
				result.Images[index].Placement.Crop = &crops[cropIndex]
				cropIndex++
			}
		}
	}
	result.imagePlacements = nil
	result.imageCrops = nil
	return result
}
