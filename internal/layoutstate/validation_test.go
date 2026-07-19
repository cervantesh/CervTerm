package layoutstate

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"
)

func paneNode() Node { return Node{Type: "pane", Launch: &Launch{Args: []string{}}} }

func depthTree(depth int) Node {
	if depth <= 1 {
		return paneNode()
	}
	first, second := paneNode(), depthTree(depth-1)
	return Node{Type: "split", Axis: "columns", Ratio: 5000, First: &first, Second: &second}
}

func balancedTree(leaves int) Node {
	if leaves <= 1 {
		return paneNode()
	}
	leftCount := leaves / 2
	left, right := balancedTree(leftCount), balancedTree(leaves-leftCount)
	return Node{Type: "split", Axis: "rows", Ratio: 5000, First: &left, Second: &right}
}

func oneWindow(root Node) Window {
	return Window{Bounds: Bounds{Width: 800, Height: 600}, ActiveTab: 0, Tabs: []Tab{{FocusedLeaf: 0, Root: root}}}
}

func documentWithWorkspaces(count int) Document {
	workspaces := make([]Workspace, count)
	for i := range workspaces {
		workspaces[i] = Workspace{Name: fmt.Sprintf("workspace-%d", i), ActiveWindow: -1, Windows: []Window{}}
	}
	workspaces[0].ActiveWindow = 0
	workspaces[0].Windows = []Window{oneWindow(paneNode())}
	return Document{Version: Version1, ActiveWorkspace: 0, Workspaces: workspaces}
}

func TestNewPlanRejectsCycleBeforeClone(t *testing.T) {
	root := Node{Type: "split", Axis: "columns", Ratio: 5000}
	root.First = &root
	second := paneNode()
	root.Second = &second
	document := documentWithWorkspaces(1)
	document.Workspaces[0].Windows[0].Tabs[0].Root = root
	if _, err := NewPlan(document); err == nil || !strings.Contains(err.Error(), "depth exceeds") {
		t.Fatalf("err=%v", err)
	}
}

func TestTreeDepthBoundary(t *testing.T) {
	document := documentWithWorkspaces(1)
	document.Workspaces[0].Windows[0].Tabs[0].Root = depthTree(MaxTreeDepth)
	document.Workspaces[0].Windows[0].Tabs[0].FocusedLeaf = MaxTreeDepth - 1
	if _, err := NewPlan(document); err != nil {
		t.Fatal(err)
	}
	document.Workspaces[0].Windows[0].Tabs[0].Root = depthTree(MaxTreeDepth + 1)
	if _, err := NewPlan(document); err == nil {
		t.Fatal("accepted over-depth tree")
	}
}

func TestCollectionBoundaries(t *testing.T) {
	if _, err := NewPlan(documentWithWorkspaces(MaxWorkspaces)); err != nil {
		t.Fatalf("max workspaces: %v", err)
	}
	over := documentWithWorkspaces(MaxWorkspaces)
	over.Workspaces = append(over.Workspaces, Workspace{Name: "overflow", ActiveWindow: -1, Windows: []Window{}})
	if _, err := NewPlan(over); err == nil {
		t.Fatal("accepted too many workspaces")
	}

	document := documentWithWorkspaces(1)
	windows := make([]Window, MaxWindows)
	for i := range windows {
		windows[i] = oneWindow(paneNode())
	}
	document.Workspaces[0].Windows = windows
	if _, err := NewPlan(document); err != nil {
		t.Fatalf("max windows: %v", err)
	}
	document.Workspaces[0].Windows = append(document.Workspaces[0].Windows, oneWindow(paneNode()))
	if _, err := NewPlan(document); err == nil {
		t.Fatal("accepted too many windows")
	}

	document = documentWithWorkspaces(1)
	tabs := make([]Tab, MaxTabsWindow)
	for i := range tabs {
		tabs[i] = Tab{FocusedLeaf: 0, Root: paneNode()}
	}
	document.Workspaces[0].Windows[0].Tabs = tabs
	if _, err := NewPlan(document); err != nil {
		t.Fatalf("max tabs: %v", err)
	}
	document.Workspaces[0].Windows[0].Tabs = append(document.Workspaces[0].Windows[0].Tabs, Tab{FocusedLeaf: 0, Root: paneNode()})
	if _, err := NewPlan(document); err == nil {
		t.Fatal("accepted too many tabs")
	}
}

