package accessibility

import "strings"

type Range struct {
	providerID uint64
	generation uint64
	instance   uint64
	node       NodeID
	start      int
	end        int
}

func NewRange(document Document, node NodeID, start, end int) (Range, error) {
	stored, exists := document.nodeValue(node)
	if !exists || !(Span{Start: start, End: end}).Valid(len(stored.graphemes)) {
		return Range{}, ErrInvalidRange
	}
	return Range{
		providerID: document.providerID,
		generation: document.generation,
		instance:   document.instance,
		node:       node,
		start:      start,
		end:        end,
	}, nil
}

func (value Range) ProviderID() uint64 { return value.providerID }
func (value Range) Generation() uint64 { return value.generation }
func (value Range) NodeID() NodeID     { return value.node }
func (value Range) Span() Span         { return Span{Start: value.start, End: value.end} }
func (value Range) Clone() Range       { return value }

func (value Range) Text(document Document) (string, error) {
	node, err := value.resolve(document)
	if err != nil {
		return "", err
	}
	return strings.Join(node.graphemes[value.start:value.end], ""), nil
}

func (value Range) Rectangles(document Document) ([]Rect, error) {
	node, err := value.resolve(document)
	if err != nil {
		return nil, err
	}
	var rectangles []Rect
	for _, row := range node.rows {
		start := max(value.start, row.start)
		end := min(value.end, row.end)
		if start >= end || len(row.bounds) == 0 {
			continue
		}
		boundsStart := start - row.start
		boundsEnd := end - row.start
		rectangles = append(rectangles, row.bounds[boundsStart:boundsEnd]...)
	}
	return append([]Rect(nil), rectangles...), nil
}

func CompareEndpoints(document Document, left Range, leftStart bool, right Range, rightStart bool) (int, error) {
	if _, err := left.resolve(document); err != nil {
		return 0, err
	}
	if _, err := right.resolve(document); err != nil {
		return 0, err
	}
	if left.node != right.node {
		return 0, ErrInvalidRange
	}
	leftValue, rightValue := left.end, right.end
	if leftStart {
		leftValue = left.start
	}
	if rightStart {
		rightValue = right.start
	}
	switch {
	case leftValue < rightValue:
		return -1, nil
	case leftValue > rightValue:
		return 1, nil
	default:
		return 0, nil
	}
}

func (value Range) resolve(document Document) (documentNode, error) {
	if value.providerID == 0 || value.providerID != document.providerID || value.generation != document.generation || value.instance != document.instance {
		return documentNode{}, ErrStaleRange
	}
	node, exists := document.nodeValue(value.node)
	if !exists || !(Span{Start: value.start, End: value.end}).Valid(len(node.graphemes)) {
		return documentNode{}, ErrInvalidRange
	}
	return node, nil
}

func (document Document) nodeValue(id NodeID) (documentNode, bool) {
	index, exists := document.index[id]
	if !exists {
		return documentNode{}, false
	}
	return document.nodes[index], true
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
