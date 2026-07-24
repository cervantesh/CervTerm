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
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	"docs/validation/phase-15-performance.md",
	"docs/validation/phase-15-performance.json",
	"docs/validation/phase-15-platform-qualification.md",
	"docs/validation/phase-15-platform-manifest.json",
	"docs/validation/phase-15-process-comparison.json",
	"docs/validation/phase-15-security-accessibility.md",
	"docs/validation/phase-15-security-manifest.json",
	"docs/validation/architecture-maturity-slice-6.3c.md",
	"scripts/capture-parity-baseline.go",
	"scripts/capture-phase15-benchmarks.go",
	"scripts/capture-phase15-process.py",
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
	findings = append(findings, checkSlice63cGuard()...)
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

type slice63cControllerSpec struct {
	path       string
	controller string
	budgetName string
	budget     int
	fields     []string
	ports      map[string]int
}

var slice63cAllowedPaths = []string{
	"docs/architecture-maturity/implementation-plan.md",
	"docs/architecture.md",
	"docs/validation/architecture-maturity-slice-6.3c.md",
	"docs/validation/architecture-maturity-slice-6.3c/benchmarks-base.txt",
	"docs/validation/architecture-maturity-slice-6.3c/benchmarks-candidate.txt",
	"docs/validation/architecture-maturity-slice-6.3c/gates.txt",
	"docs/validation/architecture-maturity-slice-6.3c/scope-and-commits.txt",
	"internal/frontend/glfwgl/action_bindings.go",
	"internal/frontend/glfwgl/action_executor.go",
	"internal/frontend/glfwgl/app.go",
	"internal/frontend/glfwgl/app_bell.go",
	"internal/frontend/glfwgl/app_callbacks.go",
	"internal/frontend/glfwgl/app_host.go",
	"internal/frontend/glfwgl/app_loop.go",
	"internal/frontend/glfwgl/app_mux.go",
	"internal/frontend/glfwgl/app_mux_test.go",
	"internal/frontend/glfwgl/app_overlay.go",
	"internal/frontend/glfwgl/app_script_native_characterization_test.go",
	"internal/frontend/glfwgl/app_status.go",
	"internal/frontend/glfwgl/command_palette.go",
	"internal/frontend/glfwgl/events_glfw.go",
	"internal/frontend/glfwgl/initial_projection.go",
	"internal/frontend/glfwgl/mouse_bindings.go",
	"internal/frontend/glfwgl/native_capability_controller.go",
	"internal/frontend/glfwgl/native_capability_controller_app.go",
	"internal/frontend/glfwgl/native_capability_controller_test.go",
	"internal/frontend/glfwgl/projection_factory_glfw.go",
	"internal/frontend/glfwgl/projection_ime_windows_test.go",
	"internal/frontend/glfwgl/reload.go",
	"internal/frontend/glfwgl/script_host_controller.go",
	"internal/frontend/glfwgl/script_host_controller_app.go",
	"internal/frontend/glfwgl/script_host_controller_test.go",
	"internal/frontend/glfwgl/script_lifecycle_controller.go",
	"internal/frontend/glfwgl/script_lifecycle_controller_app.go",
	"internal/frontend/glfwgl/script_lifecycle_controller_test.go",
	"scripts/check-maturity-gates.go",
}

var slice63cControllerSpecs = []slice63cControllerSpec{
	{
		path: "internal/frontend/glfwgl/script_host_controller.go", controller: "scriptHostController",
		budgetName: "scriptHostControllerPortBudget", budget: 21,
		fields: []string{"pane:termmux.PaneID", "initialized:bool"},
		ports:  map[string]int{"scriptHostConfigPort": 3, "scriptHostInputPort": 1, "scriptHostNotificationPort": 3, "scriptHostFontPort": 2, "scriptHostSelectionPort": 4, "scriptHostViewPort": 5, "scriptHostMutationPort": 3},
	},
	{
		path: "internal/frontend/glfwgl/script_lifecycle_controller.go", controller: "scriptLifecycleController",
		budgetName: "scriptLifecycleControllerPortBudget", budget: 14,
		fields: nil,
		ports:  map[string]int{"scriptLifecycleRuntimePort": 2, "scriptLifecycleEventPort": 5, "scriptLifecycleFailurePort": 1, "scriptLifecyclePendingPort": 3, "scriptLifecycleTimerPort": 1, "scriptLifecycleProjectionPort": 2},
	},
	{
		path: "internal/frontend/glfwgl/native_capability_controller.go", controller: "nativeCapabilityController",
		budgetName: "nativeCapabilityControllerPortBudget", budget: 8,
		fields: nil,
		ports:  map[string]int{"nativeInitialCapabilityPort": 4, "nativeChildCapabilityPort": 4},
	},
}

