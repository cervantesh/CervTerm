//go:build glfw

package glfwgl

import (
	"sort"

	"cervterm/internal/ime"
	"cervterm/internal/unicodecluster"

	"golang.org/x/text/unicode/bidi"
)

const maxPreeditVisualCells = 256

type preeditCluster struct {
	text                 string
	runeStart, runeEnd   int
	cellStart, cellWidth int
}

type preeditCellSpan struct {
	start, end int
}

type preeditPresentation struct {
	active      bool
	target      ime.Target
	revision    uint64
	clusters    []preeditCluster
	cells       int
	caretCell   int
	targetSpans []preeditCellSpan
	truncated   bool
}

func preparePreeditPresentation(snapshot ime.Snapshot, availableCells int) preeditPresentation {
	presentation := preeditPresentation{target: snapshot.Target, revision: snapshot.Revision}
	if !snapshot.Active || snapshot.Text == "" || availableCells <= 0 {
		return presentation
	}
	limit := min(availableCells, maxPreeditVisualCells)
	runeOffset, logicalCells := 0, 0
	for _, cluster := range unicodecluster.Segment(snapshot.Text) {
		width := max(1, cluster.Width)
		if logicalCells+width > limit {
			presentation.truncated = true
			break
		}
		presentation.clusters = append(presentation.clusters, preeditCluster{
			text: cluster.Text, runeStart: runeOffset, runeEnd: runeOffset + len(cluster.Runes), cellWidth: width,
		})
		runeOffset += len(cluster.Runes)
		logicalCells += width
	}
	if runeOffset < len(snapshot.Runes) {
		presentation.truncated = true
	}
	visualCell := 0
	for _, logical := range preeditVisualOrder(presentation.clusters) {
		presentation.clusters[logical].cellStart = visualCell
		visualCell += presentation.clusters[logical].cellWidth
	}
	presentation.active = len(presentation.clusters) > 0
	presentation.cells = visualCell
	presentation.caretCell = presentation.cellForRune(snapshot.CursorRune)
	presentation.targetSpans = presentation.cellsForRuneSpan(snapshot.TargetRuneSpan)
	return presentation
}

func (presentation preeditPresentation) cellForRune(index int) int {
	if len(presentation.clusters) == 0 {
		return 0
	}
	for _, cluster := range presentation.clusters {
		if index <= cluster.runeStart {
			if preeditClusterRTL(cluster) {
				return cluster.cellStart + cluster.cellWidth
			}
			return cluster.cellStart
		}
		if index <= cluster.runeEnd {
			if preeditClusterRTL(cluster) {
				return cluster.cellStart
			}
			return cluster.cellStart + cluster.cellWidth
		}
	}
	return presentation.cells
}

func (presentation preeditPresentation) cellsForRuneSpan(span ime.Span) []preeditCellSpan {
	spans := make([]preeditCellSpan, 0, len(presentation.clusters))
	for _, cluster := range presentation.clusters {
		if cluster.runeEnd <= span.Start || cluster.runeStart >= span.End {
			continue
		}
		spans = append(spans, preeditCellSpan{start: cluster.cellStart, end: cluster.cellStart + cluster.cellWidth})
	}
	if len(spans) < 2 {
		return spans
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	merged := spans[:1]
	for _, span := range spans[1:] {
		last := &merged[len(merged)-1]
		if span.start <= last.end {
			last.end = max(last.end, span.end)
			continue
		}
		merged = append(merged, span)
	}
	return merged
}

func preeditVisualOrder(clusters []preeditCluster) []int {
	identity := make([]int, len(clusters))
	runes := make([]rune, len(clusters))
	hasRTL := false
	for i, cluster := range clusters {
		identity[i] = i
		clusterRunes := []rune(cluster.text)
		if len(clusterRunes) == 0 {
			continue
		}
		runes[i] = clusterRunes[0]
		property, _ := bidi.LookupRune(runes[i])
		hasRTL = hasRTL || property.Class() == bidi.R || property.Class() == bidi.AL
	}
	if !hasRTL {
		return identity
	}
	var paragraph bidi.Paragraph
	if _, err := paragraph.SetString(string(runes), bidi.DefaultDirection(bidi.LeftToRight)); err != nil {
		return identity
	}
	ordering, err := paragraph.Order()
	if err != nil {
		return identity
	}
	visual := make([]int, 0, len(clusters))
	for i := 0; i < ordering.NumRuns(); i++ {
		run := ordering.Run(i)
		start, end := run.Pos()
		if run.Direction() == bidi.RightToLeft {
			for logical := end; logical >= start; logical-- {
				visual = append(visual, logical)
			}
			continue
		}
		for logical := start; logical <= end; logical++ {
			visual = append(visual, logical)
		}
	}
	if len(visual) != len(clusters) {
		return identity
	}
	return visual
}

func preeditClusterRTL(cluster preeditCluster) bool {
	runes := []rune(cluster.text)
	if len(runes) == 0 {
		return false
	}
	property, _ := bidi.LookupRune(runes[0])
	return property.Class() == bidi.R || property.Class() == bidi.AL
}
