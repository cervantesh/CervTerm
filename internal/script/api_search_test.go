package script

import (
	"strings"
	"testing"

	"cervterm/internal/config"
)

func TestTermSearch(t *testing.T) {
	path := writeScriptConfig(t, `return {
  keys = {
    { key = "f", action = function(term)
        if term:search("needle") then
          term:notify("hit")
        else
          term:notify("miss")
        end
      end },
  },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()

	hit := &fakeHost{searchResult: true}
	if err := rt.Dispatch(0, hit); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if got := strings.Join(hit.searches, ""); got != "needle" {
		t.Fatalf("search query = %q", got)
	}
	if got := strings.Join(hit.notices, ""); got != "hit" {
		t.Fatalf("hit notice = %q, want hit", got)
	}

	miss := &fakeHost{searchResult: false}
	if err := rt.Dispatch(0, miss); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if got := strings.Join(miss.notices, ""); got != "miss" {
		t.Fatalf("miss notice = %q, want miss", got)
	}
}
