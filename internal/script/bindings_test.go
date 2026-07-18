package script

import (
	"fmt"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func decodeBindingFixture(t *testing.T, body string) (BindingSet, callbackTable, error) {
	t.Helper()
	state := lua.NewState()
	defer state.Close()
	if err := state.DoString("cfg=" + body); err != nil {
		t.Fatal(err)
	}
	return loadBindingSet(state.GetGlobal("cfg").(*lua.LTable))
}

func TestBindingSetDecodesLeaderTablesMouseAndStableCallbackRefs(t *testing.T) {
	set, callbacks, err := decodeBindingFixture(t, `{leader={key="a",mods="ctrl",timeout_ms=900},keys={{key="p",table="pane"}},key_tables={{name="pane",one_shot=false,timeout_ms=1200,keys={{key="h",action=function() end}}}},mouse_bindings={{event="press",button="right",mods="shift",click_count=1,action=function() end}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if set.Leader == nil || set.Leader.TimeoutMS != 900 || len(set.Tables) != 1 || set.Tables[0].OneShot || set.Tables[0].TimeoutMS != 1200 || len(set.Mouse) != 1 {
		t.Fatalf("set=%#v", set)
	}
	tableRef := CallbackRef{Domain: CallbackTable, Table: "pane", Slot: 0}
	mouseRef := CallbackRef{Domain: CallbackMouse, Slot: 0}
	if callbacks[tableRef] == nil || callbacks[mouseRef] == nil {
		t.Fatalf("callback refs=%v", callbacks)
	}
	clone := set.Clone()
	clone.Tables[0].Bindings[0].Label = "changed"
	if set.Tables[0].Bindings[0].Label == "changed" {
		t.Fatal("BindingSet clone aliased table binding")
	}
}

func TestBindingSetRejectsDuplicatesUnknownTransitionsAndCycles(t *testing.T) {
	cases := []struct{ body, want string }{
		{`{keys={{key="a",action=function() end},{key="a",action=function() end}}}`, "duplicate key"},
		{`{keys={{key="a",table="missing"}}}`, "unknown key table"},
		{`{keys={{key="a",table="one"}},key_tables={{name="one",keys={{key="b",table="one"}}}}}`, "cycle"},
		{`{key_tables={{name="x",timeout_ms=99,keys={}}}}`, "between 100 and 10000"},
		{`{mouse_bindings={{event="wheel",button="left",action=function() end}}}`, "wheel requires"},
	}
	for _, tc := range cases {
		_, _, err := decodeBindingFixture(t, tc.body)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("body=%s err=%v want %q", tc.body, err, tc.want)
		}
	}
}

func TestBindingSetEnforcesTableAndMouseBounds(t *testing.T) {
	var tables strings.Builder
	tables.WriteString(`{key_tables={`)
	for i := 0; i < MaxBindingTables+1; i++ {
		if i > 0 {
			tables.WriteByte(',')
		}
		tables.WriteString(fmt.Sprintf(`{name="t%d",keys={}}`, i))
	}
	tables.WriteString(`}}`)
	if _, _, err := decodeBindingFixture(t, tables.String()); err == nil || !strings.Contains(err.Error(), "at most 32") {
		t.Fatalf("table bound err=%v", err)
	}
}
