package mux

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

	"cervterm/internal/termimage"
)

var l302OwnerReadMethods = map[string]bool{
	"Generation": true, "Closed": true, "FocusedPane": true, "PaneIDs": true,
	"Layout": true, "PaneView": true, "Tabs": true, "ActiveTab": true,
	"TabForPane": true, "Windows": true, "WindowForPane": true, "WindowForTab": true,
	"WorkspaceForWindow": true, "Workspaces": true, "ActiveWorkspace": true,
	"FreshSessionSnapshot": true, "RestoreWindowIDs": true, "ResolveEventAddresses": true,
	"AcquireImageResource": true, "ImageSetupError": true, "NextImageDeadline": true,
	"GlobalRowToViewport": true, "Line": true, "LineWrapped": true,
	"QuickSelectSnapshot": true, "QuickSelectSnapshotCurrent": true, "SemanticSnapshot": true,
	"SemanticSnapshotCurrent": true, "ValidateSemanticRange": true, "SemanticRangeText": true,
	"ReplyCounters": true,
	"ForWindow":     true,
}

func l302MutationMethods(t *testing.T) map[string]bool {
	t.Helper()
	files := l302ProductionFiles(t, ".")
	ownerMethods := make(map[string]*ast.FuncDecl)
	result := make(map[string]bool)
	for _, file := range files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || l302ReceiverName(function) != "Owner" || function.Body == nil {
				continue
			}
			ownerMethods[function.Name.Name] = function
			if !l302OwnerReadMethods[function.Name.Name] && l302ContainsCall(l302CallNames(function.Body), "begin") {
				result[function.Name.Name] = true
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for name, function := range ownerMethods {
			if result[name] || l302OwnerReadMethods[name] {
				continue
			}
			for _, call := range l302CallNames(function.Body) {
				if result[call] {
					result[name] = true
					changed = true
					break
				}
			}
		}
	}
	return result
}

func l302ReceiverName(decl *ast.FuncDecl) string {
	if decl.Recv == nil || len(decl.Recv.List) != 1 {
		return ""
	}
	typeName := decl.Recv.List[0].Type
	if star, ok := typeName.(*ast.StarExpr); ok {
		typeName = star.X
	}
	if ident, ok := typeName.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

func l302MuxVariables(decl *ast.FuncDecl) map[string]bool {
	result := make(map[string]bool)
	if decl.Recv != nil && l302ReceiverName(decl) == "Mux" && len(decl.Recv.List[0].Names) == 1 {
		result[decl.Recv.List[0].Names[0].Name] = true
	}
	if decl.Type.Params == nil {
		return result
	}
	for _, field := range decl.Type.Params.List {
		star, ok := field.Type.(*ast.StarExpr)
		ident, named := starIdent(star, ok)
		if !named || ident != "Mux" {
			continue
		}
		for _, name := range field.Names {
			result[name.Name] = true
		}
	}
	return result
}

func starIdent(star *ast.StarExpr, ok bool) (string, bool) {
	if !ok || star == nil {
		return "", false
	}
	ident, named := star.X.(*ast.Ident)
	if !named {
		return "", false
	}
	return ident.Name, true
}

func l302ProductionFiles(t *testing.T, dir string) map[string]*ast.File {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string]*ast.File)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		files[path] = file
	}
	return files
}

func l302CallNames(body *ast.BlockStmt) []string {
	var result []string
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch function := call.Fun.(type) {
		case *ast.Ident:
			result = append(result, function.Name)
		case *ast.SelectorExpr:
			result = append(result, function.Sel.Name)
		}
		return true
	})
	return result
}

func l302ContainsCall(calls []string, name string) bool {
	for _, call := range calls {
		if call == name {
			return true
		}
	}
	return false
}

func l302MuxMutationSink(ownerMethod string) string {
	if ownerMethod == "AbortRestore" {
		return "abortRestoreOwned"
	}
	return strings.ToLower(ownerMethod[:1]) + ownerMethod[1:]
}

