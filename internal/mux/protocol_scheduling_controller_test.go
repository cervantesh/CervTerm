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

type fakeProtocolSchedulingPort struct {
	trace         []string
	kittyEvent    Event
	expiryEvent   Event
	completeEvent Event
	kittyInput    Event
	expiryInput   Event
	completeInput Event
}

type fakeProtocolSchedulingController = protocolSchedulingController[*fakeProtocolSchedulingPort, *fakeProtocolSchedulingPort]

func newFakeProtocolSchedulingController() fakeProtocolSchedulingController {
	return newProtocolSchedulingController[*fakeProtocolSchedulingPort, *fakeProtocolSchedulingPort]()
}

func (p *fakeProtocolSchedulingPort) dispatchKitty(events []Event) []Event {
	p.trace = append(p.trace, "kitty")
	p.kittyInput = events[0]
	return append(events, p.kittyEvent)
}

func (p *fakeProtocolSchedulingPort) dispatchSixel() {
	p.trace = append(p.trace, "sixel")
}

func (p *fakeProtocolSchedulingPort) dispatchITerm() {
	p.trace = append(p.trace, "iterm")
}

func (p *fakeProtocolSchedulingPort) applyExpiry(events []Event) []Event {
	p.trace = append(p.trace, "expiry")
	p.expiryInput = events[0]
	return append(events, p.expiryEvent)
}

func (p *fakeProtocolSchedulingPort) applyCompletion(events []Event) []Event {
	p.trace = append(p.trace, "completion")
	p.completeInput = events[0]
	return append(events, p.completeEvent)
}

func protocolSchedulingPrefix() Event {
	return Event{
		Kind: PaneTransferred, Window: 11, SourceWindow: 12, Pane: 13,
		Workspace: 14, SourceWorkspace: 15, Tab: 16, SourceTab: 17,
		Data: []byte("prefix"), BytesRead: 6, Text: "preserve", Fresh: true,
		Geometry: PaneGeometry{Rows: 18, Cols: 19}, Err: errors.New("prefix"), Revision: 20,
	}
}

func TestProtocolSchedulingControllerDispatchesExactEventsInOrder(t *testing.T) {
	controller := newFakeProtocolSchedulingController()
	prefix := protocolSchedulingPrefix()
	kitty := Event{Kind: PaneDirty, Pane: 21, SourceWindow: 22, SourceWorkspace: 23, SourceTab: 24, Revision: 25}
	port := &fakeProtocolSchedulingPort{trace: make([]string, 0, 3), kittyEvent: kitty}
	events := make([]Event, 1, 2)
	events[0] = prefix

	got := controller.dispatch(events, port)
	if want := []string{"kitty", "sixel", "iterm"}; !reflect.DeepEqual(port.trace, want) {
		t.Fatalf("dispatch trace=%v want=%v", port.trace, want)
	}
	if len(got) != 2 || !reflect.DeepEqual(got[0], prefix) || !reflect.DeepEqual(got[1], kitty) {
		t.Fatalf("dispatch events=%#v want prefix=%#v kitty=%#v", got, prefix, kitty)
	}
	if !reflect.DeepEqual(port.kittyInput, prefix) {
		t.Fatalf("Kitty input=%#v want exact prefix=%#v", port.kittyInput, prefix)
	}
	if &got[0] != &events[0] {
		t.Fatal("dispatch replaced the caller event backing store")
	}
}

