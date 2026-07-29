package termimage

func (o *StoreOwner) PrepareCandidateWithRetention(candidate *DecodedCandidate, retention ResourceRetention) (*PreparedStoreState, ResourceRef, error) {
	scope, err := o.enter()
	if err != nil {
		return nil, ResourceRef{}, err
	}
	defer o.leave(scope)
	return o.store.prepareCandidateWithRetention(scope, candidate, retention)
}

func (s *Store) ResourceRetention(ref ResourceRef) (ResourceRetention, bool) {
	if s == nil || ref.Image == 0 || ref.Generation == 0 {
		return ResourceDurable, false
	}
	state := s.state.Load()
	if state == nil || state.closed {
		return ResourceDurable, false
	}
	stored := state.resources[ref.Image]
	if stored == nil || stored.ref != ref {
		return ResourceDurable, false
	}
	return stored.retention, true
}
