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

// protocolSchedulingController owns protocol dispatch/apply delegation only. All
// mutable effects and operation values remain behind operation-scoped ports.
// Its private value pairing keeps operation adapters concrete across the generic
// boundary; allocation tests protect this shape.
// TODO(L3-01; expires Slice 6.2d): remove the preparatory facade adapter.
type protocolSchedulingController[
	dispatchPort protocolSchedulingDispatchPort,
	applyPort protocolSchedulingApplyPort,
] struct{}

func newProtocolSchedulingController[
	dispatchPort protocolSchedulingDispatchPort,
	applyPort protocolSchedulingApplyPort,
]() protocolSchedulingController[dispatchPort, applyPort] {
	return protocolSchedulingController[dispatchPort, applyPort]{}
}

func (protocolSchedulingController[dispatchPort, applyPort]) dispatchKitty(events []Event, port dispatchPort) []Event {
	return port.dispatchKitty(events)
}

func (protocolSchedulingController[dispatchPort, applyPort]) dispatchSixel(port dispatchPort) {
	port.dispatchSixel()
}

func (protocolSchedulingController[dispatchPort, applyPort]) dispatchITerm(port dispatchPort) {
	port.dispatchITerm()
}

func (protocolSchedulingController[dispatchPort, applyPort]) applyExpiry(events []Event, port applyPort) []Event {
	return port.applyExpiry(events)
}

func (protocolSchedulingController[dispatchPort, applyPort]) applyCompletion(events []Event, port applyPort) []Event {
	return port.applyCompletion(events)
}
