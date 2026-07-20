package core

const (
	MaxHyperlinkEntries     = 4096
	MaxHyperlinkURIBytes    = 2048
	MaxHyperlinkParamsBytes = 1024
)

type HyperlinkID uint16

type Hyperlink struct {
	ID  HyperlinkID
	URI string
}

type hyperlinkState struct {
	entries map[HyperlinkID]string
	keys    map[string]HyperlinkID
	order   []HyperlinkID
	next    uint32
	current HyperlinkID
}

func (t *Terminal) OpenHyperlink(uri, explicitID string) bool {
	if uri == "" || len(uri) > MaxHyperlinkURIBytes {
		return false
	}
	state := &t.hyperlinks
	if state.entries == nil {
		state.entries = make(map[HyperlinkID]string)
		state.keys = make(map[string]HyperlinkID)
		state.next = 1
	}
	key := explicitID + "\x00" + uri
	if explicitID != "" {
		if id, ok := state.keys[key]; ok {
			state.current = id
			return true
		}
	}
	if len(state.entries) == MaxHyperlinkEntries && !t.reclaimHyperlink() {
		return false
	}
	id, ok := state.allocateID()
	if !ok {
		return false
	}
	state.entries[id] = uri
	state.order = append(state.order, id)
	if explicitID != "" {
		state.keys[key] = id
	}
	state.current = id
	return true
}

func (s *hyperlinkState) allocateID() (HyperlinkID, bool) {
	const maxID = uint32(^HyperlinkID(0))
	for attempts := uint32(0); attempts < maxID; attempts++ {
		if s.next == 0 || s.next > maxID {
			s.next = 1
		}
		id := HyperlinkID(s.next)
		s.next++
		if _, used := s.entries[id]; !used {
			return id, true
		}
	}
	return 0, false
}

func (t *Terminal) reclaimHyperlink() bool {
	referenced := make(map[HyperlinkID]struct{})
	for _, cell := range t.cells {
		if cell.HyperlinkID != 0 {
			referenced[cell.HyperlinkID] = struct{}{}
		}
	}
	for _, cell := range t.scrollback {
		if cell.HyperlinkID != 0 {
			referenced[cell.HyperlinkID] = struct{}{}
		}
	}
	for index, id := range t.hyperlinks.order {
		if _, live := referenced[id]; live {
			continue
		}
		t.hyperlinks.order = append(t.hyperlinks.order[:index], t.hyperlinks.order[index+1:]...)
		delete(t.hyperlinks.entries, id)
		for key, value := range t.hyperlinks.keys {
			if value == id {
				delete(t.hyperlinks.keys, key)
			}
		}
		return true
	}
	return false
}

func (t *Terminal) CloseHyperlink() { t.hyperlinks.current = 0 }

func (t *Terminal) HyperlinkURI(id HyperlinkID) (string, bool) {
	uri, ok := t.hyperlinks.entries[id]
	return uri, ok
}

func (t *Terminal) ProjectHyperlinks(cells []Cell, dst []Hyperlink) []Hyperlink {
	dst = dst[:0]
	seen := make(map[HyperlinkID]struct{})
	for _, cell := range cells {
		id := cell.HyperlinkID
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		uri, ok := t.HyperlinkURI(id)
		if !ok {
			continue
		}
		seen[id] = struct{}{}
		dst = append(dst, Hyperlink{ID: id, URI: uri})
	}
	return dst
}

func (s *hyperlinkState) reset() { *s = hyperlinkState{} }

func (s hyperlinkState) clone() hyperlinkState {
	out := hyperlinkState{next: s.next, current: s.current, order: append([]HyperlinkID(nil), s.order...)}
	if s.entries != nil {
		out.entries = make(map[HyperlinkID]string, len(s.entries))
		for id, uri := range s.entries {
			out.entries[id] = uri
		}
	}
	if s.keys != nil {
		out.keys = make(map[string]HyperlinkID, len(s.keys))
		for key, id := range s.keys {
			out.keys[key] = id
		}
	}
	return out
}
