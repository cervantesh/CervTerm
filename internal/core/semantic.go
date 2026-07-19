package core

const MaxSemanticZones = 4096

type SemanticKind uint8

const (
	SemanticNone SemanticKind = iota
	SemanticPrompt
	SemanticInput
	SemanticOutput
)

type SemanticZone struct {
	Kind       SemanticKind
	Start, End int // visible-cell offsets; End is exclusive
}

func (t *Terminal) SetSemanticKind(kind SemanticKind) bool {
	if kind > SemanticOutput {
		return false
	}
	t.semanticKind = kind
	return true
}

func (t *Terminal) SemanticKind() SemanticKind { return t.semanticKind }

func ProjectSemanticZones(cells []Cell, dst []SemanticZone) ([]SemanticZone, bool) {
	dst = dst[:0]
	for index := 0; index < len(cells); {
		kind := cells[index].SemanticKind
		if kind == SemanticNone {
			index++
			continue
		}
		end := index + 1
		for end < len(cells) && cells[end].SemanticKind == kind {
			end++
		}
		if len(dst) == MaxSemanticZones {
			return dst, true
		}
		dst = append(dst, SemanticZone{Kind: kind, Start: index, End: end})
		index = end
	}
	return dst, false
}
