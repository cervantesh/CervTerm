package ime

const maxCounter = ^uint64(0)

type Controller struct {
	active         bool
	target         Target
	generation     uint64
	revision       uint64
	text           string
	runes          []rune
	cursorRune     int
	targetRuneSpan Span
	lastCancel     CancelReason
}

func (controller *Controller) Start(target Target) (uint64, error) {
	if !target.Valid() {
		return 0, ErrInvalidTarget
	}
	if controller.active {
		return 0, ErrAlreadyActive
	}
	if controller.generation == maxCounter || controller.revision == maxCounter {
		return 0, ErrCounterExhausted
	}
	controller.generation++
	controller.revision++
	controller.active = true
	controller.target = target
	controller.text = ""
	controller.runes = nil
	controller.cursorRune = 0
	controller.targetRuneSpan = Span{}
	controller.lastCancel = CancelNone
	return controller.generation, nil
}

func (controller *Controller) Update(generation uint64, update NativeUpdate) error {
	if err := controller.requireActive(generation); err != nil {
		return err
	}
	normalized, err := normalizeUpdate(update)
	if err != nil {
		return err
	}
	if controller.revision == maxCounter {
		return ErrCounterExhausted
	}
	controller.revision++
	controller.text = normalized.text
	controller.runes = append(controller.runes[:0], normalized.runes...)
	controller.cursorRune = normalized.cursorRune
	controller.targetRuneSpan = normalized.target
	return nil
}

func (controller *Controller) Commit(generation uint64, units []uint16) (Commit, error) {
	if err := controller.requireActive(generation); err != nil {
		return Commit{}, err
	}
	text, runes, err := normalizeCommit(units)
	if err != nil {
		return Commit{}, err
	}
	if controller.revision == maxCounter {
		return Commit{}, ErrCounterExhausted
	}
	commit := Commit{
		Target:     controller.target,
		Generation: controller.generation,
		Text:       text,
		Runes:      append([]rune(nil), runes...),
	}
	controller.finish(CancelNone)
	return commit, nil
}

func (controller *Controller) Cancel(generation uint64, reason CancelReason) error {
	if err := controller.requireActive(generation); err != nil {
		return err
	}
	if !reason.Valid() {
		return ErrInvalidCancelReason
	}
	if controller.revision == maxCounter {
		return ErrCounterExhausted
	}
	controller.finish(reason)
	return nil
}

func (controller *Controller) Snapshot() Snapshot {
	return Snapshot{
		Active:         controller.active,
		Target:         controller.target,
		Generation:     controller.generation,
		Revision:       controller.revision,
		Text:           controller.text,
		Runes:          append([]rune(nil), controller.runes...),
		CursorRune:     controller.cursorRune,
		TargetRuneSpan: controller.targetRuneSpan,
		LastCancel:     controller.lastCancel,
	}
}

func (controller *Controller) requireActive(generation uint64) error {
	if !controller.active {
		return ErrInactive
	}
	if generation == 0 || generation != controller.generation {
		return ErrInvalidGeneration
	}
	return nil
}

func (controller *Controller) finish(reason CancelReason) {
	controller.revision++
	controller.active = false
	controller.target = Target{}
	controller.text = ""
	controller.runes = nil
	controller.cursorRune = 0
	controller.targetRuneSpan = Span{}
	controller.lastCancel = reason
}
