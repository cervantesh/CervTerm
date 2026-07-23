//go:build ignore

// check-maturity-gates enforces lightweight beta-maturity guardrails that are
// cheap enough to run in CI. It intentionally checks repository structure and
// documentation promises rather than external services.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type finding struct {
	path   string
	reason string
}

type phase15Evidence struct {
	SchemaVersion   int                  `json:"schema_version"`
	Phase           int                  `json:"phase"`
	PhaseStatus     string               `json:"phase_status"`
	BaselineCommit  string               `json:"baseline_commit"`
	BaselineRelease string               `json:"baseline_release"`
	States          []string             `json:"states"`
	Rows            []phase15EvidenceRow `json:"rows"`
}

type phase15EvidenceRow struct {
	ID            string   `json:"id"`
	Status        string   `json:"status"`
	Attempted     bool     `json:"attempted"`
	Commit        string   `json:"commit,omitempty"`
	Environment   string   `json:"environment,omitempty"`
	Configuration string   `json:"configuration,omitempty"`
	Evidence      []string `json:"evidence,omitempty"`
	Prerequisite  string   `json:"prerequisite,omitempty"`
	Exclusion     string   `json:"exclusion,omitempty"`
}

type phase15SupportMatrix struct {
	Features []phase15SupportFeature `json:"features"`
}

type phase15SupportFeature struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	DefaultEnabled *bool  `json:"default_enabled"`
	SupportClaim   string `json:"support_claim"`
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
	"docs/wezterm-parity-roadmap.md",
	"docs/parity-baseline.md",
	"docs/parity-support-matrix.json",
	"docs/config-compatibility-policy.md",
	"docs/config-migration.md",
	"docs/validation/phase-15-preflight.md",
	"docs/validation/phase-15-evidence.json",
	"scripts/capture-parity-baseline.go",
	"scripts/check-phase15-recovery.go",
	"scripts/daily-driver-smoke.go",
	"scripts/package-beta.go",
	"scripts/release-preflight.go",
	"scripts/smoke-installed-package.go",
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
	findings = append(findings, checkPhase15Evidence()...)
	findings = append(findings, checkPhase15SupportMatrix()...)
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
	for _, required := range []string{"go vet", "govulncheck ./...", "scripts/check-phase15-recovery.go", "scripts/package-beta.go", "scripts/release-preflight.go", "scripts/smoke-installed-package.go", "scripts/daily-driver-smoke.go"} {
		if !strings.Contains(text, required) {
			findings = append(findings, finding{path: path, reason: "CI must run " + required})
		}
	}
	for _, forbidden := range []string{"power" + "shell", "p" + "wsh"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			findings = append(findings, finding{path: path, reason: "CI must not invoke forbidden shell host"})
		}
	}
	return findings
}

func checkPhase15SupportMatrix() []finding {
	const path = "docs/parity-support-matrix.json"
	data, err := os.ReadFile(path)
	if err != nil {
		return []finding{{path: path, reason: err.Error()}}
	}
	var document phase15SupportMatrix
	if err := json.Unmarshal(data, &document); err != nil {
		return []finding{{path: path, reason: "invalid JSON: " + err.Error()}}
	}
	var findings []finding
	features := make(map[string]phase15SupportFeature, len(document.Features))
	for _, feature := range document.Features {
		if feature.ID == "" {
			findings = append(findings, finding{path: path, reason: "support feature has empty id"})
			continue
		}
		if _, duplicate := features[feature.ID]; duplicate {
			findings = append(findings, finding{path: path, reason: "duplicate support feature " + feature.ID})
		}
		features[feature.ID] = feature
	}
	for _, id := range []string{"input.ime_preedit", "accessibility.windows_uia", "shell.windows_native_notifications", "graphics.kitty", "graphics.sixel_iterm"} {
		feature, exists := features[id]
		if !exists {
			findings = append(findings, finding{path: path, reason: "missing experimental support feature " + id})
			continue
		}
		claim := strings.TrimSpace(feature.SupportClaim)
		if feature.Status != "experimental" || feature.DefaultEnabled == nil || *feature.DefaultEnabled || claim == "" || claim == "supported" {
			findings = append(findings, finding{path: path, reason: id + " must remain experimental, explicit default-off, and non-supported"})
		}
	}
	for _, id := range []string{"renderer.selection", "domains.local_ssh_wsl", "mux.live_detach_reattach"} {
		if feature, exists := features[id]; !exists || feature.Status != "excluded" {
			findings = append(findings, finding{path: path, reason: id + " must remain excluded"})
		}
	}
	return findings
}

