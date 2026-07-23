package termimage

// AllocateInternalImageID reserves the next high-half pane-local image identity.
// Identities are monotonic for the Store lifetime and survive reset.
func (s *Store) AllocateInternalImageID() (ImageID, error) {
	if s == nil || s.closed.Load() || s.resetting.Load() {
		return 0, ErrClosed
	}
	s.identityMu.Lock()
	defer s.identityMu.Unlock()
	if s.closed.Load() || s.resetting.Load() {
		return 0, ErrClosed
	}
	if s.nextInternalImage == ImageID(^uint32(0)) {
		return 0, ErrInternalIDExhausted
	}
	s.nextInternalImage++
	return s.nextInternalImage, nil
}

// AllocateInternalPlacementID reserves the next high-half pane-local placement identity.
// Identities are monotonic for the Store lifetime and survive reset.
func (s *Store) AllocateInternalPlacementID() (PlacementID, error) {
	if s == nil || s.closed.Load() || s.resetting.Load() {
		return 0, ErrClosed
	}
	s.identityMu.Lock()
	defer s.identityMu.Unlock()
	if s.closed.Load() || s.resetting.Load() {
		return 0, ErrClosed
	}
	if s.nextInternalPlacement == PlacementID(^uint32(0)) {
		return 0, ErrInternalIDExhausted
	}
	s.nextInternalPlacement++
	return s.nextInternalPlacement, nil
}
