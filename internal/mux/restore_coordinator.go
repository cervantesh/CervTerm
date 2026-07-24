package mux

const restoreCoordinatorPortBudget = 5

type restorePreparationPort interface {
	freshSessionSnapshot() (FreshSessionSnapshot, error)
	prepareRestore() (*RestoreCandidate, error)
}

type restorePublicationPort interface {
	restoreWindowIDs(*RestoreCandidate) ([]WindowID, error)
	commitRestore(*RestoreCandidate) ([]Event, error)
	abortRestore(*RestoreCandidate) error
}

// restoreCoordinator owns restore preparation/publication delegation only. The
// operation-scoped preparation port retains the exact PrepareRestore arguments;
// all mutable restore state remains behind the ephemeral ports.
// TODO(L3-01; expires Slice 6.2d): remove the preparatory facade adapter.
type restoreCoordinator[
	preparationPort restorePreparationPort,
	publicationPort restorePublicationPort,
] struct{}

func newRestoreCoordinator[
	preparationPort restorePreparationPort,
	publicationPort restorePublicationPort,
]() restoreCoordinator[preparationPort, publicationPort] {
	return restoreCoordinator[preparationPort, publicationPort]{}
}

func (restoreCoordinator[preparationPort, publicationPort]) freshSessionSnapshot(port preparationPort) (FreshSessionSnapshot, error) {
	return port.freshSessionSnapshot()
}

func (restoreCoordinator[preparationPort, publicationPort]) prepareRestore(port preparationPort) (*RestoreCandidate, error) {
	return port.prepareRestore()
}

func (restoreCoordinator[preparationPort, publicationPort]) restoreWindowIDs(candidate *RestoreCandidate, port publicationPort) ([]WindowID, error) {
	return port.restoreWindowIDs(candidate)
}

func (restoreCoordinator[preparationPort, publicationPort]) commitRestore(candidate *RestoreCandidate, port publicationPort) ([]Event, error) {
	return port.commitRestore(candidate)
}

func (restoreCoordinator[preparationPort, publicationPort]) abortRestore(candidate *RestoreCandidate, port publicationPort) error {
	return port.abortRestore(candidate)
}
