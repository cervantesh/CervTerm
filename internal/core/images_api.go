package core

import (
	"errors"
	"math"
	"sync"

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
	Retention termimage.ResourceRetention
}

type ImageCommitResult struct {
	Resource  termimage.ResourceRef
	Placement *termimage.PlacementID
}

// ImageCursorAnchor returns the cell-space anchor for an image placed at the
// current cursor. Primary-screen rows include retained scrollback; alternate
// rows are top-relative. Owner-thread callers capture the returned value before
// asynchronous work so later cursor or screen changes cannot rewrite intent.
func (t *Terminal) ImageCursorAnchor() termimage.CellAnchor {
	if t == nil {
		return termimage.CellAnchor{}
	}
	row := t.cursorRow
	if !t.alternateScreen {
		row += t.scrollbackRows
	}
	return termimage.CellAnchor{Row: int64(row), Col: uint32(t.cursorCol)}
}

// ImageAnchorGeneration changes whenever terminal content or screen lifecycle
// could rebase, erase, or reroute an anchor captured before asynchronous decode.
// Cursor-only movement intentionally does not change it.
func (t *Terminal) ImageAnchorGeneration() uint64 {
	if t == nil {
		return 0
	}
	return t.imageAnchorGeneration
}

// ImageGeneration returns the current owner-thread image sidecar generation.
// Unlike a render snapshot, this accessor reflects commits and resets performed
// since the pane's last capture.
func (t *Terminal) ImageGeneration() uint64 {
	if t == nil || t.imageSidecars == nil {
		return 0
	}
	return t.imageSidecars.generation
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
		if commit.Candidate != nil || commit.Placement == nil || commit.Retention != termimage.ResourceDurable {
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
	result, err := t.commitImage(imageCommit{candidate: commit.Candidate, placement: commit.Placement, retention: commit.Retention})
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
func (t *Terminal) ResetImages() error {
	if t == nil {
		return nil
	}
	return t.resetImages()
}

// PreparedImageStoreClose binds the exact store, owner, and sidecar publication
// observed during close preflight. Commit clears core ownership only after the
// retained StoreOwner transaction has closed the store. Resolution is serialized
// here rather than on Terminal so normal owner-thread terminal paths stay lock-free.
type PreparedImageStoreClose struct {
	resolveMu sync.Mutex

	terminal *Terminal
	store    *termimage.Store
	owner    *termimage.StoreOwner
	sidecars *imageSidecars
	prepared *termimage.PreparedStoreClose
	finished bool

	afterStoreCommit func() // package-private deterministic transaction seam
}

// PrepareCloseImageStore rejects wrong, stale, closed, busy, or wrong-thread
// ownership before callers detach mux/core state. Every successful preflight
// must be resolved by Commit or Abort on the owner thread.
func (t *Terminal) PrepareCloseImageStore() (*PreparedImageStoreClose, error) {
	prepared := &PreparedImageStoreClose{terminal: t}
	if t == nil || t.imageStore == nil {
		return prepared, nil
	}
	if t.imageOwner == nil {
		return nil, termimage.ErrWrongOwner
	}
	prepared.store, prepared.owner, prepared.sidecars = t.imageStore, t.imageOwner, t.imageSidecars
	storeClose, err := t.imageOwner.PrepareClose(t.imageStore)
	if err != nil {
		return nil, err
	}
	prepared.prepared = storeClose
	return prepared, nil
}

func (p *PreparedImageStoreClose) matchesTerminalState() bool {
	if p.terminal == nil {
		return p.store == nil && p.owner == nil && p.sidecars == nil && p.prepared == nil
	}
	return p.terminal.imageStore == p.store &&
		p.terminal.imageOwner == p.owner &&
		p.terminal.imageSidecars == p.sidecars
}

func (p *PreparedImageStoreClose) Commit() error {
	if p == nil {
		return termimage.ErrWrongOwner
	}
	p.resolveMu.Lock()
	defer p.resolveMu.Unlock()

	if p.finished {
		return nil
	}
	if !p.matchesTerminalState() {
		return termimage.ErrPreparedState
	}
	if p.store == nil {
		p.finished = true
		return nil
	}
	if p.prepared == nil {
		return termimage.ErrPreparedState
	}
	if err := p.prepared.Commit(); err != nil {
		return err
	}
	if p.afterStoreCommit != nil {
		p.afterStoreCommit()
	}
	p.terminal.imageStore = nil
	p.terminal.imageOwner = nil
	p.terminal.imageSidecars = nil
	p.finished = true
	return nil
}

func (p *PreparedImageStoreClose) Abort() error {
	if p == nil {
		return termimage.ErrWrongOwner
	}
	p.resolveMu.Lock()
	defer p.resolveMu.Unlock()

	if p.finished {
		return nil
	}
	if !p.matchesTerminalState() {
		return termimage.ErrPreparedState
	}
	if p.store == nil {
		p.finished = true
		return nil
	}
	if p.prepared == nil {
		return termimage.ErrPreparedState
	}
	if err := p.prepared.Abort(); err != nil {
		return err
	}
	p.finished = true
	return nil
}

// CloseImageStore releases the terminal's attached image owner exactly once.
// A rejected close retains the store, sidecars, and owner so it can be retried.
func (t *Terminal) CloseImageStore() error {
	if t == nil {
		return nil
	}
	return t.closeImages()
}

// ValidateCloseImageStore performs and aborts the same retained preflight used
// by mux close transactions. It never releases image ownership.
func (t *Terminal) ValidateCloseImageStore() error {
	prepared, err := t.PrepareCloseImageStore()
	if err != nil {
		return err
	}
	return prepared.Abort()
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