func checkPhase15Evidence() []finding {
	const path = "docs/validation/phase-15-evidence.json"
	data, err := os.ReadFile(path)
	if err != nil {
		return []finding{{path: path, reason: err.Error()}}
	}
	var document phase15Evidence
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return []finding{{path: path, reason: "invalid JSON: " + err.Error()}}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return []finding{{path: path, reason: "JSON must contain exactly one document"}}
	}
	var findings []finding
	if document.SchemaVersion != 1 || document.Phase != 15 {
		findings = append(findings, finding{path: path, reason: "expected schema_version=1 and phase=15"})
	}
	if document.PhaseStatus != "in_progress" && document.PhaseStatus != "complete" {
		findings = append(findings, finding{path: path, reason: "phase_status must be in_progress or complete"})
	}
	if strings.TrimSpace(document.BaselineCommit) == "" || strings.TrimSpace(document.BaselineRelease) == "" {
		findings = append(findings, finding{path: path, reason: "baseline_commit and baseline_release are required"})
	}
	wantStates := []string{"PASS", "FAIL", "SKIP", "UNRUN", "NOT-APPLICABLE"}
	if strings.Join(document.States, "|") != strings.Join(wantStates, "|") {
		findings = append(findings, finding{path: path, reason: "evidence states must be PASS, FAIL, SKIP, UNRUN, NOT-APPLICABLE in canonical order"})
	}
	requiredRows := []string{
		"authority.preflight", "release.incoming_checkpoint", "compatibility.doctor",
		"config.real_user_migrations", "recovery.redaction", "performance.phase15",
		"security.automated", "accessibility.automated", "platform.windows_daily_driver",
		"platform.windows_real_gui", "platform.linux_headless", "platform.linux_real_gui",
		"platform.macos_build", "platform.macos_real_gui", "release.candidate_readiness",
		"release.phase15_checkpoint",
	}
	seen := make(map[string]phase15EvidenceRow, len(document.Rows))
	for _, row := range document.Rows {
		if row.ID == "" {
			findings = append(findings, finding{path: path, reason: "evidence row has empty id"})
			continue
		}
		if _, exists := seen[row.ID]; exists {
			findings = append(findings, finding{path: path, reason: "duplicate evidence row " + row.ID})
		}
		seen[row.ID] = row
		for _, item := range row.Evidence {
			item = strings.TrimSpace(item)
			if item == "" {
				findings = append(findings, finding{path: path, reason: row.ID + " has empty evidence"})
			} else if !strings.HasPrefix(item, "https://") {
				if info, statErr := os.Stat(filepath.FromSlash(item)); statErr != nil || info.IsDir() {
					findings = append(findings, finding{path: path, reason: row.ID + " references missing evidence " + item})
				}
			}
		}
		identityComplete := strings.TrimSpace(row.Commit) != "" && strings.TrimSpace(row.Environment) != "" && strings.TrimSpace(row.Configuration) != ""
		switch row.Status {
		case "PASS", "FAIL":
			if !row.Attempted || !identityComplete || len(row.Evidence) == 0 || row.Prerequisite != "" || row.Exclusion != "" {
				findings = append(findings, finding{path: path, reason: row.ID + " requires attempted identity/evidence and no disposition fields"})
			}
		case "SKIP":
			if !row.Attempted || !identityComplete || strings.TrimSpace(row.Prerequisite) == "" || len(row.Evidence) == 0 || row.Exclusion != "" {
				findings = append(findings, finding{path: path, reason: row.ID + " SKIP requires attempted identity, prerequisite, evidence, and no exclusion"})
			}
		case "UNRUN":
			if row.Attempted || row.Commit != "" || row.Environment != "" || row.Configuration != "" || len(row.Evidence) != 0 || row.Prerequisite != "" || row.Exclusion != "" {
				findings = append(findings, finding{path: path, reason: row.ID + " UNRUN cannot carry execution or disposition fields"})
			}
		case "NOT-APPLICABLE":
			if row.Attempted || row.Commit != "" || row.Environment != "" || row.Configuration != "" || row.Prerequisite != "" || strings.TrimSpace(row.Exclusion) == "" {
				findings = append(findings, finding{path: path, reason: row.ID + " NOT-APPLICABLE requires only an exclusion and optional evidence"})
			}
		default:
			findings = append(findings, finding{path: path, reason: row.ID + " has unknown status " + row.Status})
		}
	}
	if len(seen) != len(requiredRows) {
		findings = append(findings, finding{path: path, reason: "evidence rows must match the canonical Phase 15 inventory"})
	}
	for _, id := range requiredRows {
		if _, exists := seen[id]; !exists {
			findings = append(findings, finding{path: path, reason: "missing required evidence row " + id})
		}
	}
	if document.PhaseStatus == "complete" {
		for _, id := range []string{
			"authority.preflight", "release.incoming_checkpoint", "compatibility.doctor",
			"config.real_user_migrations", "recovery.redaction", "performance.phase15",
			"security.automated", "accessibility.automated", "platform.windows_daily_driver",
			"platform.linux_headless", "platform.macos_build", "release.candidate_readiness",
			"release.phase15_checkpoint",
		} {
			if row := seen[id]; row.Status != "PASS" {
				findings = append(findings, finding{path: path, reason: id + " must PASS before phase_status=complete"})
			}
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
