package mux

import (
	"bytes"
	"errors"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type fakeRestoreCoordinatorPort struct {
	trace []string

	snapshot          FreshSessionSnapshot
	prepared          *RestoreCandidate
	windowIDs         []WindowID
	events            []Event
	freshErr          error
	prepareErr        error
	windowIDsErr      error
	commitErr         error
	abortErr          error
	windowIDCandidate *RestoreCandidate
	commitCandidate   *RestoreCandidate
	abortCandidate    *RestoreCandidate
}

type fakeRestoreCoordinator = restoreCoordinator[*fakeRestoreCoordinatorPort, *fakeRestoreCoordinatorPort]

func newFakeRestoreCoordinator() fakeRestoreCoordinator {
	return newRestoreCoordinator[*fakeRestoreCoordinatorPort, *fakeRestoreCoordinatorPort]()
}

func (p *fakeRestoreCoordinatorPort) freshSessionSnapshot() (FreshSessionSnapshot, error) {
	p.trace = append(p.trace, "fresh")
	return p.snapshot, p.freshErr
}

func (p *fakeRestoreCoordinatorPort) prepareRestore() (*RestoreCandidate, error) {
	p.trace = append(p.trace, "prepare")
	return p.prepared, p.prepareErr
}

func (p *fakeRestoreCoordinatorPort) restoreWindowIDs(candidate *RestoreCandidate) ([]WindowID, error) {
	p.trace = append(p.trace, "windows")
	p.windowIDCandidate = candidate
	return p.windowIDs, p.windowIDsErr
}

func (p *fakeRestoreCoordinatorPort) commitRestore(candidate *RestoreCandidate) ([]Event, error) {
	p.trace = append(p.trace, "commit")
	p.commitCandidate = candidate
	return p.events, p.commitErr
}

func (p *fakeRestoreCoordinatorPort) abortRestore(candidate *RestoreCandidate) error {
	p.trace = append(p.trace, "abort")
	p.abortCandidate = candidate
	return p.abortErr
}

func TestRestoreCoordinatorForwardsExactResultsErrorsAndCandidates(t *testing.T) {
	freshErr := errors.New("fresh")
	prepareErr := errors.New("prepare")
	windowsErr := errors.New("windows")
	commitErr := errors.New("commit")
	abortErr := errors.New("abort")
	prepared := &RestoreCandidate{}
	accepted := &RestoreCandidate{}
	port := &fakeRestoreCoordinatorPort{
		trace:        make([]string, 0, 5),
		snapshot:     FreshSessionSnapshot{ActiveWorkspace: 4, Workspaces: []FreshWorkspace{{Name: "detached"}}},
		prepared:     prepared,
		windowIDs:    []WindowID{11, 12},
		events:       []Event{{Kind: PaneStarted, Pane: 13}, {Kind: WindowActivated, Window: 12}},
		freshErr:     freshErr,
		prepareErr:   prepareErr,
		windowIDsErr: windowsErr,
		commitErr:    commitErr,
		abortErr:     abortErr,
	}
	controller := newFakeRestoreCoordinator()

	snapshot, gotFreshErr := controller.freshSessionSnapshot(port)
	candidate, gotPrepareErr := controller.prepareRestore(port)
	windowIDs, gotWindowsErr := controller.restoreWindowIDs(accepted, port)
	events, gotCommitErr := controller.commitRestore(accepted, port)
	gotAbortErr := controller.abortRestore(accepted, port)

	if want := []string{"fresh", "prepare", "windows", "commit", "abort"}; !reflect.DeepEqual(port.trace, want) {
		t.Fatalf("trace=%v want=%v", port.trace, want)
	}
	if !reflect.DeepEqual(snapshot, port.snapshot) || !errors.Is(gotFreshErr, freshErr) {
		t.Fatalf("fresh snapshot=%#v err=%v", snapshot, gotFreshErr)
	}
	if candidate != prepared || !errors.Is(gotPrepareErr, prepareErr) {
		t.Fatalf("prepare candidate=%p want=%p err=%v", candidate, prepared, gotPrepareErr)
	}
	if len(windowIDs) != 2 || &windowIDs[0] != &port.windowIDs[0] || !errors.Is(gotWindowsErr, windowsErr) {
		t.Fatalf("window IDs=%v err=%v", windowIDs, gotWindowsErr)
	}
	if len(events) != 2 || &events[0] != &port.events[0] || !errors.Is(gotCommitErr, commitErr) {
		t.Fatalf("events=%#v err=%v", events, gotCommitErr)
	}
	if !errors.Is(gotAbortErr, abortErr) {
		t.Fatalf("abort err=%v", gotAbortErr)
	}
	if port.windowIDCandidate != accepted || port.commitCandidate != accepted || port.abortCandidate != accepted {
		t.Fatalf("accepted candidates windows=%p commit=%p abort=%p want=%p", port.windowIDCandidate, port.commitCandidate, port.abortCandidate, accepted)
	}
}

func TestRestoreCoordinatorZeroAndEagerValuesMatch(t *testing.T) {
	controllers := []struct {
		name       string
		controller fakeRestoreCoordinator
	}{
		{name: "eager", controller: newFakeRestoreCoordinator()},
		{name: "zero", controller: fakeRestoreCoordinator{}},
	}
	for _, test := range controllers {
		t.Run(test.name, func(t *testing.T) {
			port := &fakeRestoreCoordinatorPort{trace: make([]string, 0, 5)}
			if snapshot, err := test.controller.freshSessionSnapshot(port); !reflect.DeepEqual(snapshot, FreshSessionSnapshot{}) || err != nil {
				t.Fatalf("zero fresh=%#v err=%v", snapshot, err)
			}
			if candidate, err := test.controller.prepareRestore(port); candidate != nil || err != nil {
				t.Fatalf("zero prepare=%p err=%v", candidate, err)
			}
			if ids, err := test.controller.restoreWindowIDs(nil, port); ids != nil || err != nil {
				t.Fatalf("nil windows=%v err=%v", ids, err)
			}
			if events, err := test.controller.commitRestore(nil, port); events != nil || err != nil {
				t.Fatalf("nil commit=%#v err=%v", events, err)
			}
			if err := test.controller.abortRestore(nil, port); err != nil {
				t.Fatalf("nil abort=%v", err)
			}
			if want := []string{"fresh", "prepare", "windows", "commit", "abort"}; !reflect.DeepEqual(port.trace, want) {
				t.Fatalf("zero/eager trace=%v want=%v", port.trace, want)
			}
			if port.windowIDCandidate != nil || port.commitCandidate != nil || port.abortCandidate != nil {
				t.Fatalf("nil candidate changed windows=%p commit=%p abort=%p", port.windowIDCandidate, port.commitCandidate, port.abortCandidate)
			}
		})
	}
}

var (
	restoreCoordinatorSnapshot  FreshSessionSnapshot
	restoreCoordinatorCandidate *RestoreCandidate
	restoreCoordinatorWindowIDs []WindowID
	restoreCoordinatorEvents    []Event
	restoreCoordinatorError     error
)

func TestRestoreCoordinatorSimpleAndInvalidForwardingDoNotAllocate(t *testing.T) {
	controller := newFakeRestoreCoordinator()
	candidate := &RestoreCandidate{}
	port := &fakeRestoreCoordinatorPort{
		trace:     make([]string, 0, 1),
		snapshot:  FreshSessionSnapshot{ActiveWorkspace: 2},
		prepared:  candidate,
		windowIDs: []WindowID{3},
		events:    []Event{{Kind: PaneStarted, Pane: 4}},
	}
	tests := []struct {
		name string
		run  func()
	}{
		{name: "fresh", run: func() {
			port.trace = port.trace[:0]
			restoreCoordinatorSnapshot, restoreCoordinatorError = controller.freshSessionSnapshot(port)
		}},
		{name: "prepare", run: func() {
			port.trace = port.trace[:0]
			restoreCoordinatorCandidate, restoreCoordinatorError = controller.prepareRestore(port)
		}},
		{name: "invalid windows", run: func() {
			port.trace = port.trace[:0]
			restoreCoordinatorWindowIDs, restoreCoordinatorError = controller.restoreWindowIDs(nil, port)
		}},
		{name: "invalid commit", run: func() {
			port.trace = port.trace[:0]
			restoreCoordinatorEvents, restoreCoordinatorError = controller.commitRestore(nil, port)
		}},
		{name: "invalid abort", run: func() {
			port.trace = port.trace[:0]
			restoreCoordinatorError = controller.abortRestore(nil, port)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if allocs := testing.AllocsPerRun(1000, test.run); allocs != 0 {
				t.Fatalf("allocations=%v want=0", allocs)
			}
			if len(port.trace) != 1 {
				t.Fatalf("trace=%v want one eager call", port.trace)
			}
		})
	}
}

func TestRestoreCoordinatorPortsFieldsAndSignaturesAreExact(t *testing.T) {
	controller := reflect.TypeOf(fakeRestoreCoordinator{})
	if controller.NumField() != 0 || controller.Size() != 0 {
		t.Fatalf("controller fields=%d size=%d want zero-field zero-size", controller.NumField(), controller.Size())
	}
	preparation := reflect.TypeOf((*restorePreparationPort)(nil)).Elem()
	publication := reflect.TypeOf((*restorePublicationPort)(nil)).Elem()
	if preparation.NumMethod() != 2 || publication.NumMethod() != 3 {
		t.Fatalf("port methods preparation=%d publication=%d want=2/3", preparation.NumMethod(), publication.NumMethod())
	}
	if preparation.NumMethod() > 3 || publication.NumMethod() > 3 {
		t.Fatalf("largest port exceeds 3: preparation=%d publication=%d", preparation.NumMethod(), publication.NumMethod())
	}
	if restoreCoordinatorPortBudget != 5 {
		t.Fatalf("port budget=%d want=5", restoreCoordinatorPortBudget)
	}
	if got := preparation.NumMethod() + publication.NumMethod(); got != restoreCoordinatorPortBudget {
		t.Fatalf("aggregate methods=%d budget=%d", got, restoreCoordinatorPortBudget)
	}

	errorType := reflect.TypeOf((*error)(nil)).Elem()
	candidateType := reflect.TypeOf((*RestoreCandidate)(nil))
	assertRestoreCoordinatorPortMethod(t, preparation, "freshSessionSnapshot", nil, []reflect.Type{reflect.TypeOf(FreshSessionSnapshot{}), errorType})
	assertRestoreCoordinatorPortMethod(t, preparation, "prepareRestore", nil, []reflect.Type{candidateType, errorType})
	assertRestoreCoordinatorPortMethod(t, publication, "restoreWindowIDs", []reflect.Type{candidateType}, []reflect.Type{reflect.TypeOf([]WindowID(nil)), errorType})
	assertRestoreCoordinatorPortMethod(t, publication, "commitRestore", []reflect.Type{candidateType}, []reflect.Type{reflect.TypeOf([]Event(nil)), errorType})
	assertRestoreCoordinatorPortMethod(t, publication, "abortRestore", []reflect.Type{candidateType}, []reflect.Type{errorType})
}

func assertRestoreCoordinatorPortMethod(t *testing.T, port reflect.Type, name string, inputs, outputs []reflect.Type) {
	t.Helper()
	method, ok := port.MethodByName(name)
	if !ok || method.Type.NumIn() != len(inputs) || method.Type.NumOut() != len(outputs) {
		t.Fatalf("%s.%s signature=%v", port.Name(), name, method.Type)
	}
	for index, want := range inputs {
		got := method.Type.In(index)
		if got != want {
			t.Fatalf("%s.%s input[%d]=%s want=%s", port.Name(), name, index, got, want)
		}
		assertRestoreCoordinatorBoundaryType(t, port.Name()+"."+name, got)
	}
	for index, want := range outputs {
		got := method.Type.Out(index)
		if got != want {
			t.Fatalf("%s.%s output[%d]=%s want=%s", port.Name(), name, index, got, want)
		}
		assertRestoreCoordinatorBoundaryType(t, port.Name()+"."+name, got)
	}
}

func assertRestoreCoordinatorBoundaryType(t *testing.T, name string, typ reflect.Type) {
	t.Helper()
	for _, forbidden := range []reflect.Type{
		reflect.TypeOf((*Mux)(nil)),
		reflect.TypeOf((*Model)(nil)),
		reflect.TypeOf((*localSessionRegistry)(nil)),
		reflect.TypeOf((*pane)(nil)),
	} {
		if typ == forbidden {
			t.Errorf("%s exposes forbidden owner type %s", name, typ)
		}
	}
	switch typ.Kind() {
	case reflect.Map, reflect.Func, reflect.Chan:
		t.Errorf("%s uses forbidden boundary bag %s", name, typ)
	case reflect.Interface:
		if typ.NumMethod() == 0 && typ != reflect.TypeOf((*error)(nil)).Elem() {
			t.Errorf("%s uses any/empty-interface bag %s", name, typ)
		}
	case reflect.Slice, reflect.Array:
		assertRestoreCoordinatorBoundaryType(t, name, typ.Elem())
	case reflect.Pointer:
		if typ != reflect.TypeOf((*RestoreCandidate)(nil)) {
			assertRestoreCoordinatorBoundaryType(t, name, typ.Elem())
		}
	}
}

func TestRestoreCoordinatorSourceIsExactPrivateAndUnwired(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	dir := filepath.Dir(testFile)
	productionPath := filepath.Join(dir, "restore_coordinator.go")
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, productionPath, nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Imports) != 0 {
		t.Fatalf("controller imports=%d want=0", len(file.Imports))
	}
	var comments strings.Builder
	for _, group := range file.Comments {
		comments.WriteString(group.Text())
	}
	if count := strings.Count(comments.String(), "TODO(L3-01; expires Slice 6.2d)"); count != 1 {
		t.Fatalf("controller expiry TODO count=%d want=1", count)
	}

	declarations := make(map[string]int)
	methods := make(map[string]*ast.FuncDecl)
	var constructor *ast.FuncDecl
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					assertRestoreCoordinatorPrivateName(t, spec.Name)
					declarations[spec.Name.Name]++
					assertRestoreCoordinatorTypeExpression(t, spec.Type)
					if spec.Name.Name == "restoreCoordinator" {
						got := renderRestoreCoordinatorNamedFields(fileSet, spec.TypeParams)
						want := []string{"preparationPort restorePreparationPort", "publicationPort restorePublicationPort"}
						if !reflect.DeepEqual(got, want) {
							t.Errorf("controller generic parameters=%v want=%v", got, want)
						}
					}
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						assertRestoreCoordinatorPrivateName(t, name)
						declarations[name.Name]++
					}
				}
			}
		case *ast.FuncDecl:
			assertRestoreCoordinatorPrivateName(t, declaration.Name)
			if declaration.Recv == nil {
				declarations[declaration.Name.Name]++
				constructor = declaration
			} else {
				declarations["method:"+declaration.Name.Name]++
				methods[declaration.Name.Name] = declaration
			}
			assertRestoreCoordinatorFieldList(t, declaration.Recv)
			assertRestoreCoordinatorFieldList(t, declaration.Type.TypeParams)
			assertRestoreCoordinatorFieldList(t, declaration.Type.Params)
			assertRestoreCoordinatorFieldList(t, declaration.Type.Results)
		}
	}
	wantDeclarations := map[string]int{
		"restoreCoordinatorPortBudget": 1,
		"restorePreparationPort":       1,
		"restorePublicationPort":       1,
		"restoreCoordinator":           1,
		"newRestoreCoordinator":        1,
		"method:freshSessionSnapshot":  1,
		"method:prepareRestore":        1,
		"method:restoreWindowIDs":      1,
		"method:commitRestore":         1,
		"method:abortRestore":          1,
	}
	if !reflect.DeepEqual(declarations, wantDeclarations) {
		t.Fatalf("controller declarations=%v want=%v", declarations, wantDeclarations)
	}
	assertRestoreCoordinatorConstructor(t, fileSet, constructor)

	methodContracts := map[string]struct {
		signature string
		port      string
		arguments []string
	}{
		"freshSessionSnapshot": {signature: "func(port preparationPort) (FreshSessionSnapshot, error)", port: "port"},
		"prepareRestore":       {signature: "func(port preparationPort) (*RestoreCandidate, error)", port: "port"},
		"restoreWindowIDs":     {signature: "func(candidate *RestoreCandidate, port publicationPort) ([]WindowID, error)", port: "port", arguments: []string{"candidate"}},
		"commitRestore":        {signature: "func(candidate *RestoreCandidate, port publicationPort) ([]Event, error)", port: "port", arguments: []string{"candidate"}},
		"abortRestore":         {signature: "func(candidate *RestoreCandidate, port publicationPort) error", port: "port", arguments: []string{"candidate"}},
	}
	for name, contract := range methodContracts {
		assertRestoreCoordinatorMethod(t, fileSet, methods[name], name, contract.signature, contract.port, contract.arguments)
	}

	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	seamNames := map[string]bool{
		"restoreCoordinatorPortBudget": true,
		"restorePreparationPort":       true,
		"restorePublicationPort":       true,
		"restoreCoordinator":           true,
		"newRestoreCoordinator":        true,
	}
	for _, path := range paths {
		if path == productionPath || strings.HasSuffix(path, "_test.go") {
			continue
		}
		production, parseErr := parser.ParseFile(fileSet, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		ast.Inspect(production, func(node ast.Node) bool {
			identifier, found := node.(*ast.Ident)
			if found && seamNames[identifier.Name] {
				t.Errorf("existing production path %s references unwired seam %s", filepath.Base(path), identifier.Name)
			}
			return true
		})
	}
}

