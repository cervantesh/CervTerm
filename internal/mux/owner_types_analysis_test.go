package mux

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

type l302TypedResult struct {
	writes     map[string]bool
	locators   []string
	unresolved []string
}

type l302GoListPackage struct {
	Dir             string
	ImportPath      string
	Export          string
	CompiledGoFiles []string
	Error           *struct{ Err string }
}

var (
	l302TypedProductionOnce   sync.Once
	l302TypedProductionCached l302TypedResult
	l302TypedProductionErr    error
)

func l302AnalyzeTypedFixture(t *testing.T, source string) l302TypedResult {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", source, parser.AllErrors)
	if err != nil {
		t.Fatal(err)
	}
	info := l302NewTypesInfo()
	var typeErrors []string
	config := &types.Config{
		Importer: importer.Default(),
		Error:    func(err error) { typeErrors = append(typeErrors, err.Error()) },
	}
	if _, err := config.Check("fixture/mux", fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("type-check fixture: %v (%s)", err, strings.Join(typeErrors, "; "))
	}
	result := l302TypedResult{writes: make(map[string]bool)}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		writes, locator, unresolved := l302TypedFunctionWrites(function, info, function.Name.Name, false)
		result.writes[function.Name.Name] = writes
		if locator {
			result.locators = append(result.locators, function.Name.Name)
		}
		result.unresolved = append(result.unresolved, unresolved...)
	}
	sort.Strings(result.locators)
	sort.Strings(result.unresolved)
	return result
}

func l302AnalyzeUnresolvedProductionFixture(t *testing.T, source string) l302TypedResult {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "unresolved.go", source, parser.AllErrors)
	if err != nil {
		t.Fatal(err)
	}
	info := l302NewTypesInfo()
	config := &types.Config{Importer: importer.Default(), Error: func(error) {}}
	_, _ = config.Check("fixture/mux", fset, []*ast.File{file}, info)
	result := l302TypedResult{writes: make(map[string]bool)}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		writes, locator, unresolved := l302TypedFunctionWrites(function, info, function.Name.Name, true)
		result.writes[function.Name.Name] = writes
		if locator {
			result.locators = append(result.locators, function.Name.Name)
		}
		result.unresolved = append(result.unresolved, unresolved...)
	}
	return result
}

func l302TypedProductionAnalysis(t *testing.T) l302TypedResult {
	t.Helper()
	l302TypedProductionOnce.Do(func() {
		root, err := filepath.Abs("../..")
		if err != nil {
			l302TypedProductionErr = err
			return
		}
		l302TypedProductionCached, l302TypedProductionErr = l302LoadTypedProduction(root)
	})
	if l302TypedProductionErr != nil {
		t.Fatal(l302TypedProductionErr)
	}
	return l302TypedProductionCached
}