func TestProtocolSchedulingControllerAppliesExactEvents(t *testing.T) {
	prefix := protocolSchedulingPrefix()
	tests := []struct {
		name      string
		wantTrace string
		wantEvent Event
		apply     func(fakeProtocolSchedulingController, []Event, *fakeProtocolSchedulingPort) []Event
		input     func(*fakeProtocolSchedulingPort) Event
	}{
		{
			name: "expiry", wantTrace: "expiry",
			wantEvent: Event{Kind: PaneWriteFailed, Pane: 31, SourceWindow: 32, SourceWorkspace: 33, SourceTab: 34, Revision: 35},
			apply: func(controller fakeProtocolSchedulingController, events []Event, port *fakeProtocolSchedulingPort) []Event {
				return controller.applyExpiry(events, port)
			},
			input: func(port *fakeProtocolSchedulingPort) Event { return port.expiryInput },
		},
		{
			name: "completion", wantTrace: "completion",
			wantEvent: Event{Kind: PaneResizeFailed, Pane: 41, SourceWindow: 42, SourceWorkspace: 43, SourceTab: 44, Revision: 45},
			apply: func(controller fakeProtocolSchedulingController, events []Event, port *fakeProtocolSchedulingPort) []Event {
				return controller.applyCompletion(events, port)
			},
			input: func(port *fakeProtocolSchedulingPort) Event { return port.completeInput },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			controller := newFakeProtocolSchedulingController()
			port := &fakeProtocolSchedulingPort{trace: make([]string, 0, 1), expiryEvent: test.wantEvent, completeEvent: test.wantEvent}
			events := make([]Event, 1, 2)
			events[0] = prefix
			got := test.apply(controller, events, port)
			if want := []string{test.wantTrace}; !reflect.DeepEqual(port.trace, want) {
				t.Fatalf("apply trace=%v want=%v", port.trace, want)
			}
			if len(got) != 2 || !reflect.DeepEqual(got[0], prefix) || !reflect.DeepEqual(got[1], test.wantEvent) {
				t.Fatalf("apply events=%#v want prefix=%#v event=%#v", got, prefix, test.wantEvent)
			}
			if input := test.input(port); !reflect.DeepEqual(input, prefix) {
				t.Fatalf("apply input=%#v want exact prefix=%#v", input, prefix)
			}
			if &got[0] != &events[0] {
				t.Fatal("apply replaced the caller event backing store")
			}
		})
	}
}

func TestProtocolSchedulingControllerEagerAndZeroValuesMatch(t *testing.T) {
	controllers := []struct {
		name       string
		controller fakeProtocolSchedulingController
	}{
		{name: "eager", controller: newFakeProtocolSchedulingController()},
		{name: "zero", controller: fakeProtocolSchedulingController{}},
	}
	for _, test := range controllers {
		t.Run(test.name, func(t *testing.T) {
			prefix := protocolSchedulingPrefix()
			port := &fakeProtocolSchedulingPort{
				trace:         make([]string, 0, 5),
				kittyEvent:    Event{Kind: PaneDirty, Pane: 51},
				expiryEvent:   Event{Kind: PaneWriteFailed, Pane: 52},
				completeEvent: Event{Kind: PaneResizeFailed, Pane: 53},
			}
			dispatchEvents := make([]Event, 1, 2)
			dispatchEvents[0] = prefix
			expiryEvents := make([]Event, 1, 2)
			expiryEvents[0] = prefix
			completionEvents := make([]Event, 1, 2)
			completionEvents[0] = prefix

			dispatched := test.controller.dispatch(dispatchEvents, port)
			expired := test.controller.applyExpiry(expiryEvents, port)
			completed := test.controller.applyCompletion(completionEvents, port)
			if want := []string{"kitty", "sixel", "iterm", "expiry", "completion"}; !reflect.DeepEqual(port.trace, want) {
				t.Fatalf("trace=%v want=%v", port.trace, want)
			}
			if !reflect.DeepEqual(dispatched[1], port.kittyEvent) || !reflect.DeepEqual(expired[1], port.expiryEvent) || !reflect.DeepEqual(completed[1], port.completeEvent) {
				t.Fatalf("delegated events dispatch=%#v expiry=%#v completion=%#v", dispatched, expired, completed)
			}
		})
	}
}

var protocolSchedulingControllerEvents []Event

func TestProtocolSchedulingControllerOperationsDoNotAllocate(t *testing.T) {
	controller := newFakeProtocolSchedulingController()
	prefix := Event{Kind: PaneStarted, Pane: 61}
	port := &fakeProtocolSchedulingPort{
		trace:         make([]string, 0, 3),
		kittyEvent:    Event{Kind: PaneDirty, Pane: 62},
		expiryEvent:   Event{Kind: PaneWriteFailed, Pane: 63},
		completeEvent: Event{Kind: PaneResizeFailed, Pane: 64},
	}
	events := make([]Event, 1, 2)
	events[0] = prefix

	tests := []struct {
		name string
		run  func()
	}{
		{name: "dispatch", run: func() {
			port.trace = port.trace[:0]
			protocolSchedulingControllerEvents = controller.dispatch(events[:1], port)
		}},
		{name: "expiry", run: func() {
			port.trace = port.trace[:0]
			protocolSchedulingControllerEvents = controller.applyExpiry(events[:1], port)
		}},
		{name: "completion", run: func() {
			port.trace = port.trace[:0]
			protocolSchedulingControllerEvents = controller.applyCompletion(events[:1], port)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(1000, test.run)
			if allocs != 0 {
				t.Fatalf("%s allocations=%v want=0", test.name, allocs)
			}
			if len(protocolSchedulingControllerEvents) != 2 || &protocolSchedulingControllerEvents[0] != &events[0] {
				t.Fatalf("%s result=%#v", test.name, protocolSchedulingControllerEvents)
			}
		})
	}
}

