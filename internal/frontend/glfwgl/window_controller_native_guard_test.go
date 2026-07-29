//go:build glfw

package glfwgl

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"cervterm/internal/layoutrestore"
	termmux "cervterm/internal/mux"
	"cervterm/internal/ownerthread"
)

func TestWindowControllerNativePathInventoryRequiresExactLoopGuard(t *testing.T) {
	guarded := []string{
		"abortRestoreProjections", "acquireWindowCapability", "activate", "activateRestoreProjections",
		"activateRuntimeProjection", "applyWorkspaceProjection", "cancelProjectionPaneComposition",
		"cancelProjectionTabComposition", "closeProjection", "closeProjectionLoop", "closeRuntimeProjection",
		"createProjection", "createRuntimeProjection", "currentLayoutPlan", "dispatch", "focus",
		"pollEvents", "prepareRestoreProjections", "publishRestoreProjections", "recordRuntimeFocus",
		"restoreStartupProjections", "restoreStartupProjectionsBeforeMux", "shouldClose",
		"syncPendingRestoreApps", "syncSharedProjectionState", "transferProjectionGeometry", "waitEvents", "withCurrent",
	}
	unguarded := []string{
		"accessibilityWindow", "activateRuntimeProjectionFrom", "activeProjectionApp", "adoptProjectionBundle",
		"attach", "attachApp", "clearDamage", "closeRuntimeProjectionFrom", "createRuntimeProjectionFrom",
		"createWorkspace", "drainMux", "installProcessServices", "markDamage", "markDamageFrom",
		"moveWindowToWorkspace", "processClosed", "processReady", "processWindows", "processWorkspaces",
		"projectionApp", "projectionAvailable", "projectionCount", "projectionIDs", "projectionVisible",
		"queuePending", "renameWorkspace", "requireLoop", "requireOrigin", "restoreGeometries",
		"setCandidateFactory", "setCandidateProjectionFactory", "setProjectionFactory", "setRestoreWindows",
		"setRuntimeWindows", "setSharedServices", "setTeardown", "shutdownServices", "startLoop", "stopLoop",
		"switchWorkspace", "transferPaneBetweenWindows", "transferTabBetweenWindows", "updateProcessConfig",
		"validRestoreProjectionCandidate", "windowForPane",
	}
	sort.Strings(guarded)
	sort.Strings(unguarded)
	found := make(map[string]*ast.FuncDecl)
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, source, 0)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || function.Recv == nil || len(function.Recv.List) != 1 {
				continue
			}
			star, ok := function.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			name, ok := star.X.(*ast.Ident)
			if ok && name.Name == "windowController" {
				found[function.Name.Name] = function
			}
		}
	}
	actual := make([]string, 0, len(found))
	for name := range found {
		actual = append(actual, name)
	}
	sort.Strings(actual)
	want := append(append([]string(nil), guarded...), unguarded...)
	sort.Strings(want)
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("windowController method inventory=%v want exact=%v", actual, want)
	}
	for _, name := range guarded {
		if !checkedRequireLoopFirst(found[name]) {
			t.Errorf("windowController.%s must begin with checked `if err := c.requireLoop(); err != nil { return ...err }`", name)
		}
	}
}

func checkedRequireLoopFirst(function *ast.FuncDecl) bool {
	if function == nil || len(function.Body.List) == 0 {
		return false
	}
	condition, ok := function.Body.List[0].(*ast.IfStmt)
	if !ok || len(condition.Body.List) == 0 {
		return false
	}
	if condition.Init == nil {
		return directRequireLoopFailure(condition)
	}
	assign, ok := condition.Init.(*ast.AssignStmt)
	if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return false
	}
	errName, ok := assign.Lhs[0].(*ast.Ident)
	if !ok {
		return false
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	receiver, receiverOK := selector.X.(*ast.Ident)
	if !ok || !receiverOK || receiver.Name != "c" || selector.Sel.Name != "requireLoop" {
		return false
	}
	binary, ok := condition.Cond.(*ast.BinaryExpr)
	if !ok || binary.Op != token.NEQ {
		return false
	}
	left, leftOK := binary.X.(*ast.Ident)
	right, rightOK := binary.Y.(*ast.Ident)
	if !leftOK || left.Name != errName.Name || !rightOK || right.Name != "nil" {
		return false
	}
	if len(condition.Body.List) != 1 {
		return false
	}
	result, ok := condition.Body.List[0].(*ast.ReturnStmt)
	if !ok {
		return false
	}
	for _, expression := range result.Results {
		if ident, ok := expression.(*ast.Ident); ok && ident.Name == errName.Name {
			return true
		}
	}
	return false
}