func l302LoadTypedProduction(root string) (l302TypedResult, error) {
	result := l302TypedResult{writes: make(map[string]bool)}
	targets := []string{"./internal/mux", "./internal/core", "./internal/termimage", "./internal/ownerthread"}
	if runtime.GOOS == "windows" {
		targets = append(targets, "./internal/frontend/glfwgl")
	}
	args := append([]string{"list", "-deps", "-export", "-compiled", "-json", "-tags=glfw"}, targets...)
	command := exec.Command("go", args...)
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return result, fmt.Errorf("go list typed production: %w: %s", err, strings.TrimSpace(string(exit.Stderr)))
		}
		return result, fmt.Errorf("go list typed production: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	packages := make(map[string]l302GoListPackage)
	exports := make(map[string]string)
	for {
		var pkg l302GoListPackage
		if err := decoder.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}
			return result, fmt.Errorf("decode go list: %w", err)
		}
		if pkg.Error != nil {
			return result, fmt.Errorf("go list %s: %s", pkg.ImportPath, pkg.Error.Err)
		}
		if pkg.Export != "" {
			exports[pkg.ImportPath] = pkg.Export
		}
		packages[pkg.ImportPath] = pkg
	}
	lookup := func(path string) (io.ReadCloser, error) {
		export := exports[path]
		if export == "" {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(export)
	}
	wanted := []string{"cervterm/internal/mux", "cervterm/internal/core", "cervterm/internal/termimage", "cervterm/internal/ownerthread"}
	if runtime.GOOS == "windows" {
		wanted = append(wanted, "cervterm/internal/frontend/glfwgl")
	}
	for _, importPath := range wanted {
		pkg, ok := packages[importPath]
		if !ok {
			return result, fmt.Errorf("go list omitted %s", importPath)
		}
		fset := token.NewFileSet()
		var files []*ast.File
		paths := make(map[*ast.File]string)
		for _, listed := range pkg.CompiledGoFiles {
			path := listed
			if !filepath.IsAbs(path) {
				path = filepath.Join(pkg.Dir, listed)
			}
			file, parseErr := parser.ParseFile(fset, path, nil, parser.AllErrors)
			if parseErr != nil {
				return result, fmt.Errorf("parse compiled %s: %w", path, parseErr)
			}
			files = append(files, file)
			if rel, relErr := filepath.Rel(root, path); relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				paths[file] = filepath.ToSlash(rel)
			}
		}
		info := l302NewTypesInfo()
		var typeErrors []string
		config := &types.Config{
			Importer:    importer.ForCompiler(fset, "gc", lookup),
			FakeImportC: true,
			Error:       func(err error) { typeErrors = append(typeErrors, err.Error()) },
		}
		if _, checkErr := config.Check(importPath, fset, files, info); checkErr != nil {
			return result, fmt.Errorf("type-check %s: %w (%s)", importPath, checkErr, strings.Join(typeErrors, "; "))
		}
		for _, file := range files {
			path := paths[file]
			if path == "" || strings.HasSuffix(path, "_test.go") {
				continue
			}
			for _, declaration := range file.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Body == nil {
					continue
				}
				id := path + ":" + function.Name.Name
				if receiver := l302ReceiverName(function); receiver != "" {
					id = path + ":" + receiver + "." + function.Name.Name
				}
				writes, locator, unresolved := l302TypedFunctionWrites(function, info, id, true)
				if writes {
					result.writes[id] = true
				}
				if locator {
					result.locators = append(result.locators, id)
				}
				for _, form := range unresolved {
					result.unresolved = append(result.unresolved, id+": "+form)
				}
			}
		}
	}
	sort.Strings(result.locators)
	sort.Strings(result.unresolved)
	return result, nil
}

func l302NewTypesInfo() *types.Info {
	return &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Implicits:  make(map[ast.Node]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Scopes:     make(map[ast.Node]*types.Scope),
		Instances:  make(map[*ast.Ident]types.Instance),
	}
}

