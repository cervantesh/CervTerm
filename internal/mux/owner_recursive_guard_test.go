package mux

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type l302SourceFunction struct {
	pkg, path, receiver, name string
	decl                      *ast.FuncDecl
	calls                     []string
	directWrite               bool
}

func l302RecursiveProduction(t *testing.T) ([]l302SourceFunction, map[string]*ast.File) {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	roots := []string{"internal/mux", "internal/core", "internal/termimage", "internal/frontend/glfwgl", "internal/ownerthread"}
	files := make(map[string]*ast.File)
	var functions []l302SourceFunction
	for _, relative := range roots {
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(relative)), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			rel = filepath.ToSlash(rel)
			fset := token.NewFileSet()
			file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if parseErr != nil {
				return fmt.Errorf("parse %s: %w", rel, parseErr)
			}
			files[rel] = file
			for _, declaration := range file.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Body == nil {
					continue
				}
				functions = append(functions, l302SourceFunction{
					pkg: file.Name.Name, path: rel, receiver: l302ReceiverName(function), name: function.Name.Name,
					decl: function, calls: l302QualifiedCalls(function.Body), directWrite: l302FunctionWritesState(function),
				})
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	typed := l302TypedProductionAnalysis(t)
	for index := range functions {
		if typed.writes[l302FunctionID(functions[index])] || l302ASTCapabilityLocator(functions[index], l302FunctionID(functions[index])) {
			functions[index].directWrite = true
		}
	}
	return functions, files
}

func l302QualifiedCalls(body *ast.BlockStmt) []string {
	aliases := make(map[string]string)
	for changed := true; changed; {
		changed = false
		ast.Inspect(body, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for index, right := range assignment.Rhs {
				if index >= len(assignment.Lhs) {
					continue
				}
				left, ok := assignment.Lhs[index].(*ast.Ident)
				if !ok {
					continue
				}
				target := l302CalledName(right)
				if alias := aliases[target]; alias != "" {
					target = alias
				}
				if target != "" && target != left.Name && aliases[left.Name] == "" {
					aliases[left.Name] = target
					changed = true
				}
			}
			return true
		})
	}
	var result []string
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := l302CalledName(call.Fun)
		if alias := aliases[name]; alias != "" {
			name = alias
		}
		result = append(result, name)
		return true
	})
	sort.Strings(result)
	return result
}

func l302CalledName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	case *ast.IndexExpr:
		return l302CalledName(value.X)
	case *ast.IndexListExpr:
		return l302CalledName(value.X)
	case *ast.ParenExpr:
		return l302CalledName(value.X)
	}
	return ""
}

func l302RootIdent(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return l302RootIdent(value.X)
	case *ast.IndexExpr:
		return l302RootIdent(value.X)
	case *ast.IndexListExpr:
		return l302RootIdent(value.X)
	case *ast.StarExpr:
		return l302RootIdent(value.X)
	case *ast.ParenExpr:
		return l302RootIdent(value.X)
	case *ast.SliceExpr:
		return l302RootIdent(value.X)
	}
	return ""
}

func l302AtomicMutationMethod(name string) bool {
	switch name {
	case "Add", "And", "Store", "Swap", "CAS", "CompareAndSwap", "Or":
		return true
	default:
		return false
	}
}

