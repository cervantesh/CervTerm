package mux

const sessionIngressControllerPortBudget = 3

type sessionIngressOwnerPort interface {
	acceptSessionIngress() bool
}

type sessionIngressApplyPort interface {
	applySessionIngressData([]Event, []byte) []Event
	applySessionIngressEnd([]Event, error) []Event
}

// sessionIngressController owns accepted-record phase ordering only. Owner
// validation and all mutable effects remain behind operation-scoped ports.
// TODO(L3-01; expires Slice 6.2d): remove the preparatory facade adapter.
type sessionIngressController[ownerPort sessionIngressOwnerPort, applyPort sessionIngressApplyPort] struct{}

func newSessionIngressController[ownerPort sessionIngressOwnerPort, applyPort sessionIngressApplyPort]() sessionIngressController[ownerPort, applyPort] {
	return sessionIngressController[ownerPort, applyPort]{}
}

func (sessionIngressController[ownerPort, applyPort]) route(events []Event, owner ownerPort, apply applyPort, data []byte, end error) []Event {
	if !owner.acceptSessionIngress() {
		return events
	}
	if len(data) > 0 {
		events = apply.applySessionIngressData(events, data)
	}
	if end != nil {
		events = apply.applySessionIngressEnd(events, end)
	}
	return events
}
