package termimage

func (o *StoreOwner) PrepareCandidateWithRetention(candidate *DecodedCandidate, retention ResourceRetention) (*PreparedStoreState, ResourceRef, error) {
	if !o.valid() {
		return nil, ResourceRef{}, ErrClosed
	}
	return o.store.prepareCandidateWithRetention(candidate, retention)
}

func (s *Store) ResourceRetention(ref ResourceRef) (ResourceRetention, bool) {
	if s == nil || ref.Image == 0 || ref.Generation == 0 || s.closed.Load() {
		return ResourceDurable, false
	}
	stored := s.state.resources[ref.Image]
	if stored == nil || stored.ref != ref {
		return ResourceDurable, false
	}
	return stored.retention, true
}