func l302FunctionWritesState(function *ast.FuncDecl) bool {
	stateRoots := make(map[string]bool)
	if function.Recv != nil {
		for _, field := range function.Recv.List {
			for _, name := range field.Names {
				stateRoots[name.Name] = true
			}
		}
	}
	for _, fields := range []*ast.FieldList{function.Type.Params} {
		if fields == nil {
			continue
		}
		for _, field := range fields.List {
			if !l302PotentialMutableType(field.Type) {
				continue
			}
			for _, name := range field.Names {
				stateRoots[name.Name] = true
			}
		}
	}
	writes := false
	changed := true
	for changed {
		changed = false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for index, right := range assignment.Rhs {
				if !l302ExpressionContainsRoot(right, stateRoots) {
					continue
				}
				if index < len(assignment.Lhs) {
					if name, named := assignment.Lhs[index].(*ast.Ident); named && !stateRoots[name.Name] {
						stateRoots[name.Name] = true
						changed = true
					}
				}
			}
			return true
		})
	}
	mutationMethodValues := make(map[string]bool)
	for changed := true; changed; {
		changed = false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for index, right := range assignment.Rhs {
				if index >= len(assignment.Lhs) {
					continue
				}
				selector, selected := right.(*ast.SelectorExpr)
				atomicValue := selected && l302AtomicMutationMethod(selector.Sel.Name) && stateRoots[l302RootIdent(selector.X)]
				if !atomicValue && !mutationMethodValues[l302RootIdent(right)] {
					continue
				}
				if name, ok := assignment.Lhs[index].(*ast.Ident); ok && !mutationMethodValues[name.Name] {
					mutationMethodValues[name.Name] = true
					changed = true
				}
			}
			return true
		})
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if writes {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				if stateRoots[l302RootIdent(left)] && !isBareIdentifier(left) {
					writes = true
				}
			}
		case *ast.IncDecStmt:
			writes = stateRoots[l302RootIdent(value.X)]
		case *ast.SendStmt:
			writes = stateRoots[l302RootIdent(value.Chan)]
		case *ast.CallExpr:
			name := l302CalledName(value.Fun)
			if l302AtomicMutationMethod(name) && stateRoots[l302RootIdent(value.Fun)] {
				writes = true
			}
			if mutationMethodValues[l302RootIdent(value.Fun)] {
				writes = true
			}
			if (name == "delete" || name == "clear" || name == "copy") && len(value.Args) > 0 && stateRoots[l302RootIdent(value.Args[0])] {
				writes = true
			}
			if name == "append" && len(value.Args) > 0 && stateRoots[l302RootIdent(value.Args[0])] {
				writes = true
			}
		}
		return !writes
	})
	return writes
}

func isBareIdentifier(expression ast.Expr) bool { _, ok := expression.(*ast.Ident); return ok }

func l302ExpressionContainsRoot(expression ast.Expr, roots map[string]bool) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		name, ok := node.(*ast.Ident)
		if ok && roots[name.Name] {
			found = true
			return false
		}
		return !found
	})
	return found
}

func l302PotentialMutableType(expression ast.Expr) bool {
	switch value := expression.(type) {
	case *ast.StarExpr, *ast.MapType, *ast.ChanType, *ast.InterfaceType:
		return true
	case *ast.ArrayType:
		return value.Len == nil
	case *ast.Ellipsis:
		return true
	case *ast.IndexExpr:
		return l302PotentialMutableType(value.X)
	case *ast.IndexListExpr:
		return l302PotentialMutableType(value.X)
	}
	return false
}

func l302MutationClosure(functions []l302SourceFunction) map[string]bool {
	mutating := make(map[string]bool)
	byName := make(map[string][]int)
	for index, function := range functions {
		byName[function.name] = append(byName[function.name], index)
		if function.directWrite {
			mutating[l302FunctionID(function)] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, function := range functions {
			id := l302FunctionID(function)
			if mutating[id] {
				continue
			}
			for _, call := range function.calls {
				for _, target := range byName[call] {
					callee := functions[target]
					if callee.pkg == function.pkg && mutating[l302FunctionID(callee)] {
						mutating[id] = true
						changed = true
						break
					}
				}
				if mutating[id] {
					break
				}
			}
		}
	}
	return mutating
}

func l302FunctionID(function l302SourceFunction) string {
	if function.receiver == "" {
		return function.path + ":" + function.name
	}
	return function.path + ":" + function.receiver + "." + function.name
}

func TestL302RecursiveActualSourceMutationInventory(t *testing.T) {
	functions, _ := l302RecursiveProduction(t)
	mutating := l302MutationClosure(functions)
	var inventory []string
	families := map[string]int{"mux": 0, "core": 0, "termimage": 0, "frontend": 0}
	for id := range mutating {
		inventory = append(inventory, id)
		switch {
		case strings.HasPrefix(id, "internal/mux/"):
			families["mux"]++
		case strings.HasPrefix(id, "internal/core/"):
			families["core"]++
		case strings.HasPrefix(id, "internal/termimage/"):
			families["termimage"]++
		case strings.HasPrefix(id, "internal/frontend/glfwgl/"):
			families["frontend"]++
		}
	}
	sort.Strings(inventory)
	if families["mux"] == 0 || families["core"] == 0 || families["termimage"] == 0 || families["frontend"] == 0 {
		t.Fatalf("recursive mutation inventory omitted production family: %v", families)
	}
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(inventory, "\n"))))
	want := "5e3b1f6c13d34fed50c8d39e7dbea6b24a57028e3d12c98465d7fd8244b90747"
	if runtime.GOOS != "windows" {
		want = "5466e28ee90a0683ce077491924627a95ae5cdbd490401e197f3e0da405ba8e2"
	}
	if got != want {
		t.Fatalf("recursive production mutation inventory hash=%s want=%s families=%v entries=%d first=%v last=%v", got, want, families, len(inventory), inventory[:min(10, len(inventory))], inventory[max(0, len(inventory)-10):])
	}
}

