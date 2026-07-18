package pty

import (
	"reflect"
	"testing"
)

func TestMergeEnvironmentDeterministicConfiguredWins(t *testing.T) {
	inherited := []string{"Z=last", "A=old", "A=inherited-winner", "INVALID", "M=middle"}
	configured := map[string]string{"A": "configured", "B": "value with spaces", "EMPTY": ""}
	want := []string{"A=configured", "B=value with spaces", "EMPTY=", "M=middle", "Z=last"}
	for i := 0; i < 20; i++ {
		if got := MergeEnvironment(inherited, configured, false); !reflect.DeepEqual(got, want) {
			t.Fatalf("merge %d = %#v, want %#v", i, got, want)
		}
	}
}

func TestMergeEnvironmentWindowsKeysAreCaseInsensitive(t *testing.T) {
	inherited := []string{"Path=first", "PATH=second", "keep=yes", "=C:=C:\\parent"}
	configured := map[string]string{"path": "configured", "KEEP": "configured-too"}
	want := []string{"=C:=C:\\parent", "KEEP=configured-too", "path=configured"}
	if got := MergeEnvironment(inherited, configured, true); !reflect.DeepEqual(got, want) {
		t.Fatalf("windows merge = %#v, want %#v", got, want)
	}
}

func TestMergeEnvironmentWindowsConfiguredCaseCollisionIsDeterministic(t *testing.T) {
	configured := map[string]string{"path": "lower", "PATH": "upper", "Path": "mixed"}
	want := []string{"path=lower"}
	for i := 0; i < 20; i++ {
		if got := MergeEnvironment(nil, configured, true); !reflect.DeepEqual(got, want) {
			t.Fatalf("merge %d = %#v, want %#v", i, got, want)
		}
	}
}

func TestMergeEnvironmentUnixKeysRemainCaseSensitive(t *testing.T) {
	got := MergeEnvironment([]string{"Path=one", "PATH=two"}, map[string]string{"path": "three"}, false)
	want := []string{"PATH=two", "Path=one", "path=three"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unix merge = %#v, want %#v", got, want)
	}
}
