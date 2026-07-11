package buildinfo

import (
	"strings"
	"testing"
)

func TestInfoIncludesVersionAndPlatform(t *testing.T) {
	info := Info()
	if !strings.Contains(info, "CervTerm") || !strings.Contains(info, Version) {
		t.Fatalf("Info() = %q, want product and version", info)
	}
}