func TestProtocolSchedulingControllerPortsAndFieldsAreExact(t *testing.T) {
	controller := reflect.TypeOf(fakeProtocolSchedulingController{})
	if controller.NumField() != 0 || controller.Size() != 0 {
		t.Fatalf("controller fields=%d size=%d want zero-field zero-size", controller.NumField(), controller.Size())
	}
	dispatch := reflect.TypeOf((*protocolSchedulingDispatchPort)(nil)).Elem()
	apply := reflect.TypeOf((*protocolSchedulingApplyPort)(nil)).Elem()
	for name, port := range map[string]reflect.Type{"dispatch": dispatch, "apply": apply} {
		if port.NumMethod() > 3 {
			t.Fatalf("%s methods=%d want <=3", name, port.NumMethod())
		}
	}
	if dispatch.NumMethod() != 3 {
		t.Fatalf("dispatch methods=%d want exactly 3", dispatch.NumMethod())
	}
	if apply.NumMethod() != 2 {
		t.Fatalf("apply methods=%d want exactly 2", apply.NumMethod())
	}
	if protocolSchedulingControllerPortBudget != 5 {
		t.Fatalf("port budget=%d want exactly 5", protocolSchedulingControllerPortBudget)
	}
	if got := dispatch.NumMethod() + apply.NumMethod(); got != protocolSchedulingControllerPortBudget {
		t.Fatalf("aggregate port methods=%d budget=%d", got, protocolSchedulingControllerPortBudget)
	}

	eventsType := reflect.TypeOf([]Event(nil))
	assertProtocolSchedulingMethod(t, dispatch, "dispatchKitty", []reflect.Type{eventsType}, []reflect.Type{eventsType})
	assertProtocolSchedulingMethod(t, dispatch, "dispatchSixel", nil, nil)
	assertProtocolSchedulingMethod(t, dispatch, "dispatchITerm", nil, nil)
	assertProtocolSchedulingMethod(t, apply, "applyExpiry", []reflect.Type{eventsType}, []reflect.Type{eventsType})
	assertProtocolSchedulingMethod(t, apply, "applyCompletion", []reflect.Type{eventsType}, []reflect.Type{eventsType})
}

func assertProtocolSchedulingMethod(t *testing.T, port reflect.Type, name string, inputs, outputs []reflect.Type) {
	t.Helper()
	method, ok := port.MethodByName(name)
	if !ok {
		t.Fatalf("%s missing method %s", port.Name(), name)
	}
	if method.Type.NumIn() != len(inputs) || method.Type.NumOut() != len(outputs) {
		t.Fatalf("%s.%s signature=%s", port.Name(), name, method.Type)
	}
	for index, want := range inputs {
		got := method.Type.In(index)
		if got != want {
			t.Fatalf("%s.%s input[%d]=%s want=%s", port.Name(), name, index, got, want)
		}
		assertProtocolSchedulingBoundaryType(t, port.Name()+"."+name, got)
	}
	for index, want := range outputs {
		got := method.Type.Out(index)
		if got != want {
			t.Fatalf("%s.%s output[%d]=%s want=%s", port.Name(), name, index, got, want)
		}
		assertProtocolSchedulingBoundaryType(t, port.Name()+"."+name, got)
	}
}

func assertProtocolSchedulingBoundaryType(t *testing.T, name string, typ reflect.Type) {
	t.Helper()
	switch typ.Kind() {
	case reflect.Map, reflect.Func, reflect.Chan:
		t.Errorf("%s uses forbidden boundary bag %s", name, typ)
	case reflect.Interface:
		if typ.NumMethod() == 0 {
			t.Errorf("%s uses any/empty-interface bag %s", name, typ)
		}
	case reflect.Slice, reflect.Pointer, reflect.Array:
		assertProtocolSchedulingBoundaryType(t, name, typ.Elem())
	}
	for _, forbidden := range []reflect.Type{
		reflect.TypeOf((*Mux)(nil)),
		reflect.TypeOf((*pane)(nil)),
		reflect.TypeOf((*imageDecodeScheduler)(nil)),
		reflect.TypeOf(replySlot{}),
	} {
		if typ == forbidden {
			t.Errorf("%s exposes forbidden type %s", name, typ)
		}
	}
}