func TestL302OwnerMutationInventoryMatchesProductionFacadeAndGate(t *testing.T) {
	files := l302ProductionFiles(t, ".")
	mutations := l302MutationMethods(t)
	found := make(map[string]int)
	for path, file := range files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || l302ReceiverName(function) != "Owner" {
				continue
			}
			name := function.Name.Name
			switch {
			case name == "begin" || name == "enter" || name == "enterAttested" || name == "leave" || name == "publishClosed":
				continue
			case l302OwnerReadMethods[name]:
				if mutations[name] {
					t.Fatalf("read method %s also classified as mutation", name)
				}
				continue
			case mutations[name]:
				found[name]++
				calls := l302CallNames(function.Body)
				if target, alias := map[string]string{"Close": "Shutdown", "Split": "SpawnSplit"}[name]; alias {
					if !l302ContainsCall(calls, target) {
						t.Fatalf("%s: Owner.%s must delegate to guarded Owner.%s", path, name, target)
					}
					continue
				}
				if !l302ContainsCall(calls, "begin") || !l302ContainsCall(calls, "leave") {
					t.Fatalf("%s: Owner.%s can bypass enter/leave", path, name)
				}
				if name == "Shutdown" {
					if !l302ContainsCall(calls, "publishClosed") {
						t.Fatalf("%s: Shutdown does not publish closed generation", path)
					}
				} else if sink := l302MuxMutationSink(name); !l302ContainsCall(calls, sink) {
					t.Fatalf("%s: Owner.%s does not call exact private Mux sink %s", path, name, sink)
				}
			default:
				t.Fatalf("%s: unclassified Owner method %s", path, name)
			}
		}
	}
	for name := range mutations {
		if found[name] != 1 {
			t.Fatalf("Owner mutation %s declarations=%d want=1", name, found[name])
		}
	}
}

func TestL302RecursiveProductionMuxMutationCallsCannotBypassOwnerFacade(t *testing.T) {
	files := l302ProductionFiles(t, ".")
	mutations := l302MutationMethods(t)
	for path, file := range files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			muxVariables := l302MuxVariables(function)
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || !mutations[selector.Sel.Name] {
					return true
				}
				ident, direct := selector.X.(*ast.Ident)
				if !direct || !muxVariables[ident.Name] {
					return true
				}
				if l302ReceiverName(function) == "Mux" {
					return true
				}
				t.Errorf("%s: %s calls Mux mutation %s outside Owner/Mux call graph", path, function.Name.Name, selector.Sel.Name)
				return true
			})
		}
	}
}

func TestL302ProductionConstructorAndServiceLocatorExposureAreAbsent(t *testing.T) {
	files := l302ProductionFiles(t, ".")
	newMuxCalls := 0
	for path, file := range files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if function.Name.Name == "New" {
				t.Fatalf("%s: obsolete public concrete Mux constructor remains", path)
			}
			if l302ReceiverName(function) == "Owner" && function.Type.Results != nil {
				for _, field := range function.Type.Results.List {
					if star, ok := field.Type.(*ast.StarExpr); ok {
						if ident, ok := star.X.(*ast.Ident); ok && ident.Name == "Mux" {
							t.Fatalf("%s: Owner.%s exposes *Mux service locator", path, function.Name.Name)
						}
					}
				}
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				ident, direct := callFunctionIdent(call, ok)
				if direct && ident == "newMux" {
					newMuxCalls++
					if function.Name.Name != "NewOwner" {
						t.Errorf("%s: %s bypasses sole NewOwner constructor", path, function.Name.Name)
					}
				}
				return true
			})
		}
	}
	if newMuxCalls != 1 {
		t.Fatalf("production newMux calls=%d want exact NewOwner call", newMuxCalls)
	}
	ownerType := reflect.TypeOf(Owner{})
	for i := 0; i < ownerType.NumField(); i++ {
		if ownerType.Field(i).IsExported() {
			t.Fatalf("Owner exported field %s exposes capability state", ownerType.Field(i).Name)
		}
	}
}

func callFunctionIdent(call *ast.CallExpr, ok bool) (string, bool) {
	if !ok || call == nil {
		return "", false
	}
	ident, direct := call.Fun.(*ast.Ident)
	if !direct {
		return "", false
	}
	return ident.Name, true
}