func checkSlice63cGuard() []finding {
	var findings []finding
	findings = append(findings, checkSlice63cPostGPolicySelfTest()...)
	for _, spec := range slice63cControllerSpecs {
		findings = append(findings, checkSlice63cController(spec)...)
	}
	findings = append(findings, checkSlice63cCommitsAndPaths()...)
	return findings
}

func checkSlice63cController(spec slice63cControllerSpec) []finding {
	data, err := os.ReadFile(spec.path)
	if err != nil {
		return []finding{{path: spec.path, reason: err.Error()}}
	}
	var findings []finding
	const expiry = "TODO(L1-01; expires Slice 6.3d): remove the preparatory facade adapters."
	if count := strings.Count(string(data), expiry); count != 1 {
		findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("must contain exactly one 6.3d facade-expiry TODO, found %d", count)})
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, spec.path, data, 0)
	if err != nil {
		return append(findings, finding{path: spec.path, reason: "cannot parse controller guard surface: " + err.Error()})
	}
	budgetFound := false
	controllerFound := false
	portCount := 0
	for _, declaration := range file.Decls {
		generic, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, item := range generic.Specs {
			switch node := item.(type) {
			case *ast.ValueSpec:
				for index, name := range node.Names {
					if name.Name != spec.budgetName || index >= len(node.Values) {
						continue
					}
					budgetFound = true
					literal, ok := node.Values[index].(*ast.BasicLit)
					value, parseErr := strconv.Atoi(strings.TrimSpace(literalValue(literal, ok)))
					if parseErr != nil || value != spec.budget {
						findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("%s must equal %d", spec.budgetName, spec.budget)})
					}
				}
			case *ast.TypeSpec:
				if node.Name.Name == spec.controller {
					controllerFound = true
					structure, ok := node.Type.(*ast.StructType)
					if !ok {
						findings = append(findings, finding{path: spec.path, reason: spec.controller + " must remain a private struct"})
						continue
					}
					fields := renderedFields(fset, structure)
					if strings.Join(fields, "|") != strings.Join(spec.fields, "|") {
						findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("%s fields changed: got %v want %v", spec.controller, fields, spec.fields)})
					}
					for _, field := range fields {
						findings = append(findings, forbiddenSlice63cType(spec.path, spec.controller+" field "+field, field)...)
					}
				}
				wantMethods, isPort := spec.ports[node.Name.Name]
				if !isPort {
					continue
				}
				port, ok := node.Type.(*ast.InterfaceType)
				if !ok {
					findings = append(findings, finding{path: spec.path, reason: node.Name.Name + " must remain a private interface"})
					continue
				}
				methods := len(port.Methods.List)
				if methods != wantMethods || methods == 0 || methods > 5 {
					findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("%s methods=%d want=%d and <=5", node.Name.Name, methods, wantMethods)})
				}
				portCount += methods
				for _, method := range port.Methods.List {
					function, ok := method.Type.(*ast.FuncType)
					if !ok {
						findings = append(findings, finding{path: spec.path, reason: node.Name.Name + " embeds a non-method surface"})
						continue
					}
					for _, list := range []*ast.FieldList{function.Params, function.Results} {
						if list == nil {
							continue
						}
						for _, parameter := range list.List {
							var rendered bytes.Buffer
							_ = format.Node(&rendered, fset, parameter.Type)
							findings = append(findings, forbiddenSlice63cType(spec.path, node.Name.Name, rendered.String())...)
						}
					}
				}
			}
		}
	}
	if !budgetFound {
		findings = append(findings, finding{path: spec.path, reason: "missing fixed port budget " + spec.budgetName})
	}
	if !controllerFound {
		findings = append(findings, finding{path: spec.path, reason: "missing private controller " + spec.controller})
	}
	if portCount != spec.budget {
		findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("aggregate port methods=%d budget=%d", portCount, spec.budget)})
	}
	return findings
}