func TestProtocolSchedulingControllerSourceIsExactPrivateAndUnwired(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	dir := filepath.Dir(testFile)
	productionPath := filepath.Join(dir, "protocol_scheduling_controller.go")
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
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					assertProtocolSchedulingPrivateName(t, spec.Name)
					declarations[spec.Name.Name]++
					assertProtocolSchedulingTypeExpression(t, spec.Type)
					if spec.Name.Name == "protocolSchedulingController" {
						got := renderProtocolSchedulingNamedFields(fileSet, spec.TypeParams)
						want := []string{"dispatchPort protocolSchedulingDispatchPort", "applyPort protocolSchedulingApplyPort"}
						if !reflect.DeepEqual(got, want) {
							t.Errorf("controller generic parameters=%v want=%v", got, want)
						}
					}
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						assertProtocolSchedulingPrivateName(t, name)
						declarations[name.Name]++
					}
				}
			}
		case *ast.FuncDecl:
			assertProtocolSchedulingPrivateName(t, declaration.Name)
			if declaration.Recv == nil {
				declarations[declaration.Name.Name]++
			} else {
				declarations["method:"+declaration.Name.Name]++
				methods[declaration.Name.Name] = declaration
			}
			assertProtocolSchedulingFieldList(t, declaration.Recv)
			assertProtocolSchedulingFieldList(t, declaration.Type.Params)
			assertProtocolSchedulingFieldList(t, declaration.Type.Results)
		}
	}
	wantDeclarations := map[string]int{
		"protocolSchedulingControllerPortBudget": 1,
		"protocolSchedulingDispatchPort":         1,
		"protocolSchedulingApplyPort":            1,
		"protocolSchedulingController":           1,
		"newProtocolSchedulingController":        1,
		"method:dispatch":                        1,
		"method:applyExpiry":                     1,
		"method:applyCompletion":                 1,
	}
	if !reflect.DeepEqual(declarations, wantDeclarations) {
		t.Fatalf("controller declarations=%v want=%v", declarations, wantDeclarations)
	}

	assertProtocolSchedulingControllerMethod(t, fileSet, methods["dispatch"], "func(events []Event, port dispatchPort) []Event", []string{"dispatchKitty", "dispatchSixel", "dispatchITerm"})
	assertProtocolSchedulingControllerMethod(t, fileSet, methods["applyExpiry"], "func(events []Event, port applyPort) []Event", []string{"applyExpiry"})
	assertProtocolSchedulingControllerMethod(t, fileSet, methods["applyCompletion"], "func(events []Event, port applyPort) []Event", []string{"applyCompletion"})
	assertProtocolSchedulingExactBodies(t, methods)

	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	seamNames := map[string]bool{
		"protocolSchedulingControllerPortBudget": true,
		"protocolSchedulingDispatchPort":         true,
		"protocolSchedulingApplyPort":            true,
		"protocolSchedulingController":           true,
		"newProtocolSchedulingController":        true,
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
				t.Errorf("old production path %s references unwired seam %s", filepath.Base(path), identifier.Name)
			}
			return true
		})
	}
}

func assertProtocolSchedulingControllerMethod(t *testing.T, fileSet *token.FileSet, declaration *ast.FuncDecl, wantSignature string, wantCalls []string) {
	t.Helper()
	if declaration == nil {
		t.Fatalf("missing controller method for signature %s", wantSignature)
	}
	if got := strings.Join(renderProtocolSchedulingFieldTypes(fileSet, declaration.Recv), "|"); got != "protocolSchedulingController[dispatchPort, applyPort]" {
		t.Errorf("%s receiver=%q", declaration.Name.Name, got)
	}
	if got := renderProtocolSchedulingNode(fileSet, declaration.Type); got != wantSignature {
		t.Errorf("%s signature=%q want=%q", declaration.Name.Name, got, wantSignature)
	}
	var calls []string
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok {
			calls = append(calls, selector.Sel.Name)
		}
		return true
	})
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Errorf("%s calls=%v want exact order=%v", declaration.Name.Name, calls, wantCalls)
	}
}

func assertProtocolSchedulingExactBodies(t *testing.T, methods map[string]*ast.FuncDecl) {
	t.Helper()
	dispatch := methods["dispatch"].Body.List
	if len(dispatch) != 4 {
		t.Fatalf("dispatch statements=%d want=4", len(dispatch))
	}
	assignment, ok := dispatch[0].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || !protocolSchedulingIdent(assignment.Lhs[0], "events") || !protocolSchedulingPortCall(assignment.Rhs[0], "port", "dispatchKitty", "events") {
		t.Errorf("dispatch first statement must be events = port.dispatchKitty(events)")
	}
	if !protocolSchedulingCallStatement(dispatch[1], "port", "dispatchSixel") || !protocolSchedulingCallStatement(dispatch[2], "port", "dispatchITerm") {
		t.Errorf("dispatch void delegation must be Sixel then iTerm")
	}
	result, ok := dispatch[3].(*ast.ReturnStmt)
	if !ok || len(result.Results) != 1 || !protocolSchedulingIdent(result.Results[0], "events") {
		t.Errorf("dispatch must return the exact delegated events value")
	}
	for _, method := range []string{"applyExpiry", "applyCompletion"} {
		body := methods[method].Body.List
		result, ok := firstProtocolSchedulingReturn(body)
		if len(body) != 1 || !ok || len(result.Results) != 1 || !protocolSchedulingPortCall(result.Results[0], "port", method, "events") {
			t.Errorf("%s must directly return port.%s(events)", method, method)
		}
	}
}

