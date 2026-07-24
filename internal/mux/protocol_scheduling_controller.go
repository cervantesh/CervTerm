package mux

const protocolSchedulingControllerPortBudget = 5

type protocolSchedulingDispatchPort interface {
	dispatchKitty([]Event) []Event
	dispatchSixel()
	dispatchITerm()
}

type protocolSchedulingApplyPort interface {
	applyExpiry([]Event) []Event
	applyCompletion([]Event) []Event
}

// protocolSchedulingController owns protocol dispatch/apply ordering only. All
// mutable effects and operation values remain behind operation-scoped ports.
// TODO(L3-01; expires Slice 6.2d): remove the preparatory facade adapter.
type protocolSchedulingController[dispatchPort protocolSchedulingDispatchPort, applyPort protocolSchedulingApplyPort] struct{}

func newProtocolSchedulingController[dispatchPort protocolSchedulingDispatchPort, applyPort protocolSchedulingApplyPort]() protocolSchedulingController[dispatchPort, applyPort] {
	return protocolSchedulingController[dispatchPort, applyPort]{}
}

func (protocolSchedulingController[dispatchPort, applyPort]) dispatch(events []Event, port dispatchPort) []Event {
	events = port.dispatchKitty(events)
	port.dispatchSixel()
	port.dispatchITerm()
	return events
}

func (protocolSchedulingController[dispatchPort, applyPort]) applyExpiry(events []Event, port applyPort) []Event {
	return port.applyExpiry(events)
}

func (protocolSchedulingController[dispatchPort, applyPort]) applyCompletion(events []Event, port applyPort) []Event {
	return port.applyCompletion(events)
}