func l302TypedFunctionWrites(function *ast.FuncDecl, info *types.Info, id string, production bool) (bool, bool, []string) {
	if function == nil || function.Body == nil || info == nil {
		if production {
			return true, false, []string{"missing go/types information in production mutation scope"}
		}
		return false, false, []string{"missing go/types information"}
	}
	state := make(map[types.Object]bool)
	seed := func(fields *ast.FieldList) {
		if fields == nil {
			return
		}
		for _, field := range fields.List {
			for _, name := range field.Names {
				object := info.Defs[name]
				if object == nil {
					continue
				}
				if l302MutableType(object.Type()) {
					state[object] = true
				}
			}
		}
	}
	seed(function.Recv)
	seed(function.Type.Params)

	for changed := true; changed; {
		changed = false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.AssignStmt:
				for index, left := range value.Lhs {
					right := l302AssignmentRight(value.Rhs, index)
					if right != nil && l302MarkTypedAlias(left, right, info, state) {
						changed = true
					}
				}
			case *ast.ValueSpec:
				for index, name := range value.Names {
					right := l302AssignmentRight(value.Values, index)
					if right != nil && l302MarkTypedAlias(name, right, info, state) {
						changed = true
					}
				}
			case *ast.RangeStmt:
				if !l302TypedExprState(value.X, info, state) {
					break
				}
				for _, target := range []ast.Expr{value.Key, value.Value} {
					identity, ok := target.(*ast.Ident)
					if !ok || identity.Name == "_" {
						continue
					}
					object := info.ObjectOf(identity)
					if object != nil && l302MutableType(object.Type()) && !state[object] {
						state[object] = true
						changed = true
					}
				}
			}
			return true
		})
	}

	mutators := make(map[types.Object]bool)
	for changed := true; changed; {
		changed = false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			var lefts []ast.Expr
			var rights []ast.Expr
			switch value := node.(type) {
			case *ast.AssignStmt:
				lefts, rights = value.Lhs, value.Rhs
			case *ast.ValueSpec:
				for _, name := range value.Names {
					lefts = append(lefts, name)
				}
				rights = value.Values
			default:
				return true
			}
			for index, left := range lefts {
				right := l302AssignmentRight(rights, index)
				identity, ok := left.(*ast.Ident)
				if !ok || right == nil {
					continue
				}
				object := info.ObjectOf(identity)
				if object != nil && !mutators[object] && l302TypedAtomicMutator(right, info, state, mutators) {
					mutators[object] = true
					changed = true
				}
			}
			return true
		})
	}

	allowCapability := production && l302CapabilityAllowlist[id]
	locator := false
	if !allowCapability && l302SignatureReturnsCapability(info.TypeOf(function.Name)) {
		locator = true
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if allowCapability || locator {
			return !locator
		}
		switch value := node.(type) {
		case *ast.FuncLit:
			if l302SignatureReturnsCapability(info.TypeOf(value)) {
				locator = true
				return false
			}
		case *ast.ReturnStmt:
			for _, result := range value.Results {
				if l302CapabilityType(info.TypeOf(result)) {
					locator = true
					return false
				}
			}
		}
		return true
	})

	writes := locator
	var unresolved []string
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if writes {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				if !isBareIdentifier(left) && l302TypedExprState(left, info, state) {
					writes = true
					break
				}
			}
		case *ast.IncDecStmt:
			writes = l302TypedExprState(value.X, info, state)
		case *ast.SendStmt:
			writes = l302TypedExprState(value.Chan, info, state)
		case *ast.CallExpr:
			if info.TypeOf(value.Fun) == nil && l302CallTouchesTypedState(value, info, state) {
				unresolved = append(unresolved, fmt.Sprintf("unresolved call at %d", value.Pos()))
				if production {
					writes = true
				}
				break
			}
			if l302TypedAtomicMutator(value.Fun, info, state, mutators) {
				writes = true
				break
			}
			name := l302CalledName(value.Fun)
			if (name == "delete" || name == "clear" || name == "copy" || name == "append") && len(value.Args) > 0 && l302TypedExprState(value.Args[0], info, state) {
				writes = true
			}
		}
		return !writes
	})
	return writes, locator, unresolved
}

var l302CapabilityAllowlist = map[string]bool{
	"internal/mux/mux.go:newMux":     true,
	"internal/mux/owner.go:NewOwner": true,
}

func l302ASTCapabilityLocator(function l302SourceFunction, id string) bool {
	if function.decl == nil || l302CapabilityAllowlist[id] {
		return false
	}
	if l302ASTResultsCapability(function.decl.Type.Results) {
		return true
	}
	found := false
	ast.Inspect(function.decl.Body, func(node ast.Node) bool {
		literal, ok := node.(*ast.FuncLit)
		if ok && l302ASTResultsCapability(literal.Type.Results) {
			found = true
			return false
		}
		return !found
	})
	return found
}

