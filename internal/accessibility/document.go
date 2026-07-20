package accessibility

import (
	"fmt"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/clipperhouse/uax29/v2/graphemes"
)

type documentRow struct {
	text        string
	bounds      []Rect
	start, end  int
	softWrapped bool
}

type documentNode struct {
	id        NodeID
	parent    NodeID
	role      Role
	name      string
	text      string
	rows      []documentRow
	graphemes []string
	caret     int
	hasCaret  bool
	selection Span
	hasSelect bool
}

var nextDocumentInstance atomic.Uint64

type Document struct {
	providerID uint64
	generation uint64
	instance   uint64
	nodes      []documentNode
	index      map[NodeID]int
	focus      NodeID
	truncated  bool
	rows       int
	graphemes  int
	utf8Bytes  int
}

func NewDocument(draft DocumentDraft) (Document, error) {
	if draft.ProviderID == 0 || draft.Generation == 0 || draft.Generation == ^uint64(0) || len(draft.Nodes) == 0 {
		return Document{}, ErrInvalidDocument
	}
	document := Document{
		providerID: draft.ProviderID,
		generation: draft.Generation,
		index:      make(map[NodeID]int, min(len(draft.Nodes), MaxNodes)),
		truncated:  draft.Truncated,
	}
	limit := len(draft.Nodes)
	nodesOmitted := limit > MaxNodes
	if nodesOmitted {
		limit = MaxNodes
		document.truncated = true
	}
	rootProjection := draft.Nodes[0].ID.Projection
	for index := 0; index < limit; index++ {
		draftNode := draft.Nodes[index]
		if err := validateNodeDraft(draftNode, index, rootProjection, document.index); err != nil {
			return Document{}, err
		}
		remaining := MaxUTF8Bytes - document.utf8Bytes
		if len(draftNode.Name) > remaining {
			draftNode.Name = validUTF8Prefix(draftNode.Name, remaining)
			document.truncated = true
		}
		document.utf8Bytes += len(draftNode.Name)
		node, stopped, err := document.buildNode(draftNode)
		if err != nil {
			return Document{}, err
		}
		document.index[node.id] = len(document.nodes)
		document.nodes = append(document.nodes, node)
		if stopped {
			document.truncated = true
		}
	}
	if draft.Focus.Valid() {
		if _, exists := document.index[draft.Focus]; exists {
			document.focus = draft.Focus
		} else if !nodesOmitted {
			return Document{}, fmt.Errorf("%w: focus node", ErrInvalidIdentity)
		}
	} else if draft.Focus != (NodeID{}) {
		return Document{}, fmt.Errorf("%w: focus node", ErrInvalidIdentity)
	}
	document.instance = nextDocumentInstance.Add(1)
	if document.instance == 0 {
		return Document{}, ErrCounterExhausted
	}
	return document, nil
}

func validateNodeDraft(node NodeDraft, position int, projection uint64, seen map[NodeID]int) error {
	if !node.ID.Valid() || node.ID.Projection != projection || !node.Role.Valid() || !utf8.ValidString(node.Name) || len(node.Name) > MaxNodeNameBytes {
		return ErrInvalidIdentity
	}
	if node.ID.Kind >= NodeKindPane && node.ID.Activation == 0 {
		return fmt.Errorf("%w: activation", ErrInvalidIdentity)
	}
	if _, duplicate := seen[node.ID]; duplicate {
		return fmt.Errorf("%w: duplicate node", ErrInvalidIdentity)
	}
	if position == 0 {
		if node.Parent != (NodeID{}) {
			return fmt.Errorf("%w: root parent", ErrInvalidIdentity)
		}
	} else {
		if !node.Parent.Valid() {
			return fmt.Errorf("%w: missing parent", ErrInvalidIdentity)
		}
		if _, exists := seen[node.Parent]; !exists {
			return fmt.Errorf("%w: parent order", ErrInvalidIdentity)
		}
	}
	return nil
}

