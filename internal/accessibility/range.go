package accessibility

import (
	"strings"

	"github.com/clipperhouse/uax29/v2/graphemes"
)

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

type TextUnit uint8

const (
	TextUnitCharacter TextUnit = iota
	TextUnitFormat
	TextUnitWord
	TextUnitLine
	TextUnitParagraph
	TextUnitPage
	TextUnitDocument
)

func (value Range) Equal(document Document, other Range) (bool, error) {
	if _, err := value.resolve(document); err != nil {
		return false, err
	}
	if _, err := other.resolve(document); err != nil {
		return false, err
	}
	return value.node == other.node && value.start == other.start && value.end == other.end, nil
}

func (value Range) Expand(document Document, unit TextUnit) (Range, error) {
	node, err := value.resolve(document)
	if err != nil {
		return Range{}, err
	}
	segments, err := textUnitSegments(node, unit)
	if err != nil {
		return Range{}, err
	}
	segment := segmentAt(segments, value.start, len(node.graphemes))
	value.start, value.end = segment.Start, segment.End
	return value, nil
}

func (value Range) Move(document Document, unit TextUnit, count int) (Range, int, error) {
	node, err := value.resolve(document)
	if err != nil {
		return Range{}, 0, err
	}
	segments, err := textUnitSegments(node, unit)
	if err != nil {
		return Range{}, 0, err
	}
	if count == 0 {
		return value, 0, nil
	}
	index := segmentIndex(segments, value.start, len(node.graphemes))
	target := clamp(index+count, 0, len(segments)-1)
	value.start, value.end = segments[target].Start, segments[target].End
	return value, target - index, nil
}

func (value Range) MoveEndpoint(document Document, start bool, unit TextUnit, count int) (Range, int, error) {
	node, err := value.resolve(document)
	if err != nil {
		return Range{}, 0, err
	}
	boundaries, err := textUnitBoundaries(node, unit)
	if err != nil {
		return Range{}, 0, err
	}
	if count == 0 {
		return value, 0, nil
	}
	position := value.end
	if start {
		position = value.start
	}
	position, moved := moveBoundary(boundaries, position, count)
	if start {
		value.start = position
		if value.start > value.end {
			value.end = value.start
		}
	} else {
		value.end = position
		if value.end < value.start {
			value.start = value.end
		}
	}
	return value, moved, nil
}

func (value Range) MoveEndpointTo(document Document, start bool, target Range, targetStart bool) (Range, error) {
	if _, err := value.resolve(document); err != nil {
		return Range{}, err
	}
	if _, err := target.resolve(document); err != nil {
		return Range{}, err
	}
	if value.node != target.node {
		return Range{}, ErrInvalidRange
	}
	position := target.end
	if targetStart {
		position = target.start
	}
	if start {
		value.start = position
		if value.start > value.end {
			value.end = value.start
		}
	} else {
		value.end = position
		if value.end < value.start {
			value.start = value.end
		}
	}
	return value, nil
}

func (value Range) FindText(document Document, needle string, backward, ignoreCase bool) (Range, bool, error) {
	node, err := value.resolve(document)
	if err != nil {
		return Range{}, false, err
	}
	iterator := graphemes.FromString(needle)
	var wanted []string
	for iterator.Next() {
		wanted = append(wanted, iterator.Value())
	}
	if len(wanted) == 0 || len(wanted) > value.end-value.start {
		return Range{}, false, nil
	}
	matches := func(at int) bool {
		for offset := range wanted {
			left, right := node.graphemes[at+offset], wanted[offset]
			if ignoreCase {
				if !strings.EqualFold(left, right) {
					return false
				}
			} else if left != right {
				return false
			}
		}
		return true
	}
	if backward {
		for at := value.end - len(wanted); at >= value.start; at-- {
			if matches(at) {
				result, _ := NewRange(document, value.node, at, at+len(wanted))
				return result, true, nil
			}
		}
	} else {
		for at := value.start; at+len(wanted) <= value.end; at++ {
			if matches(at) {
				result, _ := NewRange(document, value.node, at, at+len(wanted))
				return result, true, nil
			}
		}
	}
	return Range{}, false, nil
}

func textUnitSegments(node documentNode, unit TextUnit) ([]Span, error) {
	length := len(node.graphemes)
	switch unit {
	case TextUnitCharacter:
		if length == 0 {
			return []Span{{}}, nil
		}
		segments := make([]Span, length)
		for index := range segments {
			segments[index] = Span{Start: index, End: index + 1}
		}
		return segments, nil
	case TextUnitFormat, TextUnitWord, TextUnitLine:
		if len(node.rows) == 0 {
			return []Span{{Start: 0, End: length}}, nil
		}
		segments := make([]Span, len(node.rows))
		for index, row := range node.rows {
			end := row.end
			if index+1 < len(node.rows) && node.rows[index+1].start > end {
				end = node.rows[index+1].start
			}
			segments[index] = Span{Start: row.start, End: end}
		}
		return segments, nil
	case TextUnitParagraph, TextUnitPage, TextUnitDocument:
		return []Span{{Start: 0, End: length}}, nil
	default:
		return nil, ErrInvalidRange
	}
}

func textUnitBoundaries(node documentNode, unit TextUnit) ([]int, error) {
	segments, err := textUnitSegments(node, unit)
	if err != nil {
		return nil, err
	}
	boundaries := make([]int, 0, len(segments)+1)
	for _, segment := range segments {
		if len(boundaries) == 0 || boundaries[len(boundaries)-1] != segment.Start {
			boundaries = append(boundaries, segment.Start)
		}
	}
	end := len(node.graphemes)
	if len(boundaries) == 0 || boundaries[len(boundaries)-1] != end {
		boundaries = append(boundaries, end)
	}
	return boundaries, nil
}

func segmentAt(segments []Span, position, length int) Span {
	return segments[segmentIndex(segments, position, length)]
}

func segmentIndex(segments []Span, position, length int) int {
	if position >= length {
		return len(segments) - 1
	}
	for index, segment := range segments {
		if position >= segment.Start && position < segment.End {
			return index
		}
	}
	return 0
}

func moveBoundary(boundaries []int, position, count int) (int, int) {
	if count > 0 {
		first := 0
		for first < len(boundaries) && boundaries[first] <= position {
			first++
		}
		if first == len(boundaries) {
			return position, 0
		}
		target := clamp(first+count-1, first, len(boundaries)-1)
		return boundaries[target], target - first + 1
	}
	first := 0
	for first < len(boundaries) && boundaries[first] < position {
		first++
	}
	target := clamp(first+count, 0, first)
	return boundaries[target], target - first
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