func firstProtocolSchedulingReturn(statements []ast.Stmt) (*ast.ReturnStmt, bool) {
	if len(statements) == 0 {
		return nil, false
	}
	result, ok := statements[0].(*ast.ReturnStmt)
	return result, ok
}

func protocolSchedulingCallStatement(statement ast.Stmt, receiver, method string) bool {
	expression, ok := statement.(*ast.ExprStmt)
	return ok && protocolSchedulingPortCall(expression.X, receiver, method)
}

func protocolSchedulingPortCall(expression ast.Expr, receiver, method string, arguments ...string) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != len(arguments) {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !protocolSchedulingIdent(selector.X, receiver) || selector.Sel.Name != method {
		return false
	}
	for index, argument := range arguments {
		if !protocolSchedulingIdent(call.Args[index], argument) {
			return false
		}
	}
	return true
}

func protocolSchedulingIdent(expression ast.Expr, name string) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Name == name
}

func assertProtocolSchedulingPrivateName(t *testing.T, identifier *ast.Ident) {
	t.Helper()
	if ast.IsExported(identifier.Name) {
		t.Errorf("controller seam exports %s", identifier.Name)
	}
}

func assertProtocolSchedulingFieldList(t *testing.T, fields *ast.FieldList) {
	t.Helper()
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		for _, name := range field.Names {
			assertProtocolSchedulingPrivateName(t, name)
		}
		assertProtocolSchedulingTypeExpression(t, field.Type)
	}
}

func assertProtocolSchedulingTypeExpression(t *testing.T, expression ast.Expr) {
	t.Helper()
	ast.Inspect(expression, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.MapType:
			t.Errorf("controller seam uses map bag at %v", node.Pos())
		case *ast.ChanType:
			t.Errorf("controller seam uses channel bag at %v", node.Pos())
		case *ast.FuncType:
			assertProtocolSchedulingFieldList(t, node.Params)
			assertProtocolSchedulingFieldList(t, node.Results)
			return false
		case *ast.InterfaceType:
			if node.Methods == nil || len(node.Methods.List) == 0 {
				t.Errorf("controller seam uses any/empty-interface bag at %v", node.Pos())
				return false
			}
			for _, method := range node.Methods.List {
				for _, name := range method.Names {
					assertProtocolSchedulingPrivateName(t, name)
				}
				function, ok := method.Type.(*ast.FuncType)
				if !ok {
					t.Errorf("controller port embeds non-method type at %v", method.Pos())
					continue
				}
				assertProtocolSchedulingFieldList(t, function.Params)
				assertProtocolSchedulingFieldList(t, function.Results)
			}
			return false
		case *ast.Ident:
			lower := strings.ToLower(node.Name)
			for _, forbidden := range []string{"mux", "pane", "scheduler", "channel", "clock", "deadline", "store", "adapter", "owner", "replyslot", "any", "map", "func"} {
				if lower == forbidden || strings.Contains(lower, forbidden) {
					t.Errorf("controller seam signature exposes forbidden identifier %s", node.Name)
				}
			}
		}
		return true
	})
}

func renderProtocolSchedulingNode(fileSet *token.FileSet, node any) string {
	var rendered bytes.Buffer
	if err := format.Node(&rendered, fileSet, node); err != nil {
		return "<format-error>"
	}
	return rendered.String()
}

func renderProtocolSchedulingFieldTypes(fileSet *token.FileSet, fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}
	var rendered []string
	for _, field := range fields.List {
		rendered = append(rendered, renderProtocolSchedulingNode(fileSet, field.Type))
	}
	return rendered
}

func renderProtocolSchedulingNamedFields(fileSet *token.FileSet, fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}
	var rendered []string
	for _, field := range fields.List {
		for _, name := range field.Names {
			rendered = append(rendered, name.Name+" "+renderProtocolSchedulingNode(fileSet, field.Type))
		}
	}
	return rendered
}