func literalValue(literal *ast.BasicLit, ok bool) string {
	if !ok || literal == nil {
		return ""
	}
	return literal.Value
}

func renderedFields(fset *token.FileSet, structure *ast.StructType) []string {
	var fields []string
	for _, field := range structure.Fields.List {
		var rendered bytes.Buffer
		_ = format.Node(&rendered, fset, field.Type)
		for _, name := range field.Names {
			fields = append(fields, name.Name+":"+rendered.String())
		}
	}
	return fields
}

func forbiddenSlice63cType(path, owner, typeText string) []finding {
	lower := strings.ToLower(typeText)
	for _, forbidden := range []string{
		"*app", "*mux.mux", "*glfw.window", "*script.runtime", "nativeprojectionbundle",
		"compositionbeforeunbind", "wndproc", "gpu.", "prepared", "projectionresource", "map[", "func(", "chan ",
	} {
		if strings.Contains(lower, forbidden) {
			return []finding{{path: path, reason: owner + " has forbidden ownership/structural type " + typeText}}
		}
	}
	return nil
}

func checkSlice63cCommitsAndPaths() []finding {
	const (
		base        = "7656960bd334640b5e4c377bbde71b1dc9d6a3c1"
		tCommit     = "412a5ce"
		aCommit     = "43afad3"
		mCommit     = "5d9628c"
		wSubject    = "refactor(frontend): wire script and native controllers"
		gSubject    = "refactor(frontend): guard script and native controller delegation"
		sliceBranch = "arch/l1-01c-app-script-native-prep"
	)
	var findings []finding
	stages := []struct {
		class, commit, parent, subject string
	}{
		{"T", tCommit, base, "test(frontend): characterize script and native lifecycle"},
		{"A", aCommit, tCommit, "refactor(frontend): add script and native controller seams"},
		{"M", mCommit, aCommit, "refactor(frontend): split script and native adapters"},
	}
	if shallow, _ := gitText("rev-parse", "--is-shallow-repository"); shallow == "true" {
		for _, commit := range []string{base, tCommit, aCommit, mCommit} {
			if _, err := gitText("cat-file", "-e", commit+"^{commit}"); err != nil {
				return checkSlice63cDocumentedSequence(gSubject)
			}
		}
	}
	for _, stage := range stages {
		identity, err := gitFields("show", "-s", "--format=%H%x00%P%x00%s", stage.commit)
		if err != nil || len(identity) != 3 {
			findings = append(findings, finding{path: "git:" + stage.class, reason: "missing Slice 6.3c commit " + stage.commit})
			continue
		}
		parent, parentErr := gitText("rev-parse", stage.parent+"^{commit}")
		if parentErr != nil || identity[1] != parent || identity[2] != stage.subject {
			findings = append(findings, finding{path: "git:" + stage.class, reason: fmt.Sprintf("unexpected parent/subject for %s: parent=%s subject=%q", identity[0], identity[1], identity[2])})
		}
	}

	wCommit, wErr := findSlice63cCommit(wSubject, mCommit)
	if wErr != nil {
		return append(findings, finding{path: "git:W", reason: wErr.Error()})
	}
	gCommit, _ := findSlice63cCommit(gSubject, wCommit)
	head, _ := gitText("rev-parse", "HEAD")
	end := gCommit
	if gCommit == "" {
		if head != wCommit {
			findings = append(findings, finding{path: "git:G", reason: "before G, HEAD must be the exact W commit"})
			return findings
		}
		end = wCommit
	} else {
		branch, _ := gitText("symbolic-ref", "--quiet", "--short", "HEAD")
		worktree, _ := gitText("status", "--porcelain=v1", "--untracked-files=all")
		findings = append(findings, checkSlice63cPostGActiveState(branch == sliceBranch, head, gCommit, worktree)...)
	}

	pathsText, pathErr := gitText("diff", "--name-only", base+".."+end)
	if pathErr != nil {
		return append(findings, finding{path: "git:paths", reason: pathErr.Error()})
	}
	paths := nonEmptyLines(pathsText)
	if gCommit == "" {
		// Before G freezes the slice, include every tracked and nonignored untracked
		// worktree path in addition to the immutable base..W commit range.
		unstaged, _ := gitText("diff", "--name-only")
		staged, _ := gitText("diff", "--cached", "--name-only")
		untracked, _ := gitText("ls-files", "--others", "--exclude-standard")
		paths = append(paths, nonEmptyLines(unstaged)...)
		paths = append(paths, nonEmptyLines(staged)...)
		paths = append(paths, nonEmptyLines(untracked)...)
	}
	actual := make(map[string]bool, len(paths))
	for _, path := range paths {
		actual[filepath.ToSlash(path)] = true
	}
	expected := make(map[string]bool, len(slice63cAllowedPaths))
	for _, path := range slice63cAllowedPaths {
		expected[path] = true
		if !actual[path] {
			findings = append(findings, finding{path: path, reason: "missing from exact Slice 6.3c changed-path set"})
		}
	}
	for path := range actual {
		if !expected[path] {
			findings = append(findings, finding{path: path, reason: "outside exact Slice 6.3c changed-path allowlist"})
		}
	}
	return findings
}

