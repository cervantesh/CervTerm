package core

import (
	"errors"
	"math"

	"cervterm/internal/termimage"
)

var errImageStateUnavailable = errors.New("terminal image state unavailable")

type imagePlacement struct {
	placement termimage.Placement
	lease     *termimage.PlacementReservation
}

type imageSidecars struct {
	primary    []imagePlacement
	alternate  []imagePlacement
	generation uint64
}

type imageCommit struct {
	candidate *termimage.DecodedCandidate
	placement *termimage.PlacementSpec
}

type imageCommitResult struct {
	resource  termimage.ResourceRef
	placement *termimage.PlacementID
}

type imagePrepareStep uint8

const (
	imagePrepareValidate imagePrepareStep = iota + 1
	imagePreparePrimaryCopy
	imagePrepareAlternateCopy
	imagePreparePlacement
	imagePrepareReservation
	imagePrepareStore
)

type imagePrepareFault func(imagePrepareStep) error

type preparedImageMutation struct {
	terminal     *Terminal
	store        *termimage.PreparedStoreState
	sidecars     *imageSidecars
	baseSidecars *imageSidecars
	retired      []*termimage.PlacementReservation
	newLease     *termimage.PlacementReservation
	published    bool
}

func (t *Terminal) commitImage(commit imageCommit) (imageCommitResult, error) {
	prepared, result, err := t.prepareImageCommit(commit, nil)
	if err != nil {
		return imageCommitResult{}, err
	}
	t.publishPreparedImage(prepared)
	return result, nil
}

func (t *Terminal) prepareImageCommit(commit imageCommit, fault imagePrepareFault) (_ *preparedImageMutation, _ imageCommitResult, err error) {
	candidate := commit.candidate
	if t == nil || t.imageStore == nil || t.imageSidecars == nil || candidate == nil || !candidate.ValidFor(t.imageStore) {
		if candidate != nil {
			candidate.Close()
		}
		return nil, imageCommitResult{}, errImageStateUnavailable
	}
	failed := true
	defer func() {
		if failed {
			candidate.Close()
		}
	}()
	if t.imageSidecars.generation == math.MaxUint64 {
		return nil, imageCommitResult{}, termimage.ErrGenerationExhausted
	}
	if err := runImageFault(fault, imagePrepareValidate); err != nil {
		return nil, imageCommitResult{}, err
	}
	width, height, stride := candidate.Dimensions()
	if width == 0 || height == 0 || stride == 0 {
		return nil, imageCommitResult{}, termimage.ErrCandidateInvalid
	}
	oldRef, replacing := t.imageStore.ResourceRef(candidate.Image())
	if err := runImageFault(fault, imagePreparePrimaryCopy); err != nil {
		return nil, imageCommitResult{}, err
	}
	primary, retiredPrimary := cloneImagePlacements(t.imageSidecars.primary, oldRef, replacing)
	if err := runImageFault(fault, imagePrepareAlternateCopy); err != nil {
		return nil, imageCommitResult{}, err
	}
	alternate, retiredAlternate := cloneImagePlacements(t.imageSidecars.alternate, oldRef, replacing)
	retired := append(retiredPrimary, retiredAlternate...)

	var placementSpec *termimage.PlacementSpec
	if commit.placement != nil {
		if err := runImageFault(fault, imagePreparePlacement); err != nil {
			return nil, imageCommitResult{}, err
		}
		validated, validateErr := termimage.ValidatePlacementSpec(*commit.placement, width, height)
		if validateErr != nil {
			return nil, imageCommitResult{}, validateErr
		}
		if validateErr = t.validatePlacementCoordinates(validated); validateErr != nil {
			return nil, imageCommitResult{}, validateErr
		}
		if placementIDExists(primary, validated.ID) || placementIDExists(alternate, validated.ID) {
			return nil, imageCommitResult{}, termimage.ErrInvalidPlacement
		}
		placementSpec = &validated
	}

	var placementLease, newLease *termimage.PlacementReservation
	if placementSpec != nil {
		if err := runImageFault(fault, imagePrepareReservation); err != nil {
			return nil, imageCommitResult{}, err
		}
		if len(retired) > 0 {
			placementLease = retired[len(retired)-1]
			retired = retired[:len(retired)-1]
		} else {
			placementLease, err = t.imageStore.ReservePlacements(1)
			if err != nil {
				return nil, imageCommitResult{}, err
			}
			newLease = placementLease
		}
	}
	nextSidecars := &imageSidecars{primary: primary, alternate: alternate, generation: t.imageSidecars.generation + 1}
	if err := runImageFault(fault, imagePrepareStore); err != nil {
		if newLease != nil {
			newLease.Close()
		}
		return nil, imageCommitResult{}, err
	}
	storePrepared, ref, err := t.imageStore.PrepareCandidate(candidate)
	if err != nil {
		if newLease != nil {
			newLease.Close()
		}
		return nil, imageCommitResult{}, err
	}
	result := imageCommitResult{resource: ref}
	if placementSpec != nil {
		placement, placementErr := termimage.NewPlacement(*placementSpec, ref, width, height)
		if placementErr != nil { // Validation already succeeded; retain rollback safety.
			storePrepared.Abort()
			if newLease != nil {
				newLease.Close()
			}
			return nil, imageCommitResult{}, placementErr
		}
		entry := imagePlacement{placement: placement, lease: placementLease}
		if t.alternateScreen {
			alternate = append(alternate, entry)
		} else {
			primary = append(primary, entry)
		}
		id := placement.ID
		result.placement = &id
	}
	failed = false
	nextSidecars.primary, nextSidecars.alternate = primary, alternate
	return &preparedImageMutation{
		terminal: t, store: storePrepared, sidecars: nextSidecars, baseSidecars: t.imageSidecars,
		retired: retired, newLease: newLease,
	}, result, nil
}

