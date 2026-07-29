package termimage

import "sync/atomic"

// PreparedStoreState is an opaque complete replacement bound to one exact source state.
type PreparedStoreState struct {
	base           *storeState
	epoch          StoreEpoch
	owner          *StoreOwner
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

func (s *Store) prepareCandidate(scope storeMutationScope, candidate *DecodedCandidate) (*PreparedStoreState, ResourceRef, error) {
	return s.prepareCandidateWithRetention(scope, candidate, ResourceDurable)
}

func (s *Store) prepareCandidateWithRetention(scope storeMutationScope, candidate *DecodedCandidate, retention ResourceRetention) (*PreparedStoreState, ResourceRef, error) {
	if err := scope.valid(s); err != nil {
		return nil, ResourceRef{}, err
	}
	if s == nil || s.closed.Load() || s.resetting.Load() || candidate == nil || !candidate.ValidFor(s) {
		return nil, ResourceRef{}, ErrCandidateInvalid
	}
	if !retention.Valid() {
		candidate.Close()
		return nil, ResourceRef{}, ErrInvalidRetention
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
	created := &resource{ref: ref, width: candidate.width, height: candidate.height, stride: candidate.stride, rgba: pixels, lease: lease, retention: retention}
	old := resources[candidate.image]
	resources[candidate.image] = created
	prepared := &PreparedStoreState{
		owner: scope.owner, base: s.state, epoch: StoreEpoch(s.epoch.Load()),
		next:        &storeState{resources: resources, nextGeneration: ref.Generation},
		newResource: created,
	}
	if old != nil {
		prepared.retired = append(prepared.retired, old)
	}
	s.prepared = prepared
	return prepared, ref, nil
}

func (s *Store) prepareResourceRemoval(scope storeMutationScope, refs []ResourceRef) (*PreparedStoreState, error) {
	if err := scope.valid(s); err != nil {
		return nil, err
	}
	if s == nil || s.closed.Load() || s.resetting.Load() {
		return nil, ErrClosed
	}
	if s.prepared != nil {
		return nil, ErrPreparedState
	}
	removeSet := make(map[ImageID]*resource, len(refs))
	for _, ref := range refs {
		if ref.Image == 0 || ref.Generation == 0 {
			return nil, ErrInvalidID
		}
		stored := s.state.resources[ref.Image]
		if stored != nil && stored.ref == ref {
			removeSet[ref.Image] = stored
		}
	}
	resources := make(map[ImageID]*resource, len(s.state.resources)-len(removeSet))
	prepared := &PreparedStoreState{
		owner: scope.owner, base: s.state, epoch: StoreEpoch(s.epoch.Load()),
		next: &storeState{resources: resources, nextGeneration: s.state.nextGeneration},
	}
	for image, stored := range s.state.resources {
		if removeSet[image] == stored {
			prepared.retired = append(prepared.retired, stored)
			continue
		}
		resources[image] = stored
	}
	s.prepared = prepared
	return prepared, nil
}

func (s *Store) prepareReset(scope storeMutationScope) (*PreparedStoreState, error) {
	if err := scope.valid(s); err != nil {
		return nil, err
	}
	if s == nil || s.closed.Load() {
		return nil, ErrClosed
	}
	if !s.resetting.CompareAndSwap(false, true) {
		return nil, ErrPreparedState
	}
	if err := s.abortPrepared(scope); err != nil {
		s.resetting.Store(false)
		return nil, err
	}
	currentEpoch := StoreEpoch(s.epoch.Load())
	if currentEpoch == StoreEpoch(^uint64(0)) {
		s.resetting.Store(false)
		return nil, ErrGenerationExhausted
	}
	prepared := &PreparedStoreState{
		owner: scope.owner, base: s.state, epoch: currentEpoch, resetEpoch: currentEpoch + 1,
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

func (s *Store) publishPrepared(scope storeMutationScope, prepared *PreparedStoreState) error {
	if err := scope.valid(s); err != nil {
		return err
	}
	if prepared == nil || prepared.owner != scope.owner ||
		s.prepared != prepared || s.closed.Load() || s.state != prepared.base || StoreEpoch(s.epoch.Load()) != prepared.epoch || prepared.published.Load() {
		return ErrPreparedState
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
	return nil
}

// Commit finalizes a published replacement under a freshly attested owner scope.
func (p *PreparedStoreState) Commit() error { return p.transition(true) }

// Abort abandons an unpublished replacement under a freshly attested owner scope.
func (p *PreparedStoreState) Abort() error { return p.transition(false) }

// Close resolves the prepared transition according to whether it was published.
func (p *PreparedStoreState) Close() error {
	if p != nil && p.published.Load() {
		return p.Commit()
	}
	return p.Abort()
}

func (p *PreparedStoreState) transition(commit bool) error {
	if p == nil || p.owner == nil || p.owner.store == nil || p.owner.generation == 0 {
		return ErrWrongOwner
	}
	if p.finished.Load() {
		return nil
	}
	scope, err := p.owner.enter()
	if err != nil {
		return err
	}
	defer p.owner.leave(scope)
	if commit {
		return p.commit(scope)
	}
	return p.abort(scope)
}

func (p *PreparedStoreState) commit(scope storeMutationScope) error {
	if err := p.validateScope(scope); err != nil {
		return err
	}
	if !p.published.Load() {
		return ErrPreparedState
	}
	if !p.finished.CompareAndSwap(false, true) {
		return nil
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
		p.owner.store.resetting.Store(false)
	}
	return nil
}

func (p *PreparedStoreState) abort(scope storeMutationScope) error {
	if err := p.validateScope(scope); err != nil {
		return err
	}
	if p.published.Load() {
		return ErrPreparedState
	}
	if !p.finished.CompareAndSwap(false, true) {
		return nil
	}
	if p.owner.store.prepared == p {
		p.owner.store.prepared = nil
	}
	if p.resetEpoch != 0 {
		p.owner.store.resetting.Store(false)
	}
	p.abortOwnership(scope)
	return nil
}

func (p *PreparedStoreState) validateScope(scope storeMutationScope) error {
	if p == nil || p.owner == nil || p.owner.store == nil {
		return ErrWrongOwner
	}
	if err := scope.valid(p.owner.store); err != nil {
		return err
	}
	if p.owner != scope.owner {
		return ErrStaleOwner
	}
	return nil
}

func (p *PreparedStoreState) abortOwnership(scope storeMutationScope) {
	if p.validateScope(scope) != nil {
		return
	}
	if p.newResource != nil {
		p.newResource.lease.Close()
		p.newResource = nil
	}
	p.retired = nil
}

func (s *Store) abortPrepared(scope storeMutationScope) error {
	prepared := s.prepared
	if prepared == nil {
		return nil
	}
	if err := prepared.validateScope(scope); err != nil {
		return err
	}
	return prepared.abort(scope)
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
