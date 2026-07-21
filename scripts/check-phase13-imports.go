//go:build ignore

package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var termImageForbidden = []string{
	"cervterm/internal/core",
	"cervterm/internal/render",
	"cervterm/internal/mux",
	"cervterm/internal/frontend",
}

func main() {
	root, err := repositoryRoot()
	if err != nil {
		fatalf("repository root: %v", err)
	}
	var violations []string
	for _, sourceRoot := range []string{"cmd", "internal"} {
		scanRoot := filepath.Join(root, sourceRoot)
		err = filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			// The boundary is a production dependency rule. Tests may deliberately use
			// frontend fakes while production packages remain toolkit-independent.
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
			inFrontend := strings.HasPrefix(rel, "internal/frontend/")
			for _, spec := range file.Imports {
				importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
				if unquoteErr != nil {
					return unquoteErr
				}
				if inTermImage && hasForbiddenPrefix(importPath, termImageForbidden) {
					violations = append(violations, fmt.Sprintf("%s imports forbidden termimage dependency %q", rel, importPath))
				}
				if !inFrontend && (importPath == "github.com/go-gl/gl" || strings.HasPrefix(importPath, "github.com/go-gl/gl/") || importPath == "github.com/go-gl/glfw" || strings.HasPrefix(importPath, "github.com/go-gl/glfw/")) {
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
		for _, violation := range violations {
			fmt.Fprintln(os.Stderr, violation)
		}
		os.Exit(1)
	}
	fmt.Println("phase 13 import boundaries ok")
}

func hasForbiddenPrefix(importPath string, forbidden []string) bool {
	for _, prefix := range forbidden {
		if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
			return true
		}
	}
	return false
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
	fmt.Fprintf(os.Stderr, "check-phase13-imports: "+format+"\n", args...)
	os.Exit(1)
}