func l302ASTResultsCapability(results *ast.FieldList) bool {
	if results == nil {
		return false
	}
	for _, field := range results.List {
		if l302ASTCapabilityType(field.Type) {
			return true
		}
	}
	return false
}

func l302ASTCapabilityType(expression ast.Expr) bool {
	switch value := expression.(type) {
	case *ast.StarExpr:
		switch target := value.X.(type) {
		case *ast.Ident:
			return target.Name == "Mux" || target.Name == "Owner"
		case *ast.SelectorExpr:
			return target.Sel.Name == "Mux" || target.Sel.Name == "Owner"
		}
	case *ast.Ident:
		return value.Name == "processCommandCapability"
	case *ast.ParenExpr:
		return l302ASTCapabilityType(value.X)
	}
	return false
}

var l302DetachedReadAllowlist = map[string]bool{
	"cervterm/internal/mux.AcquireImageResource":  true,
	"cervterm/internal/mux.FreshSessionSnapshot":  true,
	"cervterm/internal/mux.Layout":                true,
	"cervterm/internal/mux.PaneIDs":               true,
	"cervterm/internal/mux.PaneView":              true,
	"cervterm/internal/mux.QuickSelectSnapshot":   true,
	"cervterm/internal/mux.ResolveEventAddresses": true,
	"cervterm/internal/mux.SemanticSnapshot":      true,
	"cervterm/internal/mux.Tabs":                  true,
	"cervterm/internal/mux.Windows":               true,
	"cervterm/internal/mux.Workspaces":            true,
}

func l302AssignmentRight(rights []ast.Expr, index int) ast.Expr {
	if len(rights) == 0 {
		return nil
	}
	if len(rights) == 1 {
		return rights[0]
	}
	if index < len(rights) {
		return rights[index]
	}
	return nil
}

func l302MarkTypedAlias(left ast.Expr, right ast.Expr, info *types.Info, state map[types.Object]bool) bool {
	identity, ok := left.(*ast.Ident)
	if !ok || identity.Name == "_" {
		return false
	}
	object := info.ObjectOf(identity)
	if object == nil || state[object] || !l302MutableType(object.Type()) || !l302TypedExprState(right, info, state) {
		return false
	}
	state[object] = true
	return true
}

func l302MutableType(value types.Type) bool {
	return l302MutableTypeSeen(value, make(map[types.Type]bool))
}

func l302MutableTypeSeen(value types.Type, seen map[types.Type]bool) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if seen[value] {
		return false
	}
	seen[value] = true
	switch typed := value.(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Interface, *types.Signature, *types.TypeParam:
		return true
	case *types.Named:
		return l302MutableTypeSeen(typed.Underlying(), seen)
	case *types.Array:
		return l302MutableTypeSeen(typed.Elem(), seen)
	case *types.Struct:
		for index := 0; index < typed.NumFields(); index++ {
			if l302MutableTypeSeen(typed.Field(index).Type(), seen) {
				return true
			}
		}
	case *types.Tuple:
		for index := 0; index < typed.Len(); index++ {
			if l302MutableTypeSeen(typed.At(index).Type(), seen) {
				return true
			}
		}
	}
	return false
}

func l302CapabilityType(value types.Type) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	named, ok := value.(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil {
		return false
	}
	name := named.Obj().Name()
	path := named.Obj().Pkg().Path()
	if name == "Mux" || name == "Owner" {
		return path == "cervterm/internal/mux" || path == "fixture/mux"
	}
	return name == "processCommandCapability" && (path == "cervterm/internal/frontend/glfwgl" || path == "fixture/mux")
}

func l302SignatureReturnsCapability(value types.Type) bool {
	if value == nil {
		return false
	}
	signature, ok := types.Unalias(value).(*types.Signature)
	if !ok || signature.Results() == nil {
		return false
	}
	for index := 0; index < signature.Results().Len(); index++ {
		if l302CapabilityType(signature.Results().At(index).Type()) {
			return true
		}
	}
	return false
}

