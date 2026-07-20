package accessibility

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestAccessibilityPackageHasNoFrontendMuxPTYOrNativeImports(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller path unavailable")
	}
	directory := filepath.Dir(current)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range file.Imports {
			name, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"cervterm/internal/core", "cervterm/internal/render", "cervterm/internal/frontend", "cervterm/internal/mux", "cervterm/internal/pty", "golang.org/x/sys", "github.com/go-gl"} {
				if strings.HasPrefix(name, forbidden) {
					t.Fatalf("%s imports forbidden dependency %q", entry.Name(), name)
				}
			}
		}
	}
}