func (document *Document) buildNode(draft NodeDraft) (documentNode, bool, error) {
	node := documentNode{id: draft.ID, parent: draft.Parent, role: draft.Role, name: draft.Name}
	var text strings.Builder
	stopped := false
	for _, rowDraft := range draft.Rows {
		if document.rows >= MaxRows {
			stopped = true
			break
		}
		if !utf8.ValidString(rowDraft.Text) || strings.ContainsAny(rowDraft.Text, "\r\n") {
			return documentNode{}, false, ErrInvalidText
		}
		if len(node.rows) > 0 && !node.rows[len(node.rows)-1].softWrapped {
			if document.graphemes == MaxGraphemes || document.utf8Bytes == MaxUTF8Bytes {
				stopped = true
				break
			}
			node.graphemes = append(node.graphemes, "\n")
			text.WriteByte('\n')
			document.graphemes++
			document.utf8Bytes++
		}
		row := documentRow{start: len(node.graphemes), softWrapped: rowDraft.SoftWrapped}
		iterator := graphemes.FromString(rowDraft.Text)
		clusterIndex := 0
		for iterator.Next() {
			cluster := iterator.Value()
			if document.graphemes == MaxGraphemes || len(cluster) > MaxUTF8Bytes-document.utf8Bytes {
				stopped = true
				break
			}
			if len(rowDraft.Bounds) != 0 {
				if clusterIndex >= len(rowDraft.Bounds) || !rowDraft.Bounds[clusterIndex].Valid() {
					return documentNode{}, false, ErrInvalidBounds
				}
				row.bounds = append(row.bounds, rowDraft.Bounds[clusterIndex])
			}
			node.graphemes = append(node.graphemes, cluster)
			text.WriteString(cluster)
			document.graphemes++
			document.utf8Bytes += len(cluster)
			clusterIndex++
		}
		if !stopped && len(rowDraft.Bounds) != 0 && clusterIndex != len(rowDraft.Bounds) {
			return documentNode{}, false, ErrInvalidBounds
		}
		row.end = len(node.graphemes)
		row.text = strings.Join(node.graphemes[row.start:row.end], "")
		node.rows = append(node.rows, row)
		document.rows++
		if stopped {
			break
		}
	}
	node.text = text.String()
	if draft.Caret != nil {
		if *draft.Caret < 0 {
			return documentNode{}, false, ErrInvalidRange
		}
		node.caret, node.hasCaret = *draft.Caret, true
		if node.caret > len(node.graphemes) {
			node.caret = len(node.graphemes)
			stopped = true
		}
	}
	if draft.Selection != nil {
		selection := *draft.Selection
		if selection.Start < 0 || selection.End < selection.Start {
			return documentNode{}, false, ErrInvalidRange
		}
		if selection.Start > len(node.graphemes) {
			selection.Start = len(node.graphemes)
			stopped = true
		}
		if selection.End > len(node.graphemes) {
			selection.End = len(node.graphemes)
			stopped = true
		}
		node.selection, node.hasSelect = selection, true
	}
	return node, stopped, nil
}

func validUTF8Prefix(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}

func (document Document) ProviderID() uint64 { return document.providerID }
func (document Document) Generation() uint64 { return document.generation }
func (document Document) Focus() NodeID      { return document.focus }
func (document Document) Truncated() bool    { return document.truncated }
func (document Document) RowCount() int      { return document.rows }
func (document Document) GraphemeCount() int { return document.graphemes }
func (document Document) UTF8Bytes() int     { return document.utf8Bytes }
func (document Document) NodeCount() int     { return len(document.nodes) }

func (document Document) Node(id NodeID) (NodeSnapshot, bool) {
	index, exists := document.index[id]
	if !exists {
		return NodeSnapshot{}, false
	}
	return snapshotNode(document.nodes[index]), true
}

func (document Document) Nodes() []NodeSnapshot {
	nodes := make([]NodeSnapshot, len(document.nodes))
	for index, node := range document.nodes {
		nodes[index] = snapshotNode(node)
	}
	return nodes
}

func snapshotNode(node documentNode) NodeSnapshot {
	snapshot := NodeSnapshot{
		ID: node.id, Parent: node.parent, Role: node.role, Name: node.name, Text: node.text,
		Caret: node.caret, HasCaret: node.hasCaret, Selection: node.selection, HasSelect: node.hasSelect,
		Rows: make([]RowSnapshot, len(node.rows)),
	}
	for index, row := range node.rows {
		snapshot.Rows[index] = RowSnapshot{
			Text: row.text, Bounds: append([]Rect(nil), row.bounds...), StartGrapheme: row.start, EndGrapheme: row.end, SoftWrapped: row.softWrapped,
		}
	}
	return snapshot
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