func TestL302OwnerAndWindowFacadeControlFlowOrder(t *testing.T) {
	functions, _ := l302RecursiveProduction(t)
	ownerMutations := l302MutationMethods(t)
	ownerMethods := make(map[string]l302SourceFunction)
	windowMethods := make(map[string]l302SourceFunction)
	for _, function := range functions {
		if function.pkg != "mux" {
			continue
		}
		switch function.receiver {
		case "Owner":
			ownerMethods[function.name] = function
		case "WindowOwner":
			windowMethods[function.name] = function
		}
	}
	for _, function := range functions {
		if function.pkg != "mux" || function.receiver != "Mux" || !function.decl.Name.IsExported() || !ownerMutations[function.name] {
			continue
		}
		owner := ownerMethods[function.name]
		if owner.decl == nil {
			t.Errorf("mutating Mux.%s has no Owner entry point", function.name)
			continue
		}
		if function.name == "Split" || function.name == "Close" || function.name == "Shutdown" {
			continue
		}
		calls := l302TopLevelCallOrder(owner.decl.Body)
		if !l302Ordered(calls, "begin", "leave", function.name) {
			t.Errorf("Owner.%s control flow=%v want begin/error-return -> deferred leave -> exact Mux call", function.name, calls)
		}
	}
	windowSinks := map[string]string{
		"Bootstrap": "bootstrap", "SpawnSplit": "spawnSplit", "FocusPane": "focusPane",
		"FocusDirection": "focusDirection", "FocusNext": "focusNext", "Write": "write",
		"FeedFallback": "feedFallbackOwned", "ClosePane": "closePane", "SpawnTab": "spawnTab",
		"ActivateTab": "activateTab", "RenameTab": "renameTab", "MoveTab": "moveTab",
		"CloseTab": "closeTab", "TransferPane": "transferPane", "ResizeCurrentPane": "resizeCurrentPane",
		"SwapCurrentPane": "swapCurrentPane", "MoveCurrentPane": "moveCurrentPane", "SetSplitRatio": "setSplitRatio",
		"Resize": "resizeOwned", "ResizeGrid": "resizeGrid", "ResizeBounds": "resizeBounds",
		"ResizePaneGrid": "resizePaneGrid", "ApplyResize": "applyResize", "ScrollViewport": "scrollViewport",
		"ScrollViewportToGlobalRow": "scrollViewportToGlobalRow", "SetTitle": "setTitle", "SearchUpward": "searchUpward",
	}
	ownerType := reflect.TypeOf((*Owner)(nil))
	for name, sink := range windowSinks {
		if _, exists := ownerType.MethodByName(name); exists {
			t.Errorf("process Owner still exposes window mutation %s", name)
		}
		function := windowMethods[name]
		if function.decl == nil {
			t.Errorf("WindowOwner.%s missing", name)
			continue
		}
		calls := l302TopLevelCallOrder(function.decl.Body)
		if !l302Ordered(calls, "beginWindowDispatch", "leave", sink) {
			t.Errorf("WindowOwner.%s control flow=%v want origin-bearing scope -> deferred leave -> private sink %s", name, calls, sink)
		}
	}
	if split := windowMethods["Split"]; split.decl == nil || !l302ContainsCall(l302CallNames(split.decl.Body), "SpawnSplit") {
		t.Error("WindowOwner.Split must remain an alias of scoped WindowOwner.SpawnSplit")
	}
}

