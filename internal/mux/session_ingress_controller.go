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
type sessionIngressController struct{}

func newSessionIngressController() sessionIngressController {
	return sessionIngressController{}
}

func (sessionIngressController) route(events []Event, owner sessionIngressOwnerPort, apply sessionIngressApplyPort, data []byte, end error) []Event {
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
