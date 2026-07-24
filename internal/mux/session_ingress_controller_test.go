package mux

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type fakeSessionIngressPorts struct {
	accepted bool
	trace    []string
}

func (p *fakeSessionIngressPorts) acceptSessionIngress() bool {
	p.trace = append(p.trace, "accept")
	return p.accepted
}

func (p *fakeSessionIngressPorts) applySessionIngressData(events []Event, data []byte) []Event {
	p.trace = append(p.trace, "data")
	return append(events, Event{Kind: PaneOutput, Data: data, BytesRead: len(data)})
}

func (p *fakeSessionIngressPorts) applySessionIngressEnd(events []Event, err error) []Event {
	p.trace = append(p.trace, "end")
	return append(events, Event{Kind: PaneExited, Err: err})
}

func TestSessionIngressControllerRoutesAcceptedPhasesInOrder(t *testing.T) {
	endErr := errors.New("ended")
	controller := newSessionIngressController()
	tests := []struct {
		name      string
		accepted  bool
		data      []byte
		end       error
		wantTrace []string
		wantKinds []EventKind
	}{
		{name: "reject", data: []byte("ignored"), end: endErr, wantTrace: []string{"accept"}, wantKinds: []EventKind{PaneStarted}},
		{name: "empty", accepted: true, wantTrace: []string{"accept"}, wantKinds: []EventKind{PaneStarted}},
		{name: "data", accepted: true, data: []byte("data"), wantTrace: []string{"accept", "data"}, wantKinds: []EventKind{PaneStarted, PaneOutput}},
		{name: "error", accepted: true, end: endErr, wantTrace: []string{"accept", "end"}, wantKinds: []EventKind{PaneStarted, PaneExited}},
		{name: "data+error", accepted: true, data: []byte("data"), end: endErr, wantTrace: []string{"accept", "data", "end"}, wantKinds: []EventKind{PaneStarted, PaneOutput, PaneExited}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ports := &fakeSessionIngressPorts{accepted: test.accepted, trace: make([]string, 0, 3)}
			caller := make([]Event, 1, 3)
			caller[0] = Event{Kind: PaneStarted}
			got := controller.route(caller, ports, ports, test.data, test.end)

			if !reflect.DeepEqual(ports.trace, test.wantTrace) {
				t.Fatalf("trace=%v want=%v", ports.trace, test.wantTrace)
			}
			if len(got) != len(test.wantKinds) {
				t.Fatalf("events=%#v want kinds=%v", got, test.wantKinds)
			}
			for i, kind := range test.wantKinds {
				if got[i].Kind != kind {
					t.Fatalf("event[%d].Kind=%v want=%v events=%#v", i, got[i].Kind, kind, got)
				}
			}
			if &got[0] != &caller[0] {
				t.Fatal("route replaced the caller event backing store")
			}
			if len(test.data) > 0 && test.accepted {
				dataEvent := got[1]
				if !reflect.DeepEqual(dataEvent.Data, test.data) || dataEvent.BytesRead != len(test.data) {
					t.Fatalf("data event=%#v", dataEvent)
				}
			}
			if test.end != nil && test.accepted {
				if !errors.Is(got[len(got)-1].Err, test.end) {
					t.Fatalf("end event=%#v want err=%v", got[len(got)-1], test.end)
				}
			}
		})
	}
}

func TestSessionIngressControllerAcceptedRouteDoesNotAllocate(t *testing.T) {
	controller := newSessionIngressController()
	ports := &fakeSessionIngressPorts{accepted: true, trace: make([]string, 0, 3)}
	caller := make([]Event, 1, 3)
	data := []byte("data")
	endErr := errors.New("ended")
	var got []Event

	allocs := testing.AllocsPerRun(1000, func() {
		ports.trace = ports.trace[:0]
		got = controller.route(caller[:1], ports, ports, data, endErr)
	})
	if allocs != 0 {
		t.Fatalf("accepted route allocations=%v want=0", allocs)
	}
	if len(got) != 3 || &got[0] != &caller[0] || !reflect.DeepEqual(ports.trace, []string{"accept", "data", "end"}) {
		t.Fatalf("accepted result=%#v trace=%v", got, ports.trace)
	}
}

