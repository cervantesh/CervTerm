//go:build ignore

package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"cervterm/internal/core"
)

var (
	termImageForbidden = []string{
		"cervterm/internal/core",
		"cervterm/internal/render",
		"cervterm/internal/mux",
		"cervterm/internal/frontend",
	}
	kittyForbidden = append(append([]string{}, termImageForbidden...),
		"cervterm/internal/config", "cervterm/internal/vt",
	)
	phase14StandardAllowed = map[string]struct{}{
		"bytes": {}, "context": {}, "encoding/base64": {}, "errors": {}, "fmt": {},
		"image": {}, "image/color": {}, "image/png": {}, "io": {}, "math": {},
		"strconv": {}, "strings": {}, "sync": {}, "sync/atomic": {}, "time": {},
	}
)

var authorityMarkers = map[string]string{
	".architecture-ai-project-advisor/.asi/active-project.json":                                                                                 "\"phase\": \"14\"",
	".architecture-ai-project-advisor/.asi/projects/index.json":                                                                                 "\"phase\": \"14\"",
	".architecture-ai-project-advisor/.asi/projects/cervterm/context.json":                                                                      "\"phase\": \"14\"",
	".architecture-ai-project-advisor/.asi/projects/cervterm/context.md":                                                                        "CervTerm Phase 14 Context",
	".architecture-ai-project-advisor/.asi/projects/cervterm/decisions/0016-refine-phase-14-framing-cursor-and-ephemeral-resource-semantics.md": "Accepted; supersedes ADR 0015",
	".architecture-ai-project-advisor/.asi/projects/cervterm/designs/feature-design.md":                                                         "Phase 14 Feature Design",
	".architecture-ai-project-advisor/.asi/projects/cervterm/plans/implementation-plan.md":                                                      "Phase 14 Implementation Plan",
	".architecture-ai-project-advisor/.asi/projects/cervterm/guardrails.md":                                                                     "Phase 14 Guardrails",
	".architecture-ai-project-advisor/.asi/projects/cervterm/preflight.md":                                                                      "Phase 14 Bounded Sixel and iTerm",
}

func main() {
	root, err := repositoryRoot()
	if err != nil {
		fatalf("repository root: %v", err)
	}
	var violations []string
	if size := reflect.TypeOf(core.Cell{}).Size(); size != 32 {
		violations = append(violations, fmt.Sprintf("core.Cell size = %d, want 32", size))
	}
	violations = append(violations, checkAuthority(root)...)
	for _, sourceRoot := range []string{"cmd", "internal"} {
		scanRoot := filepath.Join(root, sourceRoot)
		err = filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if parseErr != nil {
				return parseErr
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			rel = filepath.ToSlash(rel)
			inTermImage := strings.HasPrefix(rel, "internal/termimage/")
			inKitty := strings.HasPrefix(rel, "internal/kitty/")
			inPhase14Protocol := strings.HasPrefix(rel, "internal/sixel/") || strings.HasPrefix(rel, "internal/itermimage/")
			inFrontend := strings.HasPrefix(rel, "internal/frontend/")
			for _, spec := range file.Imports {
				importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
				if unquoteErr != nil {
					return unquoteErr
				}
				switch {
				case inTermImage && hasForbiddenPrefix(importPath, termImageForbidden):
					violations = append(violations, fmt.Sprintf("%s imports forbidden termimage dependency %q", rel, importPath))
				case inKitty && hasForbiddenPrefix(importPath, kittyForbidden):
					violations = append(violations, fmt.Sprintf("%s imports forbidden kitty dependency %q", rel, importPath))
				case inPhase14Protocol && !phase14ImportAllowed(importPath):
					violations = append(violations, fmt.Sprintf("%s imports unapproved Phase 14 protocol dependency %q", rel, importPath))
				}
				if !inFrontend && isFrontendOnly(importPath) {
					violations = append(violations, fmt.Sprintf("%s imports frontend-only dependency %q", rel, importPath))
				}
			}
			return nil
		})
		if err != nil {
			fatalf("scan %s imports: %v", sourceRoot, err)
		}
	}
	if len(violations) != 0 {
		sort.Strings(violations)
		for _, violation := range violations {
			fmt.Fprintln(os.Stderr, violation)
		}
		os.Exit(1)
	}
	fmt.Println("phase 14 architecture and import boundaries ok")
}

func checkAuthority(root string) []string {
	var violations []string
	for path, marker := range authorityMarkers {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			violations = append(violations, fmt.Sprintf("authority %s: %v", path, err))
			continue
		}
		if !strings.Contains(string(content), marker) {
			violations = append(violations, fmt.Sprintf("authority %s lacks marker %q", path, marker))
		}
	}
	return violations
}

func hasForbiddenPrefix(importPath string, forbidden []string) bool {
	for _, prefix := range forbidden {
		if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
			return true
		}
	}
	return false
}

func phase14ImportAllowed(importPath string) bool {
	if importPath == "cervterm/internal/termimage" {
		return true
	}
	if strings.HasPrefix(importPath, "cervterm/") || strings.Contains(importPath, ".") {
		return false
	}
	_, ok := phase14StandardAllowed[importPath]
	return ok
}

func isFrontendOnly(importPath string) bool {
	return importPath == "github.com/go-gl/gl" || strings.HasPrefix(importPath, "github.com/go-gl/gl/") ||
		importPath == "github.com/go-gl/glfw" || strings.HasPrefix(importPath, "github.com/go-gl/glfw/")
}

func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if info, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "check-phase14-imports: "+format+"\n", args...)
	os.Exit(1)
}