func TestNodeAndPaneAggregateBudgets(t *testing.T) {
	document := documentWithWorkspaces(1)
	document.Workspaces[0].Windows[0].Tabs[0].Root = balancedTree(MaxPanes)
	if _, err := NewPlan(document); err == nil || !strings.Contains(err.Error(), "layout: exceeds") {
		// The structural maximum is deliberately below the storage byte ceiling in usable plans;
		// this maximum-shape document is rejected by the final canonical byte budget.
		t.Fatalf("max shape err=%v", err)
	}
	document.Workspaces[0].Windows[0].Tabs[0].Root = balancedTree(MaxPanes + 1)
	if _, err := NewPlan(document); err == nil || (!strings.Contains(err.Error(), "nodes exceeds") && !strings.Contains(err.Error(), "panes exceeds")) {
		t.Fatalf("over shape err=%v", err)
	}
}

func TestNewPlanRejectsCanonicalOutputOverByteLimit(t *testing.T) {
	document := documentWithWorkspaces(1)
	root := balancedTree(255)
	fillPaneCWD(&root, strings.Repeat("x", 4000))
	document.Workspaces[0].Windows[0].Tabs[0].Root = root
	if _, err := NewPlan(document); err == nil || !strings.Contains(err.Error(), "layout: exceeds") {
		t.Fatalf("err=%v", err)
	}
	if _, err := Unmarshal(bytes.Repeat([]byte{' '}, MaxJSONBytes+1)); err == nil {
		t.Fatal("accepted oversized input")
	}
}

func fillPaneCWD(node *Node, cwd string) {
	if node.Type == "pane" {
		node.Launch.CWD = cwd
		return
	}
	fillPaneCWD(node.First, cwd)
	fillPaneCWD(node.Second, cwd)
}

func TestIndexRatioBoundsAndAppearanceValidation(t *testing.T) {
	mutations := []func(*Document){
		func(d *Document) { d.ActiveWorkspace = 2 },
		func(d *Document) { d.Workspaces[0].ActiveWindow = 2 },
		func(d *Document) { d.Workspaces[0].Windows[0].ActiveTab = 2 },
		func(d *Document) { d.Workspaces[0].Windows[0].Tabs[0].FocusedLeaf = 1 },
		func(d *Document) { d.Workspaces[0].Windows[0].Bounds.Width = 0 },
		func(d *Document) {
			d.Workspaces[0].Windows[0].Tabs[0].Root = Node{Type: "split", Axis: "rows", Ratio: 0, First: ptrNode(paneNode()), Second: ptrNode(paneNode())}
		},
		func(d *Document) {
			value := math.NaN()
			d.Workspaces[0].Windows[0].Appearance.BackgroundOpacity = &value
		},
		func(d *Document) { value := 513.0; d.Workspaces[0].Windows[0].Appearance.FontSize = &value },
	}
	for i, mutate := range mutations {
		document := documentWithWorkspaces(1)
		mutate(&document)
		if _, err := NewPlan(document); err == nil {
			t.Fatalf("mutation %d accepted", i)
		}
	}
}

func ptrNode(node Node) *Node { return &node }

func TestAggregateArgumentBoundary(t *testing.T) {
	document := documentWithWorkspaces(1)
	root := balancedTree(MaxArgsTotal / MaxArgs)
	fillPaneArgs(&root, MaxArgs)
	document.Workspaces[0].Windows[0].Tabs[0].Root = root
	if _, err := NewPlan(document); err != nil {
		t.Fatalf("exact aggregate args: %v", err)
	}
	root = balancedTree(MaxArgsTotal/MaxArgs + 1)
	fillPaneArgs(&root, MaxArgs)
	document.Workspaces[0].Windows[0].Tabs[0].Root = root
	if _, err := NewPlan(document); err == nil || !strings.Contains(err.Error(), "total count exceeds") {
		t.Fatalf("err=%v", err)
	}
}

func fillPaneArgs(node *Node, count int) {
	if node.Type == "pane" {
		node.Launch.Program = "sh"
		node.Launch.Args = make([]string, count)
		return
	}
	fillPaneArgs(node.First, count)
	fillPaneArgs(node.Second, count)
}

func TestLaunchDescriptorModes(t *testing.T) {
	valid := []Launch{
		{},
		{TargetID: "configured"},
		{TargetID: "configured", Program: "sh", Args: []string{"-l"}, CWD: "/tmp"},
		{Program: "sh", Args: []string{"-l"}},
		{CWD: "/tmp"},
	}
	for i, launch := range valid {
		document := documentWithWorkspaces(1)
		document.Workspaces[0].Windows[0].Tabs[0].Root.Launch = &launch
		if _, err := NewPlan(document); err != nil {
			t.Fatalf("mode %d: %v", i, err)
		}
	}
	document := documentWithWorkspaces(1)
	document.Workspaces[0].Windows[0].Tabs[0].Root.Launch = &Launch{TargetID: "configured", Args: []string{"-l"}}
	if _, err := NewPlan(document); err == nil {
		t.Fatal("accepted fallback args without program")
	}
}