func TestL302OwnerErrorsAndFastPathArePinned(t *testing.T) {
	for err, want := range map[error]string{
		ErrOwnerRequired:      "mux: owner capability required",
		ErrWrongOwner:         "mux: wrong owner capability",
		ErrWrongOwnerThread:   "mux: owner mutation called from another native thread",
		ErrWrongOrigin:        "mux: request origin does not own target",
		ErrStaleOwner:         "mux: stale owner generation",
		ErrOwnerClosed:        "mux: owner is closed",
		ErrOwnerBusy:          "mux: owner mutation already active",
		ErrStaleMutationScope: "mux: mutation scope is not active",
	} {
		if err.Error() != want || !errors.Is(err, err) {
			t.Fatalf("typed owner error=%q want=%q", err, want)
		}
	}
	ownerSource, err := os.ReadFile("owner.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(ownerSource)
	for _, forbidden := range []string{"sync.Mutex", "sync.RWMutex", "make(chan"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("owner fast path contains forbidden %q", forbidden)
		}
	}
}

func TestL302NoNativeWindowOrGLCallsBelowOwnerBoundary(t *testing.T) {
	for _, dir := range []string{".", "../termimage"} {
		for path, file := range l302ProductionFiles(t, dir) {
			for _, spec := range file.Imports {
				value := strings.Trim(spec.Path.Value, "\"")
				lower := strings.ToLower(value)
				if strings.Contains(lower, "glfw") || strings.Contains(lower, "go-gl") || strings.Contains(lower, "opengl") {
					t.Fatalf("%s imports native window/GL package %s", path, value)
				}
			}
		}
	}
}

func TestL302TermimageOwnerPublicationSurfaceIsExhaustive(t *testing.T) {
	files := l302ProductionFiles(t, "../termimage")
	want := []string{"Close", "PrepareCandidate", "PrepareCandidateWithRetention", "PrepareClose", "PrepareReset", "PrepareResourceRemoval", "PublishPrepared"}
	var got []string
	methods := make(map[string]*ast.FuncDecl)
	for _, file := range files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			receiver := l302ReceiverName(function)
			methods[receiver+"."+function.Name.Name] = function
			if receiver != "StoreOwner" || strings.HasPrefix(function.Name.Name, "enter") || function.Name.Name == "leave" {
				continue
			}
			got = append(got, function.Name.Name)
			calls := l302CallNames(function.Body)
			switch function.Name.Name {
			case "PrepareClose":
				if !l302ContainsCall(calls, "enter") || !l302ContainsCall(calls, "leave") {
					t.Fatalf("StoreOwner.PrepareClose must acquire the gate and release rejected preflights")
				}
			case "Close":
				if !l302ContainsCall(calls, "PrepareClose") || !l302ContainsCall(calls, "Commit") || l302ContainsCall(calls, "closeOwned") {
					t.Fatalf("StoreOwner.Close must use the reserved PrepareClose/Commit transaction")
				}
			default:
				if !l302ContainsCall(calls, "enter") || !l302ContainsCall(calls, "leave") {
					t.Fatalf("StoreOwner.%s bypasses enter/leave", function.Name.Name)
				}
			}
		}
	}
	for _, name := range []string{"Commit", "Abort"} {
		function := methods["PreparedStoreClose."+name]
		if function == nil {
			t.Fatalf("PreparedStoreClose.%s missing", name)
		}
		calls := l302CallNames(function.Body)
		if !l302ContainsCall(calls, "enterPreparedClose") || !l302ContainsCall(calls, "leave") {
			t.Fatalf("PreparedStoreClose.%s does not reacquire and resolve a fresh owner scope", name)
		}
		if name == "Commit" && !l302ContainsCall(calls, "closeOwned") {
			t.Fatal("PreparedStoreClose.Commit does not commit the exact owned store")
		}
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("StoreOwner mutation inventory=%v want=%v", got, want)
	}
	for _, removed := range []string{"PrepareCandidate", "PrepareCandidateWithRetention", "PrepareResourceRemoval", "PublishPrepared"} {
		if methods["Store."+removed] != nil {
			t.Fatalf("expired unscoped Store adapter %s remains", removed)
		}
	}
	scopedSinks := map[string]string{
		"Store.prepareCandidate":              "prepareCandidateWithRetention",
		"Store.prepareCandidateWithRetention": "valid",
		"Store.prepareResourceRemoval":        "valid",
		"Store.prepareReset":                  "valid",
		"Store.publishPrepared":               "valid",
		"Store.closeOwned":                    "valid",
		"Store.resetState":                    "valid",
		"PreparedStoreState.commit":           "validateScope",
		"PreparedStoreState.abort":            "validateScope",
		"PreparedStoreState.validateScope":    "valid",
		"PreparedStoreState.abortOwnership":   "validateScope",
	}
	for sink, guardCall := range scopedSinks {
		function := methods[sink]
		if function == nil || function.Type.Params == nil || len(function.Type.Params.List) == 0 {
			t.Fatalf("scope-required private sink %s missing", sink)
		}
		first := function.Type.Params.List[0]
		typeName, ok := first.Type.(*ast.Ident)
		if !ok || typeName.Name != "storeMutationScope" || len(first.Names) != 1 || first.Names[0].Name != "scope" {
			t.Fatalf("%s first parameter is not exact scope storeMutationScope", sink)
		}
		if !l302ContainsCall(l302CallNames(function.Body), guardCall) {
			t.Fatalf("%s does not preserve required %s validation/delegation", sink, guardCall)
		}
	}
	for _, err := range []error{termimage.ErrWrongOwner, termimage.ErrStaleOwner, termimage.ErrOwnerBusy, termimage.ErrClosed} {
		if err == nil || err.Error() == "" {
			t.Fatal("termimage typed owner error missing")
		}
	}
}

