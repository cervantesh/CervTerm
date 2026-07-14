package script

// statusTable is the runtime's main-thread-only ordered status-segment store.
// order retains first-registration order while texts provides replacement and
// removal by id without changing that order.
type statusTable struct {
	order []string
	texts map[string]string
	seq   int
}

func (st *statusTable) set(id, text string) {
	if st.texts == nil {
		st.texts = make(map[string]string)
	}
	old, exists := st.texts[id]
	if text == "" {
		if !exists {
			return
		}
		delete(st.texts, id)
		for i, orderedID := range st.order {
			if orderedID == id {
				st.order = append(st.order[:i], st.order[i+1:]...)
				break
			}
		}
		st.seq++
		return
	}
	if exists {
		if old == text {
			return
		}
		st.texts[id] = text
		st.seq++
		return
	}
	st.order = append(st.order, id)
	st.texts[id] = text
	st.seq++
}

func (st *statusTable) segments() []string {
	out := make([]string, 0, len(st.order))
	for _, id := range st.order {
		out = append(out, st.texts[id])
	}
	return out
}

// StatusSegments returns the current segment texts in registration order.
func (r *Runtime) StatusSegments() []string {
	if r.statuses == nil {
		return nil
	}
	return r.statuses.segments()
}

// StatusSeq returns the monotonic status mutation counter.
func (r *Runtime) StatusSeq() int {
	if r.statuses == nil {
		return 0
	}
	return r.statuses.seq
}
