package core

import (
	"errors"
	"math"

	"cervterm/internal/termimage"
)

var (
	ErrImageStoreUnavailable = ErrImageStateUnavailable
	ErrImageStoreAttached    = errors.New("terminal image store is already attached")
)

type ImageCommit struct {
	Candidate *termimage.DecodedCandidate
	Existing  *termimage.ResourceRef
	Placement *termimage.PlacementSpec
}

type ImageCommitResult struct {
	Resource  termimage.ResourceRef
	Placement *termimage.PlacementID
}

// AttachImageStore installs the pane-owned store. Terminal mutations are owner-thread only.
func (t *Terminal) AttachImageStore(store *termimage.Store) error {
	if t == nil || store == nil || store.Closed() {
		return ErrImageStoreUnavailable
	}
	if t.imageStore != nil {
		return ErrImageStoreAttached
	}
	owner := store.ClaimOwner()
	if owner == nil {
		return ErrImageStoreAttached
	}
	t.imageStore = store
	t.imageOwner = owner
	t.imageSidecars = &imageSidecars{}
	return nil
}

// CommitImage consumes Candidate on success and on every failure. Owner-thread only.
func (t *Terminal) CommitImage(commit ImageCommit) (ImageCommitResult, error) {
	if t == nil || t.imageStore == nil {
		if commit.Candidate != nil {
			commit.Candidate.Close()
		}
		return ImageCommitResult{}, ErrImageStoreUnavailable
	}
	if commit.Existing != nil {
		if commit.Candidate != nil || commit.Placement == nil {
			if commit.Candidate != nil {
				commit.Candidate.Close()
			}
			return ImageCommitResult{}, termimage.ErrInvalidPlacement
		}
		id, err := t.placeExistingImage(*commit.Existing, *commit.Placement)
		if err != nil {
			return ImageCommitResult{}, err
		}
		return ImageCommitResult{Resource: *commit.Existing, Placement: &id}, nil
	}
	result, err := t.commitImage(imageCommit{candidate: commit.Candidate, placement: commit.Placement})
	return ImageCommitResult{Resource: result.resource, Placement: result.placement}, err
}

// DeleteImages atomically applies a validated selector. Owner-thread only.
func (t *Terminal) DeleteImages(selector termimage.DeleteSelector) (int, error) {
	if t == nil || t.imageStore == nil {
		return 0, ErrImageStoreUnavailable
	}
	return t.deleteImages(selector)
}

// ResetImages atomically advances the store epoch and clears all image ownership.
// It fails closed if the epoch cannot advance. Owner-thread only.
func (t *Terminal) ResetImages() {
	if t != nil {
		t.resetImages()
	}
}

// CloseImageStore releases the terminal's attached image owner exactly once.
func (t *Terminal) CloseImageStore() {
	if t != nil {
		t.closeImages()
	}
}

// CopyImageProjection replaces detached active-screen metadata using reusable storage.
// The returned crop slice backs any non-nil Placement.Crop pointers and must be retained.
func (t *Terminal) CopyImageProjection(dst []termimage.Placement, crops []termimage.PixelRect, viewportTop, rows int) ([]termimage.Placement, []termimage.PixelRect, uint64) {
	dst, crops = dst[:0], crops[:0]
	if t == nil || t.imageSidecars == nil || rows <= 0 {
		return dst, crops, 0
	}
	rows = min(rows, t.rows)
	if !t.alternateScreen && viewportTop < 0 {
		return dst, crops, 0
	}
	source := t.imageSidecars.primary
	if t.alternateScreen {
		source, viewportTop = t.imageSidecars.alternate, 0
	}
	top := int64(viewportTop)
	if top > math.MaxInt64-int64(rows) {
		return dst, crops, 0
	}
	if cap(crops) < len(source) {
		crops = make([]termimage.PixelRect, 0, len(source))
	}
	viewport := imageCellRect{top: top, bottom: top + int64(rows), left: 0, right: uint32(t.cols)}
	for _, entry := range source {
		if !imageRectsIntersect(placementRect(entry.placement), viewport) {
			continue
		}
		placement := entry.placement
		placement.Anchor.Row -= int64(viewportTop)
		if placement.Crop != nil {
			crops = append(crops, *placement.Crop)
			placement.Crop = &crops[len(crops)-1]
		}
		dst = append(dst, placement)
	}
	return dst, crops, t.imageSidecars.generation
}

// ImageProjection returns detached active-screen metadata. Owner-thread only.
func (t *Terminal) ImageProjection(viewportTop, rows int) termimage.Projection {
	placements, _, generation := t.CopyImageProjection(nil, nil, viewportTop, rows)
	return termimage.Projection{Placements: placements, Generation: generation}
}