func (t *Terminal) publishPreparedImage(prepared *preparedImageMutation) {
	if prepared == nil || prepared.terminal != t || prepared.published {
		panic(termimage.ErrPreparedState)
	}
	if t.imageStore == nil || t.imageSidecars != prepared.baseSidecars {
		t.abortPreparedImage(prepared)
		panic(termimage.ErrPreparedState)
	}
	t.imageStore.PublishPrepared(prepared.store)
	t.imageSidecars = prepared.sidecars
	prepared.published = true
	prepared.store.Finalize()
	for _, lease := range prepared.retired {
		lease.Close()
	}
}

func (t *Terminal) abortPreparedImage(prepared *preparedImageMutation) {
	if prepared == nil || prepared.terminal != t || prepared.published {
		return
	}
	prepared.store.Abort()
	if prepared.newLease != nil {
		prepared.newLease.Close()
	}
	prepared.newLease = nil
}

func (t *Terminal) resetImages() {
	if t == nil || t.imageStore == nil {
		return
	}
	if t.imageSidecars != nil && t.imageSidecars.generation == math.MaxUint64 {
		t.closeImages()
		return
	}
	nextGeneration := uint64(1)
	if t.imageSidecars != nil {
		nextGeneration = t.imageSidecars.generation + 1
	}
	t.imageStore.Reset()
	t.imageSidecars = &imageSidecars{generation: nextGeneration}
}

func (t *Terminal) closeImages() {
	if t == nil || t.imageStore == nil {
		return
	}
	t.imageStore.Close()
	t.imageStore = nil
	t.imageSidecars = nil
}

func (t *Terminal) deleteImages(selector termimage.DeleteSelector) (int, error) {
	prepared, count, err := t.prepareImageDelete(selector, nil)
	if err != nil {
		return 0, err
	}
	t.publishPreparedImage(prepared)
	return count, nil
}