func TestSessionIngressControllerRejectedRouteDoesNotAllocate(t *testing.T) {
	controller := newSessionIngressController()
	ports := &fakeSessionIngressPorts{trace: make([]string, 0, 1)}
	caller := make([]Event, 1, 3)
	data := []byte("ignored")
	endErr := errors.New("ignored")
	var got []Event

	allocs := testing.AllocsPerRun(1000, func() {
		ports.trace = ports.trace[:0]
		got = controller.route(caller[:1], ports, ports, data, endErr)
	})
	if allocs != 0 {
		t.Fatalf("rejected route allocations=%v want=0", allocs)
	}
	if len(got) != 1 || &got[0] != &caller[0] || !reflect.DeepEqual(ports.trace, []string{"accept"}) {
		t.Fatalf("rejected result=%#v trace=%v", got, ports.trace)
	}
}

func TestSessionIngressControllerPortsAndFieldsAreExact(t *testing.T) {
	controller := reflect.TypeOf(sessionIngressController{})
	if controller.NumField() != 0 || controller.Size() != 0 {
		t.Fatalf("controller fields=%d size=%d want zero-field zero-size", controller.NumField(), controller.Size())
	}

	owner := reflect.TypeOf((*sessionIngressOwnerPort)(nil)).Elem()
	apply := reflect.TypeOf((*sessionIngressApplyPort)(nil)).Elem()
	if owner.NumMethod() > 1 {
		t.Fatalf("owner acceptance methods=%d want<=1", owner.NumMethod())
	}
	if apply.NumMethod() > 2 {
		t.Fatalf("apply methods=%d want<=2", apply.NumMethod())
	}
	got := owner.NumMethod() + apply.NumMethod()
	if got != sessionIngressControllerPortBudget {
		t.Fatalf("aggregate port methods=%d budget=%d", got, sessionIngressControllerPortBudget)
	}
	if got != 3 {
		t.Fatalf("aggregate port methods=%d want exactly 3", got)
	}

	assertSessionIngressMethod(t, owner, "acceptSessionIngress", nil, []reflect.Type{reflect.TypeOf(false)})
	assertSessionIngressMethod(t, apply, "applySessionIngressData", []reflect.Type{reflect.TypeOf([]Event(nil)), reflect.TypeOf([]byte(nil))}, []reflect.Type{reflect.TypeOf([]Event(nil))})
	assertSessionIngressMethod(t, apply, "applySessionIngressEnd", []reflect.Type{reflect.TypeOf([]Event(nil)), reflect.TypeOf((*error)(nil)).Elem()}, []reflect.Type{reflect.TypeOf([]Event(nil))})
}

func assertSessionIngressMethod(t *testing.T, port reflect.Type, name string, inputs, outputs []reflect.Type) {
	t.Helper()
	method, ok := port.MethodByName(name)
	if !ok {
		t.Fatalf("%s missing method %s", port.Name(), name)
	}
	if method.Type.NumIn() != len(inputs) || method.Type.NumOut() != len(outputs) {
		t.Fatalf("%s.%s signature=%s", port.Name(), name, method.Type)
	}
	for i, want := range inputs {
		got := method.Type.In(i)
		if got != want {
			t.Fatalf("%s.%s input[%d]=%s want=%s", port.Name(), name, i, got, want)
		}
		assertSessionIngressBoundaryType(t, port.Name()+"."+name, got)
	}
	for i, want := range outputs {
		got := method.Type.Out(i)
		if got != want {
			t.Fatalf("%s.%s output[%d]=%s want=%s", port.Name(), name, i, got, want)
		}
		assertSessionIngressBoundaryType(t, port.Name()+"."+name, got)
	}
}

func assertSessionIngressBoundaryType(t *testing.T, name string, typ reflect.Type) {
	t.Helper()
	switch typ.Kind() {
	case reflect.Map, reflect.Func, reflect.Chan:
		t.Errorf("%s uses forbidden boundary bag %s", name, typ)
	case reflect.Interface:
		if typ.NumMethod() == 0 {
			t.Errorf("%s uses any/empty-interface bag %s", name, typ)
		}
	case reflect.Slice, reflect.Pointer, reflect.Array:
		assertSessionIngressBoundaryType(t, name, typ.Elem())
	}
	for _, forbidden := range []reflect.Type{
		reflect.TypeOf((*Mux)(nil)),
		reflect.TypeOf((*localSessionRegistry)(nil)),
		reflect.TypeOf((*pane)(nil)),
	} {
		if typ == forbidden {
			t.Errorf("%s exposes forbidden owner type %s", name, typ)
		}
	}
}

