//go:build ignore

// check-maturity-gates enforces lightweight beta-maturity guardrails that are
// cheap enough to run in CI. It intentionally checks repository structure and
// documentation promises rather than external services.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type finding struct {
	path   string
	reason string
}

var requiredDocs = []string{
	"SUPPORT.md",
	".github/ISSUE_TEMPLATE/bug_report.yml",
	".github/ISSUE_TEMPLATE/feature_request.yml",
	".github/ISSUE_TEMPLATE/install_problem.yml",
	".github/ISSUE_TEMPLATE/rendering_bug.yml",
	".github/dependabot.yml",
	"docs/maturity-improvement-plan.md",
	"docs/maturity-improvement-review.md",
	"docs/product-ux-maintainability-to-9-plan.md",
	"docs/project-maturity-analysis.md",
	"docs/release-stabilization-plan.md",
	"docs/release-trust.md",
	"docs/troubleshooting.md",
	"docs/getting-started.md",
	"docs/daily-driver-smoke.md",
	"scripts/daily-driver-smoke.ps1",
}

var largeGoAllowlist = map[string]string{
	filepath.ToSlash("internal/fontglyph/backend.go"):           "known font fallback/raster orchestration split target",
	filepath.ToSlash("internal/fontglyph/color_colr_render.go"): "known COLRv1 render split target",
}

func main() {
	var findings []finding
	findings = append(findings, checkRequiredDocs()...)
	findings = append(findings, checkLargeGoFiles()...)
	findings = append(findings, checkStaleVersions()...)
	findings = append(findings, checkReleaseTrustDoc()...)
	findings = append(findings, checkCIGates()...)
	if len(findings) > 0 {
		fmt.Fprintln(os.Stderr, "maturity gate failures:")
		for _, f := range findings {
			fmt.Fprintf(os.Stderr, "FAIL %-48s %s\n", f.path, f.reason)
		}
		os.Exit(1)
	}
	fmt.Println("maturity gates ok")
	for path, reason := range largeGoAllowlist {
		fmt.Printf("known large-file exception: %s (%s)\n", path, reason)
	}
}

func checkRequiredDocs() []finding {
	var findings []finding
	for _, path := range requiredDocs {
		info, err := os.Stat(path)
		if err != nil {
			findings = append(findings, finding{path: path, reason: "required maturity/support file is missing"})
			continue
		}
		if info.IsDir() || info.Size() == 0 {
			findings = append(findings, finding{path: path, reason: "required maturity/support file is empty or a directory"})
		}
	}
	return findings
}

func checkLargeGoFiles() []finding {
	var findings []finding
	_ = filepath.WalkDir(".", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			findings = append(findings, finding{path: path, reason: err.Error()})
			return nil
		}
		if entry.IsDir() {
			slash := filepath.ToSlash(path)
			if slash == ".git" || slash == "dist" || slash == ".tmp" || strings.HasPrefix(slash, ".architecture-ai-project-advisor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		slash := filepath.ToSlash(path)
		if !strings.HasPrefix(slash, "internal/") && !strings.HasPrefix(slash, "cmd/") {
			return nil
		}
		lines, err := countLines(path)
		if err != nil {
			findings = append(findings, finding{path: slash, reason: err.Error()})
			return nil
		}
		if lines > 500 {
			if _, ok := largeGoAllowlist[slash]; !ok {
				findings = append(findings, finding{path: slash, reason: fmt.Sprintf("production Go file has %d lines; split it or add an explicit maturity-plan exception", lines)})
			}
		}
		return nil
	})
	return findings
}

func checkStaleVersions() []finding {
	var findings []finding
	for _, path := range []string{
		"README.md",
		"docs/release-packaging.md",
		"packaging/winget/README.md",
		"packaging/wix/README.md",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			findings = append(findings, finding{path: path, reason: err.Error()})
			continue
		}
		if strings.Contains(string(data), "0.1.0-beta.1") || strings.Contains(string(data), "v0.1.0-beta.1") {
			findings = append(findings, finding{path: path, reason: "stale 0.1.0 beta example; use <tag> or the current beta tag"})
		}
	}
	return findings
}

func checkReleaseTrustDoc() []finding {
	path := "docs/release-trust.md"
	data, err := os.ReadFile(path)
	if err != nil {
		return []finding{{path: path, reason: err.Error()}}
	}
	text := strings.ToLower(string(data))
	var findings []finding
	for _, required := range []string{"sha256", "attestation", "authenticode", "unsigned"} {
		if !strings.Contains(text, required) {
			findings = append(findings, finding{path: path, reason: "release trust doc must mention " + required})
		}
	}
	return findings
}

func checkCIGates() []finding {
	path := ".github/workflows/ci.yml"
	data, err := os.ReadFile(path)
	if err != nil {
		return []finding{{path: path, reason: err.Error()}}
	}
	text := string(data)
	var findings []finding
	for _, required := range []string{"go vet", "govulncheck ./...", "scripts/daily-driver-smoke.ps1"} {
		if !strings.Contains(text, required) {
			findings = append(findings, finding{path: path, reason: "CI must run " + required})
		}
	}
	return findings
}

func countLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	lines := 0
	for scanner.Scan() {
		lines++
	}
	return lines, scanner.Err()
}