func l302TypedExprState(expression ast.Expr, info *types.Info, state map[types.Object]bool) bool {
	if expression == nil {
		return false
	}
	switch value := expression.(type) {
	case *ast.Ident:
		return state[info.ObjectOf(value)]
	case *ast.ParenExpr:
		return l302TypedExprState(value.X, info, state)
	case *ast.SelectorExpr:
		return l302TypedExprState(value.X, info, state)
	case *ast.IndexExpr:
		return l302TypedExprState(value.X, info, state)
	case *ast.IndexListExpr:
		return l302TypedExprState(value.X, info, state)
	case *ast.SliceExpr:
		return l302TypedExprState(value.X, info, state)
	case *ast.StarExpr:
		return l302TypedExprState(value.X, info, state)
	case *ast.TypeAssertExpr:
		return l302TypedExprState(value.X, info, state)
	case *ast.UnaryExpr:
		return l302TypedExprState(value.X, info, state)
	case *ast.BinaryExpr:
		return l302TypedExprState(value.X, info, state) || l302TypedExprState(value.Y, info, state)
	case *ast.KeyValueExpr:
		return l302TypedExprState(value.Value, info, state)
	case *ast.CompositeLit:
		for _, element := range value.Elts {
			if l302TypedExprState(element, info, state) {
				return true
			}
		}
		return false
	case *ast.CallExpr:
		if l302CallIsDetachedRead(value, info) {
			return false
		}
		if l302CapabilityType(info.TypeOf(value)) {
			return true
		}
		if !l302MutableType(info.TypeOf(value)) {
			return false
		}
		if l302TypedExprState(value.Fun, info, state) {
			return true
		}
		for _, argument := range value.Args {
			if l302TypedExprState(argument, info, state) {
				return true
			}
		}
		return false
	}
	return false
}

func l302TypedAtomicMutator(expression ast.Expr, info *types.Info, state map[types.Object]bool, aliases map[types.Object]bool) bool {
	switch value := expression.(type) {
	case *ast.ParenExpr:
		return l302TypedAtomicMutator(value.X, info, state, aliases)
	case *ast.Ident:
		return aliases[info.ObjectOf(value)]
	case *ast.SelectorExpr:
		object := l302SelectorObject(value, info)
		function, ok := object.(*types.Func)
		if !ok || function.Pkg() == nil || function.Pkg().Path() != "sync/atomic" || !l302AtomicMutationMethod(function.Name()) {
			return false
		}
		return l302TypedExprState(value.X, info, state)
	}
	return false
}

func l302SelectorObject(selector *ast.SelectorExpr, info *types.Info) types.Object {
	if selection := info.Selections[selector]; selection != nil {
		return selection.Obj()
	}
	return info.Uses[selector.Sel]
}

func l302CallTouchesTypedState(call *ast.CallExpr, info *types.Info, state map[types.Object]bool) bool {
	if l302TypedExprState(call.Fun, info, state) {
		return true
	}
	for _, argument := range call.Args {
		if l302TypedExprState(argument, info, state) {
			return true
		}
	}
	return false
}

func l302CallIsDetachedRead(call *ast.CallExpr, info *types.Info) bool {
	object := l302CalledObject(call.Fun, info)
	function, ok := object.(*types.Func)
	if !ok || function.Pkg() == nil {
		return false
	}
	return l302DetachedReadAllowlist[function.Pkg().Path()+"."+function.Name()]
}

func l302CalledObject(expression ast.Expr, info *types.Info) types.Object {
	switch value := expression.(type) {
	case *ast.Ident:
		return info.ObjectOf(value)
	case *ast.SelectorExpr:
		return l302SelectorObject(value, info)
	case *ast.ParenExpr:
		return l302CalledObject(value.X, info)
	case *ast.IndexExpr:
		return l302CalledObject(value.X, info)
	case *ast.IndexListExpr:
		return l302CalledObject(value.X, info)
	}
	return nil
}
