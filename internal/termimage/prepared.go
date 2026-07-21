package termimage

import "sync/atomic"

// PreparedStoreState is an opaque complete replacement bound to one exact source state.
type PreparedStoreState struct {
	store          *Store
	base           *storeState
	epoch          StoreEpoch
	next           *storeState
	newResource    *resource
	retired        []*resource
	resetEpoch     StoreEpoch
	pending        []*CandidateTransfer
	placements     []*PlacementReservation
	nextPending    map[TransferID]*CandidateTransfer
	nextPlacements map[*PlacementReservation]struct{}
	candidates     []*DecodedCandidate
	nextCandidates map[*DecodedCandidate]struct{}
	published      atomic.Bool
	finished       atomic.Bool
}

func (s *Store) PrepareCandidate(candidate *DecodedCandidate) (*PreparedStoreState, ResourceRef, error) {
	if s != nil && s.owner.Load() != nil {
		return nil, ResourceRef{}, ErrPreparedState
	}
	return s.prepareCandidate(candidate)
}

func (s *Store) prepareCandidate(candidate *DecodedCandidate) (*PreparedStoreState, ResourceRef, error) {
	if s == nil || s.closed.Load() || s.resetting.Load() || candidate == nil || !candidate.ValidFor(s) {
		return nil, ResourceRef{}, ErrCandidateInvalid
	}
	if s.prepared != nil {
		return nil, ResourceRef{}, ErrPreparedState
	}
	ref, err := s.prepareNextRef(candidate.image)
	if err != nil {
		candidate.Close()
		return nil, ResourceRef{}, err
	}
	resources := cloneResources(s.state.resources)
	pixels, lease, ok := candidate.claimOwnership()
	if !ok {
		return nil, ResourceRef{}, ErrCandidateInvalid
	}
	created := &resource{ref: ref, width: candidate.width, height: candidate.height, stride: candidate.stride, rgba: pixels, lease: lease}
	old := resources[candidate.image]
	resources[candidate.image] = created
	prepared := &PreparedStoreState{
		store: s, base: s.state, epoch: StoreEpoch(s.epoch.Load()),
		next:        &storeState{resources: resources, nextGeneration: ref.Generation},
		newResource: created,
	}
	if old != nil {
		prepared.retired = append(prepared.retired, old)
	}
	s.prepared = prepared
	return prepared, ref, nil
}

func (s *Store) PrepareResourceRemoval(refs []ResourceRef) (*PreparedStoreState, error) {
	if s != nil && s.owner.Load() != nil {
		return nil, ErrPreparedState
	}
	return s.prepareResourceRemoval(refs)
}

func (s *Store) prepareResourceRemoval(refs []ResourceRef) (*PreparedStoreState, error) {
	if s == nil || s.closed.Load() || s.resetting.Load() {
		return nil, ErrClosed
	}
	if s.prepared != nil {
		return nil, ErrPreparedState
	}
	resources := cloneResources(s.state.resources)
	prepared := &PreparedStoreState{
		store: s, base: s.state, epoch: StoreEpoch(s.epoch.Load()),
		next: &storeState{resources: resources, nextGeneration: s.state.nextGeneration},
	}
	seen := make(map[ResourceRef]struct{}, len(refs))
	for _, ref := range refs {
		if ref.Image == 0 || ref.Generation == 0 {
			return nil, ErrInvalidID
		}
		if _, duplicate := seen[ref]; duplicate {
			continue
		}
		seen[ref] = struct{}{}
		stored := resources[ref.Image]
		if stored == nil || stored.ref != ref {
			continue
		}
		delete(resources, ref.Image)
		prepared.retired = append(prepared.retired, stored)
	}
	s.prepared = prepared
	return prepared, nil
}

func (s *Store) prepareReset() (*PreparedStoreState, error) {
	if s == nil || s.closed.Load() {
		return nil, ErrClosed
	}
	if !s.resetting.CompareAndSwap(false, true) {
		return nil, ErrPreparedState
	}
	s.abortPrepared()
	currentEpoch := StoreEpoch(s.epoch.Load())
	if currentEpoch == StoreEpoch(^uint64(0)) {
		s.resetting.Store(false)
		return nil, ErrGenerationExhausted
	}
	prepared := &PreparedStoreState{
		store: s, base: s.state, epoch: currentEpoch, resetEpoch: currentEpoch + 1,
		next:           &storeState{resources: make(map[ImageID]*resource), nextGeneration: s.state.nextGeneration},
		nextPending:    make(map[TransferID]*CandidateTransfer),
		nextPlacements: make(map[*PlacementReservation]struct{}),
		nextCandidates: make(map[*DecodedCandidate]struct{}),
	}
	for _, stored := range s.state.resources {
		prepared.retired = append(prepared.retired, stored)
	}
	s.pendingMu.Lock()
	for _, transfer := range s.pending {
		prepared.pending = append(prepared.pending, transfer)
	}
	s.pendingMu.Unlock()
	s.placementMu.Lock()
	for placement := range s.placements {
		prepared.placements = append(prepared.placements, placement)
	}
	s.placementMu.Unlock()
	s.candidateMu.Lock()
	for candidate := range s.candidates {
		prepared.candidates = append(prepared.candidates, candidate)
	}
	s.candidateMu.Unlock()
	s.prepared = prepared
	return prepared, nil
}