func TestSessionIngressControllerSourceIsPrivateBoundedAndUnwired(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	dir := filepath.Dir(testFile)
	productionPath := filepath.Join(dir, "session_ingress_controller.go")
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
	if !strings.Contains(comments.String(), "TODO(L3-01; expires Slice 6.2d)") {
		t.Fatal("controller TODO must keep L3-01 open through Slice 6.2d")
	}

	declarations := make(map[string]int)
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					assertSessionIngressPrivateName(t, spec.Name)
					declarations[spec.Name.Name]++
					assertSessionIngressTypeExpression(t, spec.Type)
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						assertSessionIngressPrivateName(t, name)
						declarations[name.Name]++
					}
				}
			}
		case *ast.FuncDecl:
			assertSessionIngressPrivateName(t, declaration.Name)
			if declaration.Recv == nil {
				declarations[declaration.Name.Name]++
			} else {
				declarations["method:"+declaration.Name.Name]++
			}
			assertSessionIngressFieldList(t, declaration.Recv)
			assertSessionIngressFieldList(t, declaration.Type.Params)
			assertSessionIngressFieldList(t, declaration.Type.Results)
		}
	}

	wantDeclarations := map[string]int{
		"sessionIngressControllerPortBudget": 1,
		"sessionIngressOwnerPort":            1,
		"sessionIngressApplyPort":            1,
		"sessionIngressController":           1,
		"newSessionIngressController":        1,
		"method:route":                       1,
	}
	if !reflect.DeepEqual(declarations, wantDeclarations) {
		t.Fatalf("controller declarations=%v want=%v", declarations, wantDeclarations)
	}

	muxFile, err := parser.ParseFile(fileSet, filepath.Join(dir, "mux.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var drainFound bool
	ast.Inspect(muxFile, func(node ast.Node) bool {
		if identifier, ok := node.(*ast.Ident); ok {
			if identifier.Name == "sessionIngressController" || identifier.Name == "newSessionIngressController" {
				t.Errorf("Mux.Drain is no longer authoritative: mux.go references %s", identifier.Name)
			}
		}
		if function, ok := node.(*ast.FuncDecl); ok && function.Name.Name == "Drain" {
			drainFound = true
		}
		return true
	})
	if !drainFound {
		t.Fatal("authoritative Mux.Drain method not found")
	}
}

func assertSessionIngressPrivateName(t *testing.T, identifier *ast.Ident) {
	t.Helper()
	if ast.IsExported(identifier.Name) {
		t.Errorf("controller seam exports %s", identifier.Name)
	}
}

func assertSessionIngressFieldList(t *testing.T, fields *ast.FieldList) {
	t.Helper()
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		for _, name := range field.Names {
			assertSessionIngressPrivateName(t, name)
		}
		assertSessionIngressTypeExpression(t, field.Type)
	}
}

func assertSessionIngressTypeExpression(t *testing.T, expression ast.Expr) {
	t.Helper()
	ast.Inspect(expression, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.MapType:
			t.Errorf("controller seam uses map bag at %v", node.Pos())
		case *ast.ChanType:
			t.Errorf("controller seam uses channel bag at %v", node.Pos())
		case *ast.FuncType:
			assertSessionIngressFieldList(t, node.Params)
			assertSessionIngressFieldList(t, node.Results)
			return false
		case *ast.InterfaceType:
			if node.Methods == nil || len(node.Methods.List) == 0 {
				t.Errorf("controller seam uses any/empty-interface bag at %v", node.Pos())
				return false
			}
			for _, method := range node.Methods.List {
				for _, name := range method.Names {
					assertSessionIngressPrivateName(t, name)
				}
				function, ok := method.Type.(*ast.FuncType)
				if !ok {
					t.Errorf("controller port embeds non-method type at %v", method.Pos())
					continue
				}
				assertSessionIngressFieldList(t, function.Params)
				assertSessionIngressFieldList(t, function.Results)
			}
			return false
		case *ast.Ident:
			forbidden := map[string]bool{
				"Mux": true, "localSessionRegistry": true, "pane": true,
				"parser": true, "Parser": true, "session": true, "Session": true,
				"terminal": true, "Terminal": true, "protocol": true, "Protocol": true,
				"resource": true, "Resource": true, "any": true,
			}
			if forbidden[node.Name] {
				t.Errorf("controller seam exposes forbidden type %s", node.Name)
			}
		}
		return true
	})
}