func directRequireLoopFailure(condition *ast.IfStmt) bool {
	binary, ok := condition.Cond.(*ast.BinaryExpr)
	if !ok || binary.Op != token.NEQ {
		return false
	}
	call, ok := binary.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	receiver, receiverOK := selector.X.(*ast.Ident)
	if !ok || !receiverOK || receiver.Name != "c" || selector.Sel.Name != "requireLoop" {
		return false
	}
	right, ok := binary.Y.(*ast.Ident)
	if !ok || right.Name != "nil" {
		return false
	}
	if len(condition.Body.List) != 1 {
		return false
	}
	_, ok = condition.Body.List[0].(*ast.ReturnStmt)
	return ok
}

func topLevelControllerCalls(body *ast.BlockStmt) []string {
	var calls []string
	for _, statement := range body.List {
		ast.Inspect(statement, func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch function := call.Fun.(type) {
			case *ast.SelectorExpr:
				calls = append(calls, function.Sel.Name)
			case *ast.Ident:
				calls = append(calls, function.Name)
			}
			return true
		})
	}
	return calls
}

func TestWindowControllerNativeMethodValuesRejectWrongThreadBeforeNativeCall(t *testing.T) {
	tests := map[string]func(*windowController) func() error{
		"activate": func(c *windowController) func() error { method := c.activate; return func() error { return method(1) } },
		"focus":    func(c *windowController) func() error { method := c.focus; return func() error { return method(1) } },
		"poll":     func(c *windowController) func() error { method := c.pollEvents; return method },
		"wait": func(c *windowController) func() error {
			method := c.waitEvents
			return func() error { return method(time.Millisecond) }
		},
		"with-current": func(c *windowController) func() error {
			method := c.withCurrent
			return func() error { return method(1, func() {}) }
		},
		"close": func(c *windowController) func() error {
			method := c.closeProjection
			return func() error { return method(1) }
		},
		"create": func(c *windowController) func() error {
			method := c.createProjection
			return func() error { return method(2) }
		},
		"runtime-create": func(c *windowController) func() error {
			method := c.createRuntimeProjection
			return func() error { _, err := method(); return err }
		},
		"runtime-activate": func(c *windowController) func() error {
			method := c.activateRuntimeProjection
			return func() error { return method(1) }
		},
		"restore": func(c *windowController) func() error {
			method := c.restoreStartupProjections
			return func() error { return method(layoutrestore.Blueprint{}, nil) }
		},
		"workspace": func(c *windowController) func() error {
			method := c.applyWorkspaceProjection
			return func() error { return method([]termmux.Event{{Kind: termmux.WorkspaceActivated}}) }
		},
		"layout": func(c *windowController) func() error {
			method := c.currentLayoutPlan
			return func() error { _, err := method(); return err }
		},
	}
	for name, bind := range tests {
		t.Run(name, func(t *testing.T) {
			var log []string
			current := ownerthread.ID(41)
			controller := newWindowController(processServices{}, fakeNativePump{log: &log})
			controller.threadSource = ownerthread.SourceFunc(func() ownerthread.ID { return current })
			if err := controller.attach(1, &fakeNativeWindow{id: "one", log: &log}, func([]termmux.Event) bool { return true }); err != nil {
				t.Fatal(err)
			}
			if err := controller.startLoop(); err != nil {
				t.Fatal(err)
			}
			invoke := bind(controller) // Capture the method value on the valid loop thread.
			before := append([]string(nil), log...)
			current = 42
			if err := invoke(); !errors.Is(err, errWindowLoopThread) {
				t.Fatalf("wrong-thread method value error=%v", err)
			}
			if !reflect.DeepEqual(log, before) {
				t.Fatalf("wrong-thread method value reached native API: before=%v after=%v", before, log)
			}
		})
	}
}