func cloneResources(source map[ImageID]*resource) map[ImageID]*resource {
	result := make(map[ImageID]*resource, len(source)+1)
	for id, stored := range source {
		result[id] = stored
	}
	return result
}

// PublishPrepared performs one infallible swap after validating owner-thread invariants.
func (s *Store) PublishPrepared(prepared *PreparedStoreState) {
	if s != nil && s.owner.Load() != nil {
		panic(ErrPreparedState)
	}
	s.publishPrepared(prepared)
}

func (s *Store) publishPrepared(prepared *PreparedStoreState) {
	if prepared == nil || prepared.store != s || s.prepared != prepared || s.closed.Load() ||
		s.state != prepared.base || StoreEpoch(s.epoch.Load()) != prepared.epoch || prepared.published.Load() {
		panic(ErrPreparedState)
	}
	prepared.published.Store(true)
	s.state = prepared.next
	s.prepared = nil
	if prepared.resetEpoch != 0 {
		s.epoch.Store(uint64(prepared.resetEpoch))
		s.pendingMu.Lock()
		s.pending = prepared.nextPending
		s.pendingMu.Unlock()
		s.placementMu.Lock()
		s.placements = prepared.nextPlacements
		s.placementMu.Unlock()
		s.candidateMu.Lock()
		s.candidates = prepared.nextCandidates
		s.candidateMu.Unlock()
	}
}

func (p *PreparedStoreState) Finalize() {
	if p == nil || !p.published.Load() || !p.finished.CompareAndSwap(false, true) {
		return
	}
	for _, stored := range p.retired {
		stored.lease.Close()
	}
	p.retired = nil
	for _, transfer := range p.pending {
		transfer.Close()
	}
	for _, placement := range p.placements {
		placement.Close()
	}
	p.pending, p.placements = nil, nil
	for _, candidate := range p.candidates {
		candidate.Close()
	}
	p.candidates = nil
	if p.resetEpoch != 0 {
		p.store.resetting.Store(false)
	}
}

func (p *PreparedStoreState) Abort() {
	if p == nil || p.published.Load() || !p.finished.CompareAndSwap(false, true) {
		return
	}
	if p.store != nil && p.store.prepared == p {
		p.store.prepared = nil
	}
	if p.resetEpoch != 0 {
		p.store.resetting.Store(false)
	}
	p.abortOwnership()
}

func (p *PreparedStoreState) abortOwnership() {
	if p.newResource != nil {
		p.newResource.lease.Close()
		p.newResource = nil
	}
	p.retired = nil
}

func (s *Store) abortPrepared() {
	prepared := s.prepared
	if prepared == nil {
		return
	}
	s.prepared = nil
	if prepared.finished.CompareAndSwap(false, true) {
		prepared.abortOwnership()
		if prepared.resetEpoch != 0 {
			s.resetting.Store(false)
		}
	}
}

func (s *Store) ResourceRef(image ImageID) (ResourceRef, bool) {
	if s == nil || s.closed.Load() {
		return ResourceRef{}, false
	}
	stored := s.state.resources[image]
	if stored == nil {
		return ResourceRef{}, false
	}
	return stored.ref, true
}

func (s *Store) ResourceRefs() []ResourceRef {
	if s == nil || s.closed.Load() {
		return nil
	}
	result := make([]ResourceRef, 0, len(s.state.resources))
	for _, stored := range s.state.resources {
		result = append(result, stored.ref)
	}
	return result
}

type PlacementReservation struct {
	store  *Store
	lease  *reservation
	closed atomic.Bool
}

func (s *Store) ReservePlacements(count uint64) (*PlacementReservation, error) {
	if s == nil || s.closed.Load() || s.resetting.Load() || count == 0 {
		return nil, ErrInvalidPlacement
	}
	lease, err := reserve(s.process, &s.pane, Usage{Placements: count})
	if err != nil {
		return nil, err
	}
	reservation := &PlacementReservation{store: s, lease: lease}
	s.placementMu.Lock()
	if s.closed.Load() || s.resetting.Load() {
		s.placementMu.Unlock()
		lease.Close()
		return nil, ErrClosed
	}
	s.placements[reservation] = struct{}{}
	s.placementMu.Unlock()
	return reservation, nil
}

func (r *PlacementReservation) Close() {
	if r == nil || !r.closed.CompareAndSwap(false, true) {
		return
	}
	if r.store != nil {
		r.store.placementMu.Lock()
		delete(r.store.placements, r)
		r.store.placementMu.Unlock()
	}
	r.lease.Close()
}

func (s *Store) closePlacementReservations() {
	s.placementMu.Lock()
	reservations := make([]*PlacementReservation, 0, len(s.placements))
	for reservation := range s.placements {
		reservations = append(reservations, reservation)
	}
	s.placements = make(map[*PlacementReservation]struct{})
	s.placementMu.Unlock()
	for _, reservation := range reservations {
		if reservation.closed.CompareAndSwap(false, true) {
			reservation.lease.Close()
		}
	}
}