func l302TopLevelCallOrder(body *ast.BlockStmt) []string {
	var result []string
	for _, statement := range body.List {
		ast.Inspect(statement, func(node ast.Node) bool {
			if inner, nested := node.(*ast.FuncLit); nested {
				_ = inner
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if ok {
				name := l302CalledName(call.Fun)
				if name != "" {
					result = append(result, name)
				}
			}
			return true
		})
	}
	return result
}

func l302Ordered(values []string, required ...string) bool {
	position := -1
	for _, want := range required {
		found := -1
		for index := position + 1; index < len(values); index++ {
			if values[index] == want {
				found = index
				break
			}
		}
		if found < 0 {
			return false
		}
		position = found
	}
	return true
}

func TestL302NoConcreteMuxServiceLocatorAndNarrowCapabilityInventories(t *testing.T) {
	_, files := l302RecursiveProduction(t)
	for path, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			field, ok := node.(*ast.Field)
			if !ok || !l302IsMuxPointer(field.Type) {
				return true
			}
			if strings.HasPrefix(path, "internal/frontend/") {
				t.Errorf("%s exposes concrete *Mux in frontend field/function/interface", path)
			}
			for _, name := range field.Names {
				if name.IsExported() {
					t.Errorf("%s exports concrete *Mux as %s", path, name.Name)
				}
			}
			return true
		})
	}
	frontend := files["internal/frontend/glfwgl/mux_capabilities.go"]
	if frontend == nil {
		t.Fatal("frontend narrow capability declarations missing")
	}
	want := map[string][]string{
		"windowMuxCapability":      {"AcquireImageResource", "ActivateTab", "ActiveTab", "ApplyResize", "Bootstrap", "ClosePane", "CloseTab", "FeedFallback", "FocusDirection", "FocusNext", "FocusPane", "FocusedPane", "GlobalRowToViewport", "ImageSetupError", "Layout", "Line", "LineWrapped", "MoveCurrentPane", "MoveTab", "NextImageDeadline", "PaneIDs", "PaneView", "QuickSelectSnapshot", "QuickSelectSnapshotCurrent", "RenameTab", "ReplyCounters", "Resize", "ResizeBounds", "ResizeCurrentPane", "ResizeGrid", "ResizePaneGrid", "ScrollViewport", "ScrollViewportToGlobalRow", "SearchUpward", "SemanticRangeText", "SemanticSnapshot", "SemanticSnapshotCurrent", "SetSplitRatio", "SetTitle", "SpawnSplit", "SpawnTab", "Split", "SwapCurrentPane", "TabForPane", "Tabs", "TransferPane", "ValidateSemanticRange", "Write"},
		"processCommandCapability": {"ActiveWorkspace", "Closed", "CreateWorkspace", "Drain", "FreshSessionSnapshot", "ImageSetupError", "MoveWindowToWorkspace", "RenameWorkspace", "ResolveEventAddresses", "SetHideCursorWhenScrolled", "SetPaletteBase", "SetScrollbackCapacity", "Shutdown", "SwitchWorkspace", "TransferPaneBetweenWindows", "TransferTabBetweenWindows", "WindowForPane", "Windows", "Workspaces"},
		"windowCapabilityFactory":  {"ForWindow"},
	}
	for name, expected := range want {
		actual := l302InterfaceMethods(frontend, name)
		sort.Strings(expected)
		if strings.Join(actual, ",") != strings.Join(expected, ",") {
			t.Errorf("%s method inventory=%v want=%v", name, actual, expected)
		}
	}
}

func l302IsMuxPointer(expression ast.Expr) bool {
	star, ok := expression.(*ast.StarExpr)
	if !ok {
		return false
	}
	switch value := star.X.(type) {
	case *ast.Ident:
		return value.Name == "Mux"
	case *ast.SelectorExpr:
		return value.Sel.Name == "Mux"
	}
	return false
}