func assertRestoreCoordinatorConstructor(t *testing.T, fileSet *token.FileSet, declaration *ast.FuncDecl) {
	t.Helper()
	if declaration == nil || declaration.Name.Name != "newRestoreCoordinator" {
		t.Fatal("missing exact constructor")
	}
	if got := renderRestoreCoordinatorNamedFields(fileSet, declaration.Type.TypeParams); !reflect.DeepEqual(got, []string{"preparationPort restorePreparationPort", "publicationPort restorePublicationPort"}) {
		t.Fatalf("constructor generic parameters=%v", got)
	}
	if declaration.Type.Params.NumFields() != 0 || renderRestoreCoordinatorNode(fileSet, declaration.Type.Results.List[0].Type) != "restoreCoordinator[preparationPort, publicationPort]" {
		t.Fatalf("constructor signature=%s", renderRestoreCoordinatorNode(fileSet, declaration.Type))
	}
	if len(declaration.Body.List) != 1 {
		t.Fatalf("constructor statements=%d want=1", len(declaration.Body.List))
	}
	result, ok := declaration.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(result.Results) != 1 || renderRestoreCoordinatorNode(fileSet, result.Results[0]) != "restoreCoordinator[preparationPort, publicationPort]{}" {
		t.Fatalf("constructor must return exact zero value: %s", renderRestoreCoordinatorNode(fileSet, declaration.Body))
	}
}