func findSlice63cCommit(subject, parent string) (string, error) {
	parentFull, err := gitText("rev-parse", parent+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("cannot resolve expected parent %s", parent)
	}
	log, err := gitText("log", "--format=%H%x00%P%x00%s", "HEAD")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(log, "\n") {
		parts := strings.Split(line, "\x00")
		if len(parts) == 3 && parts[1] == parentFull && parts[2] == subject {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("missing commit with exact subject %q and parent %s", subject, parentFull)
}

func checkSlice63cPostGActiveState(active bool, head, gCommit, worktree string) []finding {
	if !active {
		return nil
	}
	var findings []finding
	if head != gCommit {
		findings = append(findings, finding{path: "git:G", reason: "after G exists on the active Slice 6.3c branch, HEAD must equal G exactly"})
	}
	if strings.TrimSpace(worktree) != "" {
		findings = append(findings, finding{path: "git:worktree", reason: "after G exists on the active Slice 6.3c branch, the nonignored worktree must be clean"})
	}
	return findings
}

func checkSlice63cPostGPolicySelfTest() []finding {
	if got := checkSlice63cPostGActiveState(true, "later", "g", ""); len(got) != 1 || got[0].path != "git:G" {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "post-G policy self-test did not reject an active-branch commit after G"}}
	}
	if got := checkSlice63cPostGActiveState(true, "g", "g", " M dirty.go"); len(got) != 1 || got[0].path != "git:worktree" {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "post-G policy self-test did not reject an active-branch dirty worktree"}}
	}
	if got := checkSlice63cPostGActiveState(false, "later", "g", " M unrelated.go"); len(got) != 0 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "post-G policy self-test rejected later merge/main history"}}
	}
	return nil
}

func checkSlice63cDocumentedSequence(gSubject string) []finding {
	const path = "docs/validation/architecture-maturity-slice-6.3c.md"
	data, err := os.ReadFile(path)
	if err != nil {
		return []finding{{path: path, reason: err.Error()}}
	}
	text := string(data)
	required := []string{
		"| T | `412a5ce` |",
		"| A | `43afad3` |",
		"| M | `5d9628c` |",
		"| W | `7787b49` |",
		"| G | pending |",
		gSubject,
		"L1-01 remains **partial**",
		"formal closure is deferred to Slice 6.3d",
	}
	var findings []finding
	for _, value := range required {
		if !strings.Contains(text, value) {
			findings = append(findings, finding{path: path, reason: "shallow checkout is missing documented commit/closure contract " + value})
		}
	}
	for _, allowed := range slice63cAllowedPaths {
		if !strings.Contains(text, "\n"+allowed+"\n") {
			findings = append(findings, finding{path: path, reason: "shallow checkout allowlist is missing " + allowed})
		}
	}
	return findings
}

func gitText(args ...string) (string, error) {
	command := exec.Command("git", args...)
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(output)), nil
}

func gitFields(args ...string) ([]string, error) {
	text, err := gitText(args...)
	if err != nil {
		return nil, err
	}
	return strings.Split(text, "\x00"), nil
}

func nonEmptyLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