func (t *Terminal) prepareImageDelete(selector termimage.DeleteSelector, fault imagePrepareFault) (*preparedImageMutation, int, error) {
	if t == nil || t.imageStore == nil || t.imageSidecars == nil {
		return nil, 0, errImageStateUnavailable
	}
	if t.imageSidecars.generation == math.MaxUint64 {
		return nil, 0, termimage.ErrGenerationExhausted
	}
	if err := runImageFault(fault, imagePrepareValidate); err != nil {
		return nil, 0, err
	}
	validated, err := termimage.ValidateDeleteSelector(selector)
	if err != nil {
		return nil, 0, err
	}
	if err = runImageFault(fault, imagePreparePrimaryCopy); err != nil {
		return nil, 0, err
	}
	primary := append([]imagePlacement(nil), t.imageSidecars.primary...)
	if err = runImageFault(fault, imagePrepareAlternateCopy); err != nil {
		return nil, 0, err
	}
	alternate := append([]imagePlacement(nil), t.imageSidecars.alternate...)
	refs := make(map[termimage.ResourceRef]struct{})
	matches := func(entry imagePlacement, alternateSide bool) bool {
		if validated.CurrentScreen && alternateSide != t.alternateScreen {
			return false
		}
		placement := entry.placement
		switch {
		case validated.All:
			return true
		case validated.Placement != nil:
			return placement.ID == *validated.Placement
		case validated.Image != nil:
			return placement.Resource.Image == *validated.Image
		case validated.UnderCursor:
			row := int64(t.cursorRow)
			if !alternateSide {
				row += int64(t.scrollbackRows)
			}
			return row >= placement.Anchor.Row && row < placement.Anchor.Row+int64(placement.Rows) &&
				uint32(t.cursorCol) >= placement.Anchor.Col && uint32(t.cursorCol) < placement.Anchor.Col+uint32(placement.Cols)
		}
		return false
	}
	for _, entry := range primary {
		if matches(entry, false) {
			refs[entry.placement.Resource] = struct{}{}
		}
	}
	for _, entry := range alternate {
		if matches(entry, true) {
			refs[entry.placement.Resource] = struct{}{}
		}
	}
	if validated.DeleteResource && !validated.CurrentScreen {
		if validated.All {
			for _, ref := range t.imageStore.ResourceRefs() {
				refs[ref] = struct{}{}
			}
		}
		if validated.Image != nil {
			if ref, ok := t.imageStore.ResourceRef(*validated.Image); ok {
				refs[ref] = struct{}{}
			}
		}
	}
	var retired []*termimage.PlacementReservation
	filter := func(source []imagePlacement, alternateSide bool) []imagePlacement {
		result := source[:0]
		for _, entry := range source {
			remove := matches(entry, alternateSide)
			if validated.DeleteResource {
				_, remove = refs[entry.placement.Resource]
			}
			if remove {
				retired = append(retired, entry.lease)
			} else {
				result = append(result, entry)
			}
		}
		return result
	}
	primary = filter(primary, false)
	alternate = filter(alternate, true)
	removeRefs := make([]termimage.ResourceRef, 0, len(refs))
	if validated.DeleteResource {
		for ref := range refs {
			removeRefs = append(removeRefs, ref)
		}
	}
	if err = runImageFault(fault, imagePrepareStore); err != nil {
		return nil, 0, err
	}
	storePrepared, err := t.imageStore.PrepareResourceRemoval(removeRefs)
	if err != nil {
		return nil, 0, err
	}
	return &preparedImageMutation{
		terminal: t, store: storePrepared, baseSidecars: t.imageSidecars,
		sidecars: &imageSidecars{primary: primary, alternate: alternate, generation: t.imageSidecars.generation + 1},
		retired:  retired,
	}, len(retired), nil
}

func cloneImagePlacements(source []imagePlacement, replaced termimage.ResourceRef, remove bool) ([]imagePlacement, []*termimage.PlacementReservation) {
	result := make([]imagePlacement, 0, len(source)+1)
	var retired []*termimage.PlacementReservation
	for _, entry := range source {
		if remove && entry.placement.Resource == replaced {
			retired = append(retired, entry.lease)
		} else {
			result = append(result, entry)
		}
	}
	return result, retired
}

func placementIDExists(entries []imagePlacement, id termimage.PlacementID) bool {
	for _, entry := range entries {
		if entry.placement.ID == id {
			return true
		}
	}
	return false
}

func (t *Terminal) validatePlacementCoordinates(spec termimage.PlacementSpec) error {
	if spec.Anchor.Col >= uint32(t.cols) {
		return termimage.ErrInvalidPlacement
	}
	maxRow := int64(t.rows)
	if !t.alternateScreen {
		maxRow += int64(t.scrollbackRows)
	}
	if spec.Anchor.Row < 0 || spec.Anchor.Row >= maxRow {
		return termimage.ErrInvalidPlacement
	}
	return nil
}

func runImageFault(fault imagePrepareFault, step imagePrepareStep) error {
	if fault != nil {
		return fault(step)
	}
	return nil
}