func assertRestoreCoordinatorMethod(t *testing.T, fileSet *token.FileSet, declaration *ast.FuncDecl, name, signature, port string, arguments []string) {
	t.Helper()
	if declaration == nil {
		t.Fatalf("missing controller method %s", name)
	}
	if got := strings.Join(renderRestoreCoordinatorFieldTypes(fileSet, declaration.Recv), "|"); got != "restoreCoordinator[preparationPort, publicationPort]" {
		t.Errorf("%s receiver=%q", name, got)
	}
	if got := renderRestoreCoordinatorNode(fileSet, declaration.Type); got != signature {
		t.Errorf("%s signature=%q want=%q", name, got, signature)
	}
	if len(declaration.Body.List) != 1 {
		t.Fatalf("%s statements=%d want exact one-call body", name, len(declaration.Body.List))
	}
	result, ok := declaration.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(result.Results) != 1 || !restoreCoordinatorPortCall(result.Results[0], port, name, arguments...) {
		t.Errorf("%s must directly return %s.%s(%s)", name, port, name, strings.Join(arguments, ", "))
	}
}

func restoreCoordinatorPortCall(expression ast.Expr, receiver, method string, arguments ...string) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != len(arguments) {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !restoreCoordinatorIdent(selector.X, receiver) || selector.Sel.Name != method {
		return false
	}
	for index, argument := range arguments {
		if !restoreCoordinatorIdent(call.Args[index], argument) {
			return false
		}
	}
	return true
}