func l302ExpressionRoot(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return l302ExpressionRoot(value.X)
	case *ast.IndexExpr:
		return l302ExpressionRoot(value.X)
	case *ast.StarExpr:
		return l302ExpressionRoot(value.X)
	case *ast.ParenExpr:
		return l302ExpressionRoot(value.X)
	}
	return ""
}

func l302DirectStateWrite(function *ast.FuncDecl) bool {
	if function == nil || function.Body == nil || function.Recv == nil || len(function.Recv.List) != 1 || len(function.Recv.List[0].Names) != 1 {
		return false
	}
	receiver := function.Recv.List[0].Names[0].Name
	writes := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			if value.Tok == token.DEFINE {
				return true
			}
			for _, target := range value.Lhs {
				if l302ExpressionRoot(target) == receiver {
					writes = true
				}
			}
		case *ast.IncDecStmt:
			if l302ExpressionRoot(value.X) == receiver {
				writes = true
			}
		case *ast.SendStmt:
			if l302ExpressionRoot(value.Chan) == receiver {
				writes = true
			}
		case *ast.CallExpr:
			if ident, ok := value.Fun.(*ast.Ident); ok && (ident.Name == "delete" || ident.Name == "clear") && len(value.Args) > 0 && l302ExpressionRoot(value.Args[0]) == receiver {
				writes = true
			}
		}
		return !writes
	})
	return writes
}

func TestL302MutationInventoryIsDerivedFromProductionWritesAndCalls(t *testing.T) {
	files := l302ProductionFiles(t, ".")
	type methodInfo struct {
		receiver string
		name     string
		exported bool
		calls    []string
		writes   bool
	}
	var methods []methodInfo
	mutatingNames := make(map[string]bool)
	for _, file := range files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || function.Recv == nil {
				continue
			}
			info := methodInfo{receiver: l302ReceiverName(function), name: function.Name.Name, exported: function.Name.IsExported(), calls: l302CallNames(function.Body), writes: l302DirectStateWrite(function)}
			methods = append(methods, info)
			if info.writes {
				mutatingNames[info.name] = true
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, method := range methods {
			if mutatingNames[method.name] {
				continue
			}
			for _, call := range method.calls {
				if mutatingNames[call] {
					mutatingNames[method.name] = true
					changed = true
					break
				}
			}
		}
	}
	ownerMutations := l302MutationMethods(t)
	for _, method := range methods {
		if method.receiver != "Mux" || !method.exported || !mutatingNames[method.name] {
			continue
		}
		if !ownerMutations[method.name] {
			t.Errorf("production writes/calls derive unguarded Mux mutation %s", method.name)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
	if len(ownerMutations) == 0 {
		t.Fatal("structural mutation inventory is empty")
	}
}