func l302InterfaceMethods(file *ast.File, name string) []string {
	var result []string
	for _, declaration := range file.Decls {
		generic, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range generic.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != name {
				continue
			}
			iface, ok := typeSpec.Type.(*ast.InterfaceType)
			if !ok {
				continue
			}
			for _, method := range iface.Methods.List {
				for _, methodName := range method.Names {
					result = append(result, methodName.Name)
				}
			}
		}
	}
	sort.Strings(result)
	return result
}

func TestL302StructuralMutationFixturesFailClosed(t *testing.T) {
	fixtures := map[string]string{
		"alias bypass":              `package mux; type Mux struct{ n int }; func bad(m *Mux){ alias:=m; alias.n++ }`,
		"field bypass":              `package mux; type Mux struct{ n int }; type box struct{ mux *Mux }; func bad(m *Mux){ b:=box{mux:m}; b.mux.n++ }`,
		"func literal capture":      `package mux; type Mux struct{ n int }; func bad(m *Mux){ f:=func(){m.n++}; f() }`,
		"generic composite":         `package mux; type Mux struct{ n int }; type box[T any] struct{ v T }; func bad(m *Mux){ b:=box[*Mux]{v:m}; b.v.n++ }`,
		"atomic add":                `package mux; import "sync/atomic"; type Mux struct{ n atomic.Uint64 }; func bad(m *Mux){ m.n.Add(1) }`,
		"atomic store alias":        `package mux; import "sync/atomic"; type Mux struct{ n atomic.Uint64 }; func bad(m *Mux){ alias:=m; alias.n.Store(1) }`,
		"atomic swap":               `package mux; import "sync/atomic"; type Mux struct{ n atomic.Uint64 }; func bad(m *Mux){ m.n.Swap(1) }`,
		"atomic compare and swap":   `package mux; import "sync/atomic"; type Mux struct{ n atomic.Uint64 }; func bad(m *Mux){ m.n.CompareAndSwap(0,1) }`,
		"atomic method value":       `package mux; import "sync/atomic"; type Mux struct{ n atomic.Uint64 }; func bad(m *Mux){ store:=m.n.Store; store(1) }`,
		"atomic method value call":  `package mux; import "sync/atomic"; type Mux struct{ n atomic.Uint64 }; func bad(m *Mux){ (m.n.Store)(1) }`,
		"helper closure alias":      `package mux; type Mux struct{ n int }; func bad(m *Mux){ alias:=m; helper:=func(){ alias.n++ }; helper() }`,
		"transitive locator fields": `package mux; type Mux struct{ n int }; type inner struct{ mux *Mux }; type outer struct{ in inner }; func bad(m *Mux){ a:=outer{in:inner{mux:m}}; b:=a.in; c:=b.mux; c.n++ }`,
	}
	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), name+".go", source, 0)
			if err != nil {
				t.Fatal(err)
			}
			function := file.Decls[len(file.Decls)-1].(*ast.FuncDecl)
			if !l302FunctionWritesState(function) {
				t.Fatal("mutation fixture escaped recursive alias/capture/composite analysis")
			}
		})
	}
	t.Run("helper alias bypass", func(t *testing.T) {
		const source = `package mux; type Mux struct{ n int }; func mutate(m *Mux){ m.n++ }; func bad(m *Mux){ alias:=m; mutate(alias) }; func badMethodValue(m *Mux){ method:=mutate; call:=method; call(m) }`
		file, err := parser.ParseFile(token.NewFileSet(), "helper.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		var functions []l302SourceFunction
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			functions = append(functions, l302SourceFunction{
				pkg: "mux", path: "helper.go", name: function.Name.Name, decl: function,
				calls: l302QualifiedCalls(function.Body), directWrite: l302FunctionWritesState(function),
			})
		}
		closure := l302MutationClosure(functions)
		for _, function := range functions {
			if strings.HasPrefix(function.name, "bad") && !closure[l302FunctionID(function)] {
				t.Fatal("helper mutation fixture escaped recursive call closure")
			}
		}
	})

	for name, source := range map[string]string{
		"originless owner delegate": `package mux; func (w *WindowOwner) Bad(id PaneID) (bool,error) { return w.owner.mux.setTitle(mutationScope{},id,"x") }`,
		"scope created but ignored": `package mux; func (w *WindowOwner) Bad(id PaneID) (bool,error) { scope,err:=w.beginWindowDispatch(); if err!=nil{return false,err}; defer w.owner.leave(scope); return w.owner.mux.setTitle(mutationScope{},id,"x") }`,
		"validate then delegate":    `package mux; func (w *WindowOwner) Bad(id PaneID) (bool,error) { if err:=w.validatePane(id); err!=nil{return false,err}; return w.owner.mux.setTitle(mutationScope{},id,"x") }`,
	} {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), name+".go", source, 0)
			if err != nil {
				t.Fatal(err)
			}
			function := file.Decls[0].(*ast.FuncDecl)
			calls := l302TopLevelCallOrder(function.Body)
			if l302Ordered(calls, "beginWindowDispatch", "leave", "setTitle") && l302WindowSinkReceivesScope(function.Body, "setTitle") {
				t.Fatal("originless/ignored-scope fixture was accepted")
			}
		})
	}
}