func restoreCoordinatorIdent(expression ast.Expr, name string) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Name == name
}

func assertRestoreCoordinatorPrivateName(t *testing.T, identifier *ast.Ident) {
	t.Helper()
	if ast.IsExported(identifier.Name) {
		t.Errorf("controller seam exports %s", identifier.Name)
	}
}

func assertRestoreCoordinatorFieldList(t *testing.T, fields *ast.FieldList) {
	t.Helper()
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		for _, name := range field.Names {
			assertRestoreCoordinatorPrivateName(t, name)
		}
		assertRestoreCoordinatorTypeExpression(t, field.Type)
	}
}

func assertRestoreCoordinatorTypeExpression(t *testing.T, expression ast.Expr) {
	t.Helper()
	allowed := map[string]bool{
		"FreshSessionSnapshot":   true,
		"RestoreCandidate":       true,
		"WindowID":               true,
		"Event":                  true,
		"error":                  true,
		"restorePreparationPort": true,
		"restorePublicationPort": true,
		"restoreCoordinator":     true,
		"preparationPort":        true,
		"publicationPort":        true,
	}
	ast.Inspect(expression, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.MapType:
			t.Errorf("controller seam uses map bag at %v", node.Pos())
		case *ast.ChanType:
			t.Errorf("controller seam uses channel bag at %v", node.Pos())
		case *ast.FuncType:
			assertRestoreCoordinatorFieldList(t, node.TypeParams)
			assertRestoreCoordinatorFieldList(t, node.Params)
			assertRestoreCoordinatorFieldList(t, node.Results)
			return false
		case *ast.InterfaceType:
			if node.Methods == nil || len(node.Methods.List) == 0 {
				t.Errorf("controller seam uses any/empty-interface bag at %v", node.Pos())
				return false
			}
			for _, method := range node.Methods.List {
				for _, name := range method.Names {
					assertRestoreCoordinatorPrivateName(t, name)
				}
				function, ok := method.Type.(*ast.FuncType)
				if !ok {
					t.Errorf("controller port embeds non-method type at %v", method.Pos())
					continue
				}
				assertRestoreCoordinatorFieldList(t, function.Params)
				assertRestoreCoordinatorFieldList(t, function.Results)
			}
			return false
		case *ast.Ident:
			if allowed[node.Name] {
				return true
			}
			lower := strings.ToLower(node.Name)
			for _, forbidden := range []string{"mux", "model", "registry", "pane", "session", "store", "protocol", "any", "map", "func", "channel"} {
				if lower == forbidden || strings.Contains(lower, forbidden) {
					t.Errorf("controller seam signature exposes forbidden identifier %s", node.Name)
				}
			}
		}
		return true
	})
}

func renderRestoreCoordinatorNode(fileSet *token.FileSet, node any) string {
	var rendered bytes.Buffer
	if err := format.Node(&rendered, fileSet, node); err != nil {
		return "<format-error>"
	}
	return rendered.String()
}

func renderRestoreCoordinatorFieldTypes(fileSet *token.FileSet, fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}
	var rendered []string
	for _, field := range fields.List {
		rendered = append(rendered, renderRestoreCoordinatorNode(fileSet, field.Type))
	}
	return rendered
}

func renderRestoreCoordinatorNamedFields(fileSet *token.FileSet, fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}
	var rendered []string
	for _, field := range fields.List {
		for _, name := range field.Names {
			rendered = append(rendered, name.Name+" "+renderRestoreCoordinatorNode(fileSet, field.Type))
		}
	}
	return rendered
}
