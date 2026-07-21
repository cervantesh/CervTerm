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

// ImageProjection returns detached active-screen metadata. Owner-thread only.
func (t *Terminal) ImageProjection(viewportTop, rows int) termimage.Projection {
	if t == nil || t.imageSidecars == nil || rows <= 0 {
		return termimage.Projection{}
	}
	rows = min(rows, t.rows)
	if !t.alternateScreen && viewportTop < 0 {
		return termimage.Projection{}
	}
	source := t.imageSidecars.primary
	if t.alternateScreen {
		source, viewportTop = t.imageSidecars.alternate, 0
	}
	top := int64(viewportTop)
	if top > math.MaxInt64-int64(rows) {
		return termimage.Projection{}
	}
	viewport := imageCellRect{top: top, bottom: top + int64(rows), left: 0, right: uint32(t.cols)}
	projection := termimage.Projection{Generation: t.imageSidecars.generation}
	for _, entry := range source {
		if !imageRectsIntersect(placementRect(entry.placement), viewport) {
			continue
		}
		placement := entry.placement
		placement.Anchor.Row -= int64(viewportTop)
		if placement.Crop != nil {
			crop := *placement.Crop
			placement.Crop = &crop
		}
		projection.Placements = append(projection.Placements, placement)
	}
	return projection
}