func l302WindowSinkReceivesScope(body *ast.BlockStmt, sink string) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || l302CalledName(call.Fun) != sink || len(call.Args) == 0 {
			return true
		}
		identity, ok := call.Args[0].(*ast.Ident)
		found = ok && identity.Name == "scope"
		return !found
	})
	return found
}

func l302FirstStatementChecksError(body *ast.BlockStmt) bool {
	if body == nil || len(body.List) == 0 {
		return false
	}
	statement, ok := body.List[0].(*ast.IfStmt)
	if !ok || statement.Init == nil || len(statement.Body.List) == 0 {
		return false
	}
	_, returns := statement.Body.List[0].(*ast.ReturnStmt)
	return returns
}

func TestEveryStructurallyDerivedOwnerMutationRejectsWrongThreadReentrantStaleAndClosed(t *testing.T) {
	invokers := l302OwnerMutationInvokers()
	derived := l302MutationMethods(t)
	if len(invokers) != len(derived) {
		t.Fatalf("mutation invokers=%d structurally-derived=%d", len(invokers), len(derived))
	}
	for name := range derived {
		if invokers[name] == nil {
			t.Fatalf("structurally-derived mutation %s has no runtime rejection test", name)
		}
	}
	for name, invoke := range invokers {
		t.Run(name, func(t *testing.T) {
			owner := NewOwner(&fakeFactory{}, Options{})
			if _, _, _, err := testWindowOwnerForOwner(owner).Bootstrap(SpawnSpec{}, PixelRect{Width: 80, Height: 24}, CellMetrics{CellWidth: 1, CellHeight: 1}); err != nil {
				t.Fatal(err)
			}
			scope, err := owner.begin()
			if err != nil {
				t.Fatal(err)
			}
			before := captureL302MuxFingerprint(owner.mux)
			if err := invoke(owner); !errors.Is(err, ErrOwnerBusy) {
				t.Fatalf("reentrant error=%v", err)
			}
			crossThread := make(chan error, 1)
			go func() { crossThread <- invoke(owner) }()
			if err := <-crossThread; !errors.Is(err, ErrWrongOwnerThread) {
				t.Fatalf("wrong-thread error=%v", err)
			}
			if after := captureL302MuxFingerprint(owner.mux); !reflect.DeepEqual(before, after) {
				t.Fatalf("rejected mutation changed state: before=%#v after=%#v", before, after)
			}
			owner.leave(scope)
			owner.state.generation.Add(1)
			if err := invoke(owner); !errors.Is(err, ErrStaleOwner) {
				t.Fatalf("stale error=%v", err)
			}
			owner.state.generation.Store(owner.generation)
			if err := owner.Shutdown(); err != nil {
				t.Fatal(err)
			}
			closedBefore := captureL302MuxFingerprint(owner.mux)
			err = invoke(owner)
			if name == "Shutdown" || name == "Close" {
				if err != nil {
					t.Fatalf("idempotent close error=%v", err)
				}
			} else if !errors.Is(err, ErrOwnerClosed) {
				t.Fatalf("closed error=%v", err)
			}
			if after := captureL302MuxFingerprint(owner.mux); !reflect.DeepEqual(closedBefore, after) {
				t.Fatalf("closed rejection changed state: before=%#v after=%#v", closedBefore, after)
			}
		})
	}
}
