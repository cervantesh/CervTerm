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
	"docs/validation/architecture-maturity-slice-6.2a.md",
	"scripts/capture-parity-baseline.go",
	"docs/validation/architecture-maturity-slice-6.2b.md",
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
	filepath.ToSlash("internal/mux/mux.go"):                     "L3-01 preparatory facade; formal split target Slice 6.2d",
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
	findings = append(findings, checkSlice62aGuard()...)
	findings = append(findings, checkSlice62bGuard()...)
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

	wCommit, wErr := findMaturitySliceCommit(wSubject, mCommit)
	if wErr != nil {
		return append(findings, finding{path: "git:W", reason: wErr.Error()})
	}
	gCommit, _ := findMaturitySliceCommit(gSubject, wCommit)
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

func findMaturitySliceCommit(subject, parent string) (string, error) {
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

type slice62aControllerSpec struct {
	path       string
	controller string
	budgetName string
	budget     int
	maxMethods int
	ports      map[string]int
}

var slice62aAllowedPaths = []string{
	"docs/architecture-maturity/implementation-plan.md",
	"docs/architecture.md",
	"docs/validation/architecture-maturity-slice-6.2a.md",
	"docs/validation/architecture-maturity-slice-6.2a/benchmarks-base.txt",
	"docs/validation/architecture-maturity-slice-6.2a/benchmarks-candidate.txt",
	"docs/validation/architecture-maturity-slice-6.2a/gates.txt",
	"docs/validation/architecture-maturity-slice-6.2a/scope-and-commits.txt",
	"internal/mux/mux.go",
	"internal/mux/mux_kitty_test.go",
	"internal/mux/mux_session_ingress_test.go",
	"internal/mux/session_ingress_controller.go",
	"internal/mux/session_ingress_controller_test.go",
	"internal/mux/session_registry.go",
	"scripts/check-maturity-gates.go",
}

var slice62aController = slice62aControllerSpec{
	path:       "internal/mux/session_ingress_controller.go",
	controller: "sessionIngressController",
	budgetName: "sessionIngressControllerPortBudget",
	budget:     3,
	maxMethods: 2,
	ports: map[string]int{
		"sessionIngressOwnerPort": 1,
		"sessionIngressApplyPort": 2,
	},
}

var slice62aExactPortMethods = map[string][]string{
	"sessionIngressOwnerPort": {"acceptSessionIngress() bool"},
	"sessionIngressApplyPort": {
		"applySessionIngressData([]Event, []byte) []Event",
		"applySessionIngressEnd([]Event, error) []Event",
	},
}

const (
	slice62aExactConstructorSignature = "func[ownerPort sessionIngressOwnerPort, applyPort sessionIngressApplyPort]() sessionIngressController[ownerPort, applyPort]"
	slice62aExactRouteReceiver        = "sessionIngressController[ownerPort, applyPort]"
	slice62aExactRouteSignature       = "func(events []Event, owner ownerPort, apply applyPort, data []byte, end error) []Event"
)

func checkSlice62aGuard() []finding {
	var findings []finding
	findings = append(findings, checkSlice62aPostGPolicySelfTest()...)
	findings = append(findings, checkSlice62aCombinedShallowPostGSelfTest()...)
	findings = append(findings, checkSlice62aRouteAliasSelfTest()...)
	findings = append(findings, checkSlice62aDocumentedSequence()...)
	findings = append(findings, checkSlice62aController(slice62aController)...)
	findings = append(findings, checkSlice62aIngressSurface()...)
	findings = append(findings, checkSlice62aCommitsAndPaths()...)
	return findings
}

func checkSlice62aController(spec slice62aControllerSpec) []finding {
	data, err := os.ReadFile(spec.path)
	if err != nil {
		return []finding{{path: spec.path, reason: err.Error()}}
	}
	var findings []finding
	const expiry = "TODO(L3-01; expires Slice 6.2d): remove the preparatory facade adapter."
	if count := strings.Count(string(data), expiry); count != 1 {
		findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("must contain exactly one 6.2d facade-expiry TODO, found %d", count)})
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, spec.path, data, 0)
	if err != nil {
		return append(findings, finding{path: spec.path, reason: "cannot parse controller guard surface: " + err.Error()})
	}
	if len(file.Imports) != 0 {
		findings = append(findings, finding{path: spec.path, reason: "generic session-ingress controller must remain import-free"})
	}

	wantDeclarations := map[string]int{
		spec.budgetName:               1,
		"sessionIngressOwnerPort":     1,
		"sessionIngressApplyPort":     1,
		spec.controller:               1,
		"newSessionIngressController": 1,
		"method:route":                1,
	}
	declarations := make(map[string]int)
	portCount := 0
	methodInventory := make(map[string]int)
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			for _, item := range declaration.Specs {
				switch node := item.(type) {
				case *ast.ValueSpec:
					for index, name := range node.Names {
						declarations[name.Name]++
						if token.IsExported(name.Name) {
							findings = append(findings, finding{path: spec.path, reason: "controller declaration must remain private: " + name.Name})
						}
						if name.Name != spec.budgetName || index >= len(node.Values) {
							continue
						}
						literal, ok := node.Values[index].(*ast.BasicLit)
						value, parseErr := strconv.Atoi(strings.TrimSpace(literalValue(literal, ok)))
						if parseErr != nil || value != spec.budget {
							findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("%s must equal %d", spec.budgetName, spec.budget)})
						}
					}
				case *ast.TypeSpec:
					declarations[node.Name.Name]++
					if token.IsExported(node.Name.Name) {
						findings = append(findings, finding{path: spec.path, reason: "controller type must remain private: " + node.Name.Name})
					}
					if node.Name.Name == spec.controller {
						structure, ok := node.Type.(*ast.StructType)
						if !ok {
							findings = append(findings, finding{path: spec.path, reason: spec.controller + " must remain a private zero-field struct"})
						} else if len(structure.Fields.List) != 0 {
							findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("%s must retain no Mux or mutable state; got %d field declarations", spec.controller, len(structure.Fields.List))})
						}
						gotParams := renderedNamedFields(fset, node.TypeParams)
						wantParams := []string{"ownerPort:sessionIngressOwnerPort", "applyPort:sessionIngressApplyPort"}
						if strings.Join(gotParams, "|") != strings.Join(wantParams, "|") {
							findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("%s generic parameters=%v want=%v", spec.controller, gotParams, wantParams)})
						}
					}
					wantMethods, isPort := slice62aExactPortMethods[node.Name.Name]
					if !isPort {
						continue
					}
					port, ok := node.Type.(*ast.InterfaceType)
					if !ok {
						findings = append(findings, finding{path: spec.path, reason: node.Name.Name + " must remain a private interface"})
						continue
					}
					gotMethods := make([]string, 0, len(port.Methods.List))
					for _, method := range port.Methods.List {
						gotMethods = append(gotMethods, renderSlice62aInterfaceMethod(fset, method))
						function, ok := method.Type.(*ast.FuncType)
						if !ok || len(method.Names) != 1 || token.IsExported(method.Names[0].Name) {
							findings = append(findings, finding{path: spec.path, reason: node.Name.Name + " must contain only exact private methods"})
							continue
						}
						for _, list := range []*ast.FieldList{function.Params, function.Results} {
							for _, typeText := range renderedUnnamedFields(fset, list) {
								findings = append(findings, forbiddenSlice62aType(spec.path, node.Name.Name, typeText)...)
							}
						}
					}
					if strings.Join(gotMethods, "|") != strings.Join(wantMethods, "|") {
						findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("%s methods=%v want exact %v", node.Name.Name, gotMethods, wantMethods)})
					}
					if len(gotMethods) == 0 || len(gotMethods) > spec.maxMethods || len(gotMethods) > 5 {
						findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("%s methods=%d must be nonzero, <=%d and <=5", node.Name.Name, len(gotMethods), spec.maxMethods)})
					}
					portCount += len(gotMethods)
				}
			}
		case *ast.FuncDecl:
			if token.IsExported(declaration.Name.Name) {
				findings = append(findings, finding{path: spec.path, reason: "controller function must remain private: " + declaration.Name.Name})
			}
			if declaration.Recv == nil {
				declarations[declaration.Name.Name]++
				if declaration.Name.Name == "newSessionIngressController" && renderSlice62aNode(fset, declaration.Type) != slice62aExactConstructorSignature {
					findings = append(findings, finding{path: spec.path, reason: "constructor signature changed: " + renderSlice62aNode(fset, declaration.Type)})
				}
				continue
			}
			key := "method:" + declaration.Name.Name
			declarations[key]++
			methodInventory[declaration.Name.Name]++
			if declaration.Name.Name != "route" {
				continue
			}
			gotReceiver := strings.Join(renderedUnnamedFields(fset, declaration.Recv), "|")
			gotSignature := renderSlice62aNode(fset, declaration.Type)
			if gotReceiver != slice62aExactRouteReceiver || gotSignature != slice62aExactRouteSignature {
				findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("route contract receiver=%q signature=%q", gotReceiver, gotSignature)})
			}
			var phaseCalls []string
			ast.Inspect(declaration.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if ok && (selector.Sel.Name == "acceptSessionIngress" || selector.Sel.Name == "applySessionIngressData" || selector.Sel.Name == "applySessionIngressEnd") {
					phaseCalls = append(phaseCalls, selector.Sel.Name)
				}
				return true
			})
			wantOrder := []string{"acceptSessionIngress", "applySessionIngressData", "applySessionIngressEnd"}
			if strings.Join(phaseCalls, "|") != strings.Join(wantOrder, "|") {
				findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("route phase source order=%v want accept->data->end", phaseCalls)})
			}
		}
	}
	if !mapsEqualStringInt(declarations, wantDeclarations) {
		findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("controller declarations=%v want exact %v", declarations, wantDeclarations)})
	}
	if len(methodInventory) != 1 || methodInventory["route"] != 1 {
		findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("controller method inventory=%v want exactly route", methodInventory)})
	}
	if portCount != spec.budget {
		findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("aggregate port methods=%d budget=%d", portCount, spec.budget)})
	}
	return findings
}

func renderSlice62aNode(fset *token.FileSet, node any) string {
	var rendered bytes.Buffer
	_ = format.Node(&rendered, fset, node)
	return rendered.String()
}

func renderSlice62aInterfaceMethod(fset *token.FileSet, method *ast.Field) string {
	if len(method.Names) != 1 {
		return renderSlice62aNode(fset, method)
	}
	return method.Names[0].Name + strings.TrimPrefix(renderSlice62aNode(fset, method.Type), "func")
}

func mapsEqualStringInt(got, want map[string]int) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}

func renderedNamedFields(fset *token.FileSet, fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}
	var rendered []string
	for _, field := range fields.List {
		var typeText bytes.Buffer
		_ = format.Node(&typeText, fset, field.Type)
		for _, name := range field.Names {
			rendered = append(rendered, name.Name+":"+typeText.String())
		}
	}
	return rendered
}

func renderedUnnamedFields(fset *token.FileSet, fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}
	var rendered []string
	for _, field := range fields.List {
		var typeText bytes.Buffer
		_ = format.Node(&typeText, fset, field.Type)
		rendered = append(rendered, typeText.String())
	}
	return rendered
}

func forbiddenSlice62aType(path, owner, typeText string) []finding {
	lower := strings.ToLower(typeText)
	for _, forbidden := range []string{"*mux", "*localsessionregistry", "*pane", "map[", "func(", "chan ", "interface{}", "any"} {
		if strings.Contains(lower, forbidden) {
			return []finding{{path: path, reason: owner + " has forbidden retained-owner/state type " + typeText}}
		}
	}
	return nil
}

type slice62aRouteCall struct {
	path        string
	declaration *ast.FuncDecl
	call        *ast.CallExpr
	receiver    ast.Expr
}

func checkSlice62aIngressSurface() []finding {
	const root = "internal/mux"
	fset := token.NewFileSet()
	var findings []finding
	muxControllerFields := 0
	entries, err := os.ReadDir(root)
	if err != nil {
		return []finding{{path: root, reason: err.Error()}}
	}
	files := make(map[string]*ast.File)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			findings = append(findings, finding{path: filepath.ToSlash(path), reason: parseErr.Error()})
			continue
		}
		files[filepath.ToSlash(path)] = file
	}
	for path, file := range files {
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				for _, item := range declaration.Specs {
					typeSpec, ok := item.(*ast.TypeSpec)
					if ok && token.IsExported(typeSpec.Name.Name) && strings.Contains(strings.ToLower(typeSpec.Name.Name), "ingress") {
						findings = append(findings, finding{path: path, reason: "exported ingress type bypass " + typeSpec.Name.Name})
					}
					if !ok || typeSpec.Name.Name != "Mux" {
						continue
					}
					structure, ok := typeSpec.Type.(*ast.StructType)
					if !ok {
						continue
					}
					for _, field := range structure.Fields.List {
						for _, name := range field.Names {
							if name.Name != "sessionIngress" {
								continue
							}
							muxControllerFields++
							want := "sessionIngressController[sessionIngressRecordAdapter, muxSessionIngressOperationAdapter]"
							if got := renderSlice62aNode(fset, field.Type); got != want {
								findings = append(findings, finding{path: path, reason: "Mux.sessionIngress type=" + got + " want " + want})
							}
						}
					}
				}
			case *ast.FuncDecl:
				if token.IsExported(declaration.Name.Name) && strings.Contains(strings.ToLower(declaration.Name.Name), "ingress") {
					findings = append(findings, finding{path: path, reason: "exported ingress function/method bypass " + declaration.Name.Name})
				}
				if declaration.Body == nil {
					continue
				}
				ast.Inspect(declaration.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					selector, ok := call.Fun.(*ast.SelectorExpr)
					if ok && selector.Sel.Name == "adaptSessionIngressRecord" && (declaration.Name.Name != "Drain" || !receiverNamed(declaration.Recv, "Mux")) {
						findings = append(findings, finding{path: path, reason: "session-ingress owner adaptation bypass outside (*Mux).Drain"})
					}
					return true
				})
			}
		}
	}
	findings = append(findings, checkSlice62aProductionControllerMethodInventory(files)...)
	findings = append(findings, checkSlice62aRouteExclusivity(files)...)
	if muxControllerFields != 1 {
		findings = append(findings, finding{path: "internal/mux/mux.go", reason: fmt.Sprintf("Mux session-ingress controller fields=%d want=1", muxControllerFields)})
	}
	return findings
}

func slice62aControllerTypeNames(files map[string]*ast.File) map[string]bool {
	typeNames := map[string]bool{"sessionIngressController": true}
	for {
		changed := false
		for _, file := range files {
			for _, declaration := range file.Decls {
				generic, ok := declaration.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, item := range generic.Specs {
					typeSpec, ok := item.(*ast.TypeSpec)
					if ok && !typeNames[typeSpec.Name.Name] && slice62aControllerTypeExpression(typeSpec.Type, typeNames) {
						typeNames[typeSpec.Name.Name] = true
						changed = true
					}
				}
			}
		}
		if !changed {
			break
		}
	}
	return typeNames
}

func checkSlice62aProductionControllerMethodInventory(files map[string]*ast.File) []finding {
	typeNames := slice62aControllerTypeNames(files)
	methodCount := 0
	var findings []finding
	for path, file := range files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List) != 1 || !slice62aControllerTypeExpression(function.Recv.List[0].Type, typeNames) {
				continue
			}
			methodCount++
			if path != "internal/mux/session_ingress_controller.go" || function.Name.Name != "route" {
				findings = append(findings, finding{path: path, reason: "sessionIngressController production method inventory permits only route in session_ingress_controller.go"})
			}
		}
	}
	if methodCount != 1 {
		findings = append(findings, finding{path: "internal/mux/session_ingress_controller.go", reason: fmt.Sprintf("production sessionIngressController methods=%d want exactly route", methodCount)})
	}
	return findings
}

func checkSlice62aRouteExclusivity(files map[string]*ast.File) []finding {
	var calls []slice62aRouteCall
	for path, file := range files {
		for _, declaration := range file.Decls {
			function, _ := declaration.(*ast.FuncDecl)
			var root ast.Node = declaration
			if function != nil {
				if function.Body == nil {
					continue
				}
				root = function.Body
			}
			ast.Inspect(root, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if ok && selector.Sel.Name == "route" {
					calls = append(calls, slice62aRouteCall{path: path, declaration: function, call: call, receiver: selector.X})
				}
				return true
			})
		}
	}
	var findings []finding
	for _, route := range calls {
		if !slice62aCanonicalRouteCall(route) {
			findings = append(findings, finding{path: route.path, reason: "private production mux selector method name route is reserved through Slice 6.2d; only the exact m.sessionIngress.route call inside (*Mux).Drain in mux.go is permitted"})
		}
	}
	if len(calls) != 1 {
		findings = append(findings, finding{path: "internal/mux/mux.go", reason: fmt.Sprintf("production mux selector route calls=%d want exactly 1 exact m.sessionIngress.route call in (*Mux).Drain", len(calls))})
	}
	return findings
}

func slice62aControllerTypeExpression(expression ast.Expr, names map[string]bool) bool {
	switch expression := expression.(type) {
	case nil:
		return false
	case *ast.Ident:
		return names[expression.Name]
	case *ast.IndexExpr:
		return slice62aControllerTypeExpression(expression.X, names)
	case *ast.IndexListExpr:
		return slice62aControllerTypeExpression(expression.X, names)
	case *ast.ParenExpr:
		return slice62aControllerTypeExpression(expression.X, names)
	case *ast.StarExpr:
		return slice62aControllerTypeExpression(expression.X, names)
	}
	return false
}

func slice62aCanonicalRouteCall(route slice62aRouteCall) bool {
	if route.declaration == nil || filepath.ToSlash(route.path) != "internal/mux/mux.go" || route.declaration.Name.Name != "Drain" {
		return false
	}
	if route.declaration.Recv == nil || len(route.declaration.Recv.List) != 1 || len(route.declaration.Recv.List[0].Names) != 1 || route.declaration.Recv.List[0].Names[0].Name != "m" {
		return false
	}
	pointer, ok := route.declaration.Recv.List[0].Type.(*ast.StarExpr)
	if !ok || !slice62aIdentifierNamed(pointer.X, "Mux") {
		return false
	}
	controller, ok := route.receiver.(*ast.SelectorExpr)
	if !ok || !slice62aIdentifierNamed(controller.X, "m") || controller.Sel.Name != "sessionIngress" {
		return false
	}
	args := route.call.Args
	return len(args) == 5 && slice62aIdentifierNamed(args[0], "events") && slice62aIdentifierNamed(args[1], "accepted") &&
		slice62aIdentifierNamed(args[2], "operation") && slice62aSelectorNamed(args[3], "record", "data") && slice62aSelectorNamed(args[4], "record", "err")
}

func slice62aIdentifierNamed(expression ast.Expr, name string) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Name == name
}

func slice62aSelectorNamed(expression ast.Expr, owner, field string) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	return ok && slice62aIdentifierNamed(selector.X, owner) && selector.Sel.Name == field
}

func checkSlice62aRouteAliasSelfTest() []finding {
	fixtures := []struct {
		name   string
		source string
	}{
		{
			name: "local controller alias",
			source: `package mux
				type Mux struct { sessionIngress int }
				func (m *Mux) Drain() {
					var events, accepted, operation, record any
					m.sessionIngress.route(events, accepted, operation, record.data, record.err)
					controller := m.sessionIngress
					controller.route(events, accepted, operation, record.data, record.err)
				}`,
		},
		{
			name: "transitive global controller aliases",
			source: `package mux
				type sessionIngressController[T any] struct{}
				type Mux struct { sessionIngress sessionIngressController[int] }
				var routeRoot = sessionIngressController[int]{}
				var routeAlias = routeRoot
				var transitiveRouteAlias = routeAlias
				var globalAliasBypass = transitiveRouteAlias.route(nil, nil, nil, nil, nil)
				func (m *Mux) Drain() {
					var events, accepted, operation, record any
					m.sessionIngress.route(events, accepted, operation, record.data, record.err)
				}`,
		},
		{
			name: "controller stored under another struct field",
			source: `package mux
				type sessionIngressController[T any] struct{}
				type Mux struct { sessionIngress sessionIngressController[int] }
				type routeHolder struct { controller sessionIngressController[int] }
				var alternate routeHolder
				func (m *Mux) Drain() {
					var events, accepted, operation, record any
					m.sessionIngress.route(events, accepted, operation, record.data, record.err)
				}
				func storedFieldBypass(events, accepted, operation, record any) {
					alternate.controller.route(events, accepted, operation, record.data, record.err)
				}`,
		},
	}
	var findings []finding
	for _, fixture := range fixtures {
		file, err := parser.ParseFile(token.NewFileSet(), "internal/mux/mux.go", fixture.source, 0)
		if err != nil {
			findings = append(findings, finding{path: "scripts/check-maturity-gates.go", reason: "cannot parse Slice 6.2a route-exclusivity self-test " + fixture.name + ": " + err.Error()})
			continue
		}
		got := checkSlice62aRouteExclusivity(map[string]*ast.File{"internal/mux/mux.go": file})
		rejected := false
		for _, item := range got {
			if strings.Contains(item.reason, "selector method name route is reserved") {
				rejected = true
				break
			}
		}
		if !rejected {
			findings = append(findings, finding{path: "scripts/check-maturity-gates.go", reason: "route-exclusivity self-test did not reject " + fixture.name})
		}
	}
	return findings
}

func receiverNamed(fields *ast.FieldList, name string) bool {
	if fields == nil || len(fields.List) != 1 {
		return false
	}
	typeExpr := fields.List[0].Type
	if pointer, ok := typeExpr.(*ast.StarExpr); ok {
		typeExpr = pointer.X
	}
	identifier, ok := typeExpr.(*ast.Ident)
	return ok && identifier.Name == name
}

func checkSlice62aCommitsAndPaths() []finding {
	const (
		base        = "c2a0137c50c099ce28014a7582eac3ee6a4340f1"
		tCommit     = "271325ba47efe7e948f58fa314475f37e1176bc3"
		aCommit     = "45514c813494351e7608b858aa6ab35e7643d33a"
		mCommit     = "66072c984f6df723412cadb0cc43a8fb147c255e"
		wCommit     = "aa1bfd4a830cd7fc88e7f6e25288ed4eb3879ae2"
		gSubject    = "refactor(mux): guard session ingress controller delegation"
		sliceBranch = "arch/l3-01a-mux-session-ingress"
	)
	stages := []struct {
		class, commit, parent, subject string
	}{
		{"T", tCommit, base, "test(mux): characterize session ingress ordering"},
		{"A", aCommit, tCommit, "refactor(mux): add session ingress controller seam"},
		{"M", mCommit, aCommit, "refactor(mux): split session ingress adapters"},
		{"W", wCommit, mCommit, "refactor(mux): wire session ingress controller"},
	}
	branch, _ := gitText("symbolic-ref", "--quiet", "--short", "HEAD")
	head, _ := gitText("rev-parse", "HEAD")
	worktree, _ := gitText("status", "--porcelain=v1", "--untracked-files=all")
	if shallow, _ := gitText("rev-parse", "--is-shallow-repository"); shallow == "true" {
		missingHistory := false
		for _, stage := range append([]struct{ class, commit, parent, subject string }{{class: "base", commit: base}}, stages...) {
			if _, err := gitText("cat-file", "-e", stage.commit+"^{commit}"); err != nil {
				missingHistory = true
				break
			}
		}
		if missingHistory {
			gCommit, _ := findMaturitySliceCommitBySubject(gSubject)
			return checkSlice62aShallowFallback(branch == sliceBranch, head, wCommit, gCommit, worktree)
		}
	}
	var findings []finding
	for _, stage := range stages {
		identity, err := gitFields("show", "-s", "--format=%H%x00%P%x00%s", stage.commit)
		if err != nil || len(identity) != 3 {
			findings = append(findings, finding{path: "git:" + stage.class, reason: "missing Slice 6.2a commit " + stage.commit})
			continue
		}
		parent, parentErr := gitText("rev-parse", stage.parent+"^{commit}")
		if parentErr != nil || identity[0] != stage.commit || identity[1] != parent || identity[2] != stage.subject {
			findings = append(findings, finding{path: "git:" + stage.class, reason: fmt.Sprintf("unexpected identity/parent/subject for %s: parent=%s subject=%q", identity[0], identity[1], identity[2])})
		}
	}
	gCommit, _ := findMaturitySliceCommit(gSubject, wCommit)
	end := gCommit
	includeWorktree := false
	if gCommit == "" {
		if head != wCommit {
			findings = append(findings, finding{path: "git:G", reason: "before Slice 6.2a G, HEAD must be the exact immutable W commit"})
			return findings
		}
		end = wCommit
		includeWorktree = true
	} else {
		findings = append(findings, checkSlice62aPostGActiveState(branch == sliceBranch, head, gCommit, worktree)...)
	}
	findings = append(findings, checkSlice62aExactPaths(base, end, includeWorktree)...)
	return findings
}

func findMaturitySliceCommitBySubject(subject string) (string, error) {
	log, err := gitText("log", "--format=%H%x00%s", "HEAD")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(log, "\n") {
		parts := strings.Split(line, "\x00")
		if len(parts) == 2 && parts[1] == subject {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("missing commit with exact subject %q", subject)
}

func checkSlice62aExactPaths(base, end string, includeWorktree bool) []finding {
	pathsText, err := gitText("diff", "--name-only", base+".."+end)
	if err != nil {
		return []finding{{path: "git:paths", reason: err.Error()}}
	}
	paths := nonEmptyLines(pathsText)
	if includeWorktree {
		for _, args := range [][]string{{"diff", "--name-only"}, {"diff", "--cached", "--name-only"}, {"ls-files", "--others", "--exclude-standard"}} {
			text, _ := gitText(args...)
			paths = append(paths, nonEmptyLines(text)...)
		}
	}
	actual := make(map[string]bool, len(paths))
	for _, path := range paths {
		actual[filepath.ToSlash(path)] = true
	}
	expected := make(map[string]bool, len(slice62aAllowedPaths))
	var findings []finding
	for _, path := range slice62aAllowedPaths {
		expected[path] = true
		if !actual[path] {
			findings = append(findings, finding{path: path, reason: "missing from exact Slice 6.2a changed-path set"})
		}
	}
	for path := range actual {
		if !expected[path] {
			findings = append(findings, finding{path: path, reason: "outside exact Slice 6.2a changed-path allowlist"})
		}
	}
	return findings
}

func checkSlice62aPostGActiveState(active bool, head, gCommit, worktree string) []finding {
	if !active {
		return nil
	}
	var findings []finding
	if head != gCommit {
		findings = append(findings, finding{path: "git:G", reason: "after G exists on the active Slice 6.2a branch, HEAD must equal G exactly"})
	}
	if strings.TrimSpace(worktree) != "" {
		findings = append(findings, finding{path: "git:worktree", reason: "after G exists on the active Slice 6.2a branch, the nonignored worktree must be clean"})
	}
	return findings
}

func checkSlice62aPostGPolicySelfTest() []finding {
	if got := checkSlice62aPostGActiveState(true, "later", "g", ""); len(got) != 1 || got[0].path != "git:G" {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2a post-G policy self-test did not reject an active-branch commit after G"}}
	}
	if got := checkSlice62aPostGActiveState(true, "g", "g", " M dirty.go"); len(got) != 1 || got[0].path != "git:worktree" {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2a post-G policy self-test did not reject an active-branch dirty worktree"}}
	}
	if got := checkSlice62aPostGActiveState(false, "later", "g", " M unrelated.go"); len(got) != 0 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2a post-G policy self-test rejected later merge/main history"}}
	}
	return nil
}

func slice62aDocumentRequirements() []string {
	return []string{
		"| T | `271325b` |",
		"| A | `45514c8` |",
		"| M | `66072c9` |",
		"| W | `aa1bfd4` |",
		"| G | pending |",
		"refactor(mux): guard session ingress controller delegation",
		"L3-01 remains **partial**",
		"formal closure is deferred to Slice 6.2d",
	}
}

func slice62aDocumentFindings(text string) []finding {
	const path = "docs/validation/architecture-maturity-slice-6.2a.md"
	var findings []finding
	for _, value := range slice62aDocumentRequirements() {
		if !strings.Contains(text, value) {
			findings = append(findings, finding{path: path, reason: "shallow checkout is missing documented commit/closure contract " + value})
		}
	}
	for _, allowed := range slice62aAllowedPaths {
		if !strings.Contains(text, "\n"+allowed+"\n") {
			findings = append(findings, finding{path: path, reason: "shallow checkout allowlist is missing " + allowed})
		}
	}
	return findings
}

func checkSlice62aDocumentedSequence() []finding {
	const path = "docs/validation/architecture-maturity-slice-6.2a.md"
	data, err := os.ReadFile(path)
	if err != nil {
		return []finding{{path: path, reason: err.Error()}}
	}
	return slice62aDocumentFindings(string(data))
}

func checkSlice62aShallowFallback(active bool, head, wCommit, gCommit, worktree string) []finding {
	const path = "docs/validation/architecture-maturity-slice-6.2a.md"
	data, err := os.ReadFile(path)
	if err != nil {
		return []finding{{path: path, reason: err.Error()}}
	}
	return slice62aShallowFallbackFindings(string(data), active, head, wCommit, gCommit, worktree)
}

func slice62aShallowFallbackFindings(document string, active bool, head, wCommit, gCommit, worktree string) []finding {
	findings := slice62aDocumentFindings(document)
	if !active {
		return findings
	}
	if gCommit != "" {
		return append(findings, checkSlice62aPostGActiveState(true, head, gCommit, worktree)...)
	}
	if head != wCommit {
		findings = append(findings, finding{path: "git:G", reason: "history-limited active Slice 6.2a branch without identifiable G must remain at immutable W"})
	}
	return findings
}

func checkSlice62aCombinedShallowPostGSelfTest() []finding {
	valid := "\n" + strings.Join(slice62aDocumentRequirements(), "\n") + "\n" + strings.Join(slice62aAllowedPaths, "\n") + "\n"
	if got := slice62aDocumentFindings(valid); len(got) != 0 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2a shallow-doc self-test rejected a complete synthetic contract"}}
	}
	missingContract := strings.Replace(valid, slice62aDocumentRequirements()[0], "", 1)
	if got := slice62aDocumentFindings(missingContract); len(got) != 1 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2a shallow-doc self-test did not reject one missing commit contract"}}
	}
	missingPath := strings.Replace(valid, "\n"+slice62aAllowedPaths[0]+"\n", "\n", 1)
	if got := slice62aDocumentFindings(missingPath); len(got) != 1 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2a shallow-doc self-test did not reject one missing allowlist path"}}
	}
	postG := slice62aShallowFallbackFindings(valid, true, "later", "w", "g", " M dirty.go")
	if len(postG) != 2 || postG[0].path != "git:G" || postG[1].path != "git:worktree" {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "combined shallow/post-G self-test did not enforce active-branch HEAD and clean-worktree policy"}}
	}
	if got := slice62aShallowFallbackFindings(valid, false, "later-main", "w", "g", " M unrelated.go"); len(got) != 0 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "combined shallow/post-G self-test rejected later main history"}}
	}
	if got := slice62aShallowFallbackFindings(valid, true, "later", "w", "", ""); len(got) != 1 || got[0].path != "git:G" {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "combined shallow/pre-G self-test did not reject an unidentified active-branch commit beyond W"}}
	}
	return nil
}

type slice62bControllerSpec struct {
	path       string
	controller string
	budgetName string
	budget     int
	maxMethods int
	ports      map[string][]string
}

var slice62bAllowedPaths = []string{
	"docs/architecture-maturity/implementation-plan.md",
	"docs/architecture.md",
	"docs/validation/architecture-maturity-slice-6.2b.md",
	"docs/validation/architecture-maturity-slice-6.2b/benchmarks-base.txt",
	"docs/validation/architecture-maturity-slice-6.2b/benchmarks-candidate.txt",
	"docs/validation/architecture-maturity-slice-6.2b/gates.txt",
	"docs/validation/architecture-maturity-slice-6.2b/scope-and-commits.txt",
	"internal/mux/mux.go",
	"internal/mux/mux_iterm.go",
	"internal/mux/mux_kitty.go",
	"internal/mux/mux_protocol_scheduling_test.go",
	"internal/mux/mux_sixel.go",
	"internal/mux/protocol_scheduling_controller.go",
	"internal/mux/protocol_scheduling_controller_test.go",
	"scripts/check-maturity-gates.go",
}

var slice62bController = slice62bControllerSpec{
	path:       "internal/mux/protocol_scheduling_controller.go",
	controller: "protocolSchedulingController",
	budgetName: "protocolSchedulingControllerPortBudget",
	budget:     5,
	maxMethods: 3,
	ports: map[string][]string{
		"protocolSchedulingDispatchPort": {
			"dispatchKitty([]Event) []Event",
			"dispatchSixel()",
			"dispatchITerm()",
		},
		"protocolSchedulingApplyPort": {
			"applyExpiry([]Event) []Event",
			"applyCompletion([]Event) []Event",
		},
	},
}

var slice62bExactMethodSignatures = map[string]string{
	"dispatchKitty":   "func(events []Event, port dispatchPort) []Event",
	"dispatchSixel":   "func(port dispatchPort)",
	"dispatchITerm":   "func(port dispatchPort)",
	"applyExpiry":     "func(events []Event, port applyPort) []Event",
	"applyCompletion": "func(events []Event, port applyPort) []Event",
}

var slice62bExactMethodBodies = map[string]string{
	"dispatchKitty":   "{\n\treturn port.dispatchKitty(events)\n}",
	"dispatchSixel":   "{\n\tport.dispatchSixel()\n}",
	"dispatchITerm":   "{\n\tport.dispatchITerm()\n}",
	"applyExpiry":     "{\n\treturn port.applyExpiry(events)\n}",
	"applyCompletion": "{\n\treturn port.applyCompletion(events)\n}",
}

type slice62bShimSpec struct {
	path string
	body string
}

var slice62bExactShims = map[string]slice62bShimSpec{
	"processKittyOutcomes": {
		path: "internal/mux/mux_kitty.go",
		body: "{\n\treturn m.protocolScheduling.dispatchKitty(nil, muxProtocolSchedulingDispatchOperationAdapter{mux: m, pane: p})\n}",
	},
	"processSixelOutcomes": {
		path: "internal/mux/mux_sixel.go",
		body: "{\n\tm.protocolScheduling.dispatchSixel(muxProtocolSchedulingDispatchOperationAdapter{mux: m, pane: p})\n}",
	},
	"processITermOutcomes": {
		path: "internal/mux/mux_iterm.go",
		body: "{\n\tm.protocolScheduling.dispatchITerm(muxProtocolSchedulingDispatchOperationAdapter{mux: m, pane: p})\n}",
	},
	"expireImages": {
		path: "internal/mux/mux_kitty.go",
		body: "{\n\treturn m.protocolScheduling.applyExpiry(nil, muxProtocolSchedulingApplyOperationAdapter{mux: m, now: now})\n}",
	},
	"applyImageCompletion": {
		path: "internal/mux/mux_kitty.go",
		body: "{\n\treturn m.protocolScheduling.applyCompletion(nil, muxProtocolSchedulingApplyOperationAdapter{mux: m, completion: completion})\n}",
	},
}

func checkSlice62bGuard() []finding {
	var findings []finding
	findings = append(findings, checkSlice62bPostGPolicySelfTest()...)
	findings = append(findings, checkSlice62bCombinedShallowPostGSelfTest()...)
	findings = append(findings, checkSlice62bBypassSelfTest()...)
	findings = append(findings, checkSlice62bDocumentedSequence()...)
	findings = append(findings, checkSlice62bController(slice62bController)...)
	findings = append(findings, checkSlice62bProductionSurface()...)
	findings = append(findings, checkSlice62bCallerOrder()...)
	findings = append(findings, checkSlice62bKnownDefects()...)
	findings = append(findings, checkSlice62bCommitsAndPaths()...)
	return findings
}

func checkSlice62bController(spec slice62bControllerSpec) []finding {
	data, err := os.ReadFile(spec.path)
	if err != nil {
		return []finding{{path: spec.path, reason: err.Error()}}
	}
	var findings []finding
	const expiry = "TODO(L3-01; expires Slice 6.2d): remove the preparatory facade adapter."
	if count := strings.Count(string(data), expiry); count != 1 {
		findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("must contain exactly one L3-01 Slice 6.2d facade-expiry TODO, found %d", count)})
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, spec.path, data, 0)
	if err != nil {
		return append(findings, finding{path: spec.path, reason: "cannot parse protocol-scheduling controller: " + err.Error()})
	}
	if len(file.Imports) != 0 {
		findings = append(findings, finding{path: spec.path, reason: "generic protocol-scheduling controller must remain import-free"})
	}
	wantDeclarations := map[string]int{
		spec.budgetName: 1, "protocolSchedulingDispatchPort": 1, "protocolSchedulingApplyPort": 1,
		spec.controller: 1, "newProtocolSchedulingController": 1,
		"method:dispatchKitty": 1, "method:dispatchSixel": 1, "method:dispatchITerm": 1,
		"method:applyExpiry": 1, "method:applyCompletion": 1,
	}
	declarations := make(map[string]int)
	methodInventory := make(map[string]int)
	portCount := 0
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			for _, item := range declaration.Specs {
				switch node := item.(type) {
				case *ast.ValueSpec:
					for index, name := range node.Names {
						declarations[name.Name]++
						if token.IsExported(name.Name) {
							findings = append(findings, finding{path: spec.path, reason: "controller declaration must remain private: " + name.Name})
						}
						if name.Name == spec.budgetName && index < len(node.Values) {
							literal, ok := node.Values[index].(*ast.BasicLit)
							value, parseErr := strconv.Atoi(strings.TrimSpace(literalValue(literal, ok)))
							if parseErr != nil || value != spec.budget {
								findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("%s must equal %d", spec.budgetName, spec.budget)})
							}
						}
					}
				case *ast.TypeSpec:
					declarations[node.Name.Name]++
					if token.IsExported(node.Name.Name) {
						findings = append(findings, finding{path: spec.path, reason: "controller type must remain private: " + node.Name.Name})
					}
					if node.Name.Name == spec.controller {
						structure, ok := node.Type.(*ast.StructType)
						if !ok || len(structure.Fields.List) != 0 {
							findings = append(findings, finding{path: spec.path, reason: spec.controller + " must remain a private zero-field struct"})
						}
						got := renderedNamedFields(fset, node.TypeParams)
						want := []string{"dispatchPort:protocolSchedulingDispatchPort", "applyPort:protocolSchedulingApplyPort"}
						if strings.Join(got, "|") != strings.Join(want, "|") {
							findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("%s generic parameters=%v want exact %v", spec.controller, got, want)})
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
					gotMethods := make([]string, 0, len(port.Methods.List))
					for _, method := range port.Methods.List {
						gotMethods = append(gotMethods, renderSlice62aInterfaceMethod(fset, method))
						function, ok := method.Type.(*ast.FuncType)
						if !ok || len(method.Names) != 1 || token.IsExported(method.Names[0].Name) {
							findings = append(findings, finding{path: spec.path, reason: node.Name.Name + " must contain only exact private methods"})
							continue
						}
						for _, list := range []*ast.FieldList{function.Params, function.Results} {
							for _, typeText := range renderedUnnamedFields(fset, list) {
								findings = append(findings, forbiddenSlice62bType(spec.path, node.Name.Name, typeText)...)
							}
						}
					}
					if strings.Join(gotMethods, "|") != strings.Join(wantMethods, "|") {
						findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("%s methods=%v want exact %v", node.Name.Name, gotMethods, wantMethods)})
					}
					if len(gotMethods) == 0 || len(gotMethods) > spec.maxMethods || len(gotMethods) > 5 {
						findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("%s methods=%d must be nonzero, <=%d and <=5", node.Name.Name, len(gotMethods), spec.maxMethods)})
					}
					portCount += len(gotMethods)
				}
			}
		case *ast.FuncDecl:
			if token.IsExported(declaration.Name.Name) {
				findings = append(findings, finding{path: spec.path, reason: "controller function must remain private: " + declaration.Name.Name})
			}
			if declaration.Recv == nil {
				declarations[declaration.Name.Name]++
				if declaration.Name.Name == "newProtocolSchedulingController" {
					want := "func[dispatchPort protocolSchedulingDispatchPort, applyPort protocolSchedulingApplyPort]() protocolSchedulingController[dispatchPort, applyPort]"
					if got := renderSlice62aNode(fset, declaration.Type); compactSlice62bGoText(got) != compactSlice62bGoText(want) {
						findings = append(findings, finding{path: spec.path, reason: "constructor signature changed: " + got})
					}
					wantBody := "{\n\treturn protocolSchedulingController[dispatchPort, applyPort]{}\n}"
					if got := renderSlice62aNode(fset, declaration.Body); got != wantBody {
						findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("constructor body=%q want exact zero-value body", got)})
					}
				}
				continue
			}
			key := "method:" + declaration.Name.Name
			declarations[key]++
			methodInventory[declaration.Name.Name]++
			wantSignature, known := slice62bExactMethodSignatures[declaration.Name.Name]
			if !known {
				continue
			}
			gotReceiver := strings.Join(renderedUnnamedFields(fset, declaration.Recv), "|")
			if gotReceiver != "protocolSchedulingController[dispatchPort, applyPort]" || renderSlice62aNode(fset, declaration.Type) != wantSignature {
				findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("%s receiver/signature changed", declaration.Name.Name)})
			}
			if got := renderSlice62aNode(fset, declaration.Body); got != slice62bExactMethodBodies[declaration.Name.Name] {
				findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("%s body=%q want exact one-call delegation", declaration.Name.Name, got)})
			}
		}
	}
	if !mapsEqualStringInt(declarations, wantDeclarations) {
		findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("controller declarations=%v want exact %v", declarations, wantDeclarations)})
	}
	if len(methodInventory) != 5 {
		findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("controller method inventory=%v want exact five methods", methodInventory)})
	}
	for name := range slice62bExactMethodSignatures {
		if methodInventory[name] != 1 {
			findings = append(findings, finding{path: spec.path, reason: "controller method inventory missing/duplicates " + name})
		}
	}
	if portCount != spec.budget {
		findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("aggregate port methods=%d budget=%d", portCount, spec.budget)})
	}
	return findings
}

func compactSlice62bGoText(text string) string {
	return strings.ReplaceAll(strings.Join(strings.Fields(text), ""), ",]", "]")
}

func forbiddenSlice62bType(path, owner, typeText string) []finding {
	lower := strings.ToLower(typeText)
	for _, forbidden := range []string{"*mux", "*pane", "*imagedecodescheduler", "replyslot", "map[", "func(", "chan ", "interface{}", "any"} {
		if strings.Contains(lower, forbidden) {
			return []finding{{path: path, reason: owner + " has forbidden retained-owner/state type " + typeText}}
		}
	}
	return nil
}

func slice62bExpandRetainedTypeNames(files map[string]*ast.File, names map[string]bool) map[string]bool {
	for {
		changed := false
		for _, file := range files {
			for _, declaration := range file.Decls {
				generic, ok := declaration.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, item := range generic.Specs {
					typeSpec, ok := item.(*ast.TypeSpec)
					if !ok || names[typeSpec.Name.Name] || typeSpec.Name.Name == "Mux" {
						continue
					}
					if slice62bTypeExpressionRetains(typeSpec.Type, names) {
						names[typeSpec.Name.Name] = true
						changed = true
					}
				}
			}
		}
		if !changed {
			return names
		}
	}
}

func slice62bControllerTypeNames(files map[string]*ast.File) map[string]bool {
	return slice62bExpandRetainedTypeNames(files, map[string]bool{"protocolSchedulingController": true})
}

func slice62bRetainedTypeNames(files map[string]*ast.File) map[string]bool {
	return slice62bExpandRetainedTypeNames(files, map[string]bool{
		"protocolSchedulingController":                  true,
		"muxProtocolSchedulingDispatchOperationAdapter": true,
		"muxProtocolSchedulingApplyOperationAdapter":    true,
	})
}

func slice62bTypeExpressionRetains(expression ast.Expr, names map[string]bool) bool {
	switch expression := expression.(type) {
	case nil:
		return false
	case *ast.Ident:
		return names[expression.Name]
	case *ast.IndexExpr:
		return slice62bTypeExpressionRetains(expression.X, names) || slice62bTypeExpressionRetains(expression.Index, names)
	case *ast.IndexListExpr:
		if slice62bTypeExpressionRetains(expression.X, names) {
			return true
		}
		for _, index := range expression.Indices {
			if slice62bTypeExpressionRetains(index, names) {
				return true
			}
		}
	case *ast.ParenExpr:
		return slice62bTypeExpressionRetains(expression.X, names)
	case *ast.StarExpr:
		return slice62bTypeExpressionRetains(expression.X, names)
	case *ast.ArrayType:
		return slice62bTypeExpressionRetains(expression.Elt, names)
	case *ast.MapType:
		return slice62bTypeExpressionRetains(expression.Key, names) || slice62bTypeExpressionRetains(expression.Value, names)
	case *ast.ChanType:
		return slice62bTypeExpressionRetains(expression.Value, names)
	case *ast.Ellipsis:
		return slice62bTypeExpressionRetains(expression.Elt, names)
	case *ast.StructType:
		return slice62bFieldListRetains(expression.Fields, names)
	case *ast.InterfaceType:
		return slice62bFieldListRetains(expression.Methods, names)
	case *ast.FuncType:
		return slice62bFieldListRetains(expression.Params, names) || slice62bFieldListRetains(expression.Results, names)
	}
	return false
}

func slice62bFieldListRetains(fields *ast.FieldList, names map[string]bool) bool {
	if fields == nil {
		return false
	}
	for _, field := range fields.List {
		if slice62bTypeExpressionRetains(field.Type, names) {
			return true
		}
	}
	return false
}

func slice62bCanonicalRetainedType(name string) bool {
	return name == "protocolSchedulingController" ||
		name == "muxProtocolSchedulingDispatchOperationAdapter" ||
		name == "muxProtocolSchedulingApplyOperationAdapter"
}

func slice62bRetainedType(expression ast.Expr, names map[string]bool) bool {
	return slice62bTypeExpressionRetains(expression, names)
}

func checkSlice62bProductionSurface() []finding {
	const root = "internal/mux"
	fset := token.NewFileSet()
	entries, err := os.ReadDir(root)
	if err != nil {
		return []finding{{path: root, reason: err.Error()}}
	}
	files := make(map[string]*ast.File)
	var findings []finding
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.ToSlash(filepath.Join(root, entry.Name()))
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			findings = append(findings, finding{path: path, reason: parseErr.Error()})
			continue
		}
		files[path] = file
	}
	controllerTypeNames := slice62bControllerTypeNames(files)
	retainedTypeNames := slice62bRetainedTypeNames(files)
	controllerMethods := 0
	muxControllerFields := 0
	muxInitializers := 0
	shimDeclarations := make(map[string]int)
	reservedCalls := make(map[string]int)
	protocolFieldSelectors := 0
	controllerComposites := 0
	adapterComposites := make(map[string]int)
	constructorCalls := 0
	for path, file := range files {
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				for _, item := range declaration.Specs {
					typeSpec, ok := item.(*ast.TypeSpec)
					if ok {
						if token.IsExported(typeSpec.Name.Name) && strings.Contains(strings.ToLower(typeSpec.Name.Name), "protocolscheduling") {
							findings = append(findings, finding{path: path, reason: "exported protocol-scheduling type bypass " + typeSpec.Name.Name})
						}
						if !slice62bCanonicalRetainedType(typeSpec.Name.Name) && retainedTypeNames[typeSpec.Name.Name] {
							findings = append(findings, finding{path: path, reason: "protocol-scheduling controller alias is forbidden: " + typeSpec.Name.Name})
						}
						structure, isStruct := typeSpec.Type.(*ast.StructType)
						if isStruct {
							for _, field := range structure.Fields.List {
								typeText := renderSlice62aNode(fset, field.Type)
								canonical := typeSpec.Name.Name == "Mux" && len(field.Names) == 1 && field.Names[0].Name == "protocolScheduling" && compactSlice62bGoText(typeText) == "protocolSchedulingController[muxProtocolSchedulingDispatchOperationAdapter,muxProtocolSchedulingApplyOperationAdapter]"
								if canonical {
									muxControllerFields++
									continue
								}
								if slice62bRetainedType(field.Type, retainedTypeNames) {
									fieldName := "<anonymous>"
									if len(field.Names) != 0 {
										fieldName = field.Names[0].Name
									}
									findings = append(findings, finding{path: path, reason: "controller/adapter retained recursively under forbidden struct field " + typeSpec.Name.Name + "." + fieldName})
								}
							}
						}
					}
					valueSpec, ok := item.(*ast.ValueSpec)
					if ok && valueSpec.Type != nil && slice62bRetainedType(valueSpec.Type, retainedTypeNames) {
						findings = append(findings, finding{path: path, reason: "retained protocol-scheduling controller/adapter variable is forbidden"})
					}
				}
			case *ast.FuncDecl:
				if token.IsExported(declaration.Name.Name) && strings.Contains(strings.ToLower(declaration.Name.Name), "protocolscheduling") {
					findings = append(findings, finding{path: path, reason: "exported protocol-scheduling function/method bypass " + declaration.Name.Name})
				}
				if token.IsExported(declaration.Name.Name) {
					for _, list := range []*ast.FieldList{declaration.Recv, declaration.Type.Params, declaration.Type.Results} {
						if list == nil {
							continue
						}
						for _, field := range list.List {
							if slice62bRetainedType(field.Type, retainedTypeNames) {
								findings = append(findings, finding{path: path, reason: "exported function/method exposes protocol-scheduling controller/adapter type " + declaration.Name.Name})
							}
						}
					}
				}
				if declaration.Recv != nil && len(declaration.Recv.List) == 1 && slice62aControllerTypeExpression(declaration.Recv.List[0].Type, controllerTypeNames) {
					controllerMethods++
					if path != slice62bController.path || slice62bExactMethodSignatures[declaration.Name.Name] == "" {
						findings = append(findings, finding{path: path, reason: "protocolSchedulingController production method inventory permits only the exact five methods in protocol_scheduling_controller.go"})
					}
				}
				if shim, expected := slice62bExactShims[declaration.Name.Name]; expected {
					shimDeclarations[declaration.Name.Name]++
					if path != shim.path || !receiverNamed(declaration.Recv, "Mux") || renderSlice62aNode(fset, declaration.Body) != shim.body {
						findings = append(findings, finding{path: path, reason: "private Mux shim " + declaration.Name.Name + " must remain the exact one-call facade"})
					}
				}
				if declaration.Name.Name == "New" && declaration.Body != nil {
					ast.Inspect(declaration.Body, func(node ast.Node) bool {
						keyValue, ok := node.(*ast.KeyValueExpr)
						if !ok || !slice62aIdentifierNamed(keyValue.Key, "protocolScheduling") {
							return true
						}
						call, ok := keyValue.Value.(*ast.CallExpr)
						if ok && len(call.Args) == 0 && compactSlice62bGoText(renderSlice62aNode(fset, call.Fun)) == "newProtocolSchedulingController[muxProtocolSchedulingDispatchOperationAdapter,muxProtocolSchedulingApplyOperationAdapter]" {
							muxInitializers++
						} else {
							findings = append(findings, finding{path: path, reason: "Mux protocolScheduling initializer must remain exact"})
						}
						return true
					})
				}
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.CompositeLit:
				typeText := compactSlice62bGoText(renderSlice62aNode(fset, node.Type))
				switch typeText {
				case "protocolSchedulingController[dispatchPort,applyPort]":
					controllerComposites++
				case "muxProtocolSchedulingDispatchOperationAdapter", "muxProtocolSchedulingApplyOperationAdapter":
					adapterComposites[typeText]++
				default:
					if slice62bRetainedType(node.Type, retainedTypeNames) {
						findings = append(findings, finding{path: path, reason: "controller/adapter hidden recursively in composite literal " + typeText})
					}
				}
			case *ast.CallExpr:
				if compactSlice62bGoText(renderSlice62aNode(fset, node.Fun)) == "newProtocolSchedulingController[muxProtocolSchedulingDispatchOperationAdapter,muxProtocolSchedulingApplyOperationAdapter]" {
					constructorCalls++
				}
				if identifier, ok := node.Fun.(*ast.Ident); ok && (identifier.Name == "new" || identifier.Name == "make") {
					for _, argument := range node.Args {
						if slice62bRetainedType(argument, retainedTypeNames) {
							findings = append(findings, finding{path: path, reason: "controller/adapter hidden recursively behind " + identifier.Name})
						}
					}
				}
			case *ast.SelectorExpr:
				if node.Sel.Name == "protocolScheduling" {
					protocolFieldSelectors++
				}
				if _, reserved := slice62bExactMethodSignatures[node.Sel.Name]; reserved {
					reservedCalls[node.Sel.Name]++
				}
			}
			return true
		})
	}
	if controllerMethods != 5 {
		findings = append(findings, finding{path: slice62bController.path, reason: fmt.Sprintf("production protocolSchedulingController methods=%d want exactly five", controllerMethods)})
	}
	if muxControllerFields != 1 || muxInitializers != 1 {
		findings = append(findings, finding{path: "internal/mux/mux.go", reason: fmt.Sprintf("Mux protocolScheduling fields/initializers=%d/%d want 1/1", muxControllerFields, muxInitializers)})
	}
	if controllerComposites != 1 || constructorCalls != 1 {
		findings = append(findings, finding{path: "internal/mux", reason: fmt.Sprintf("controller composite/constructor calls=%d/%d want exact constructor body/New initializer only", controllerComposites, constructorCalls)})
	}
	if adapterComposites["muxProtocolSchedulingDispatchOperationAdapter"] != 3 || adapterComposites["muxProtocolSchedulingApplyOperationAdapter"] != 2 {
		findings = append(findings, finding{path: "internal/mux", reason: fmt.Sprintf("operation-adapter composite literals=%v want exact dispatch/apply 3/2 in guarded shims", adapterComposites)})
	}
	if protocolFieldSelectors != 5 {
		findings = append(findings, finding{path: "internal/mux", reason: fmt.Sprintf("production protocolScheduling field selector uses=%d want exact five shims; aliases/bypasses are forbidden", protocolFieldSelectors)})
	}
	for name := range slice62bExactShims {
		if shimDeclarations[name] != 1 {
			findings = append(findings, finding{path: "internal/mux", reason: fmt.Sprintf("private Mux shim %s declarations=%d want exactly one", name, shimDeclarations[name])})
		}
	}
	for name := range slice62bExactMethodSignatures {
		if reservedCalls[name] != 2 {
			findings = append(findings, finding{path: "internal/mux", reason: fmt.Sprintf("reserved protocol selector calls %s=%d want controller-port plus exact Mux shim only", name, reservedCalls[name])})
		}
	}
	return findings
}

func checkSlice62bCallerOrder() []finding {
	specs := []struct {
		path     string
		function string
		want     []string
	}{
		{
			path:     "internal/mux/mux_advance.go",
			function: "advancePane",
			want: []string{
				"m.processKittyOutcomes(p)",
				"m.processSixelOutcomes(p)",
				"m.processITermOutcomes(p)",
			},
		},
		{
			path:     "internal/mux/mux.go",
			function: "applySessionIngressEnd",
			want: []string{
				"a.mux.processKittyOutcomes(a.pane)",
				"a.mux.processSixelOutcomes(a.pane)",
				"a.mux.processITermOutcomes(a.pane)",
			},
		},
	}
	var findings []finding
	for _, spec := range specs {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, spec.path, nil, 0)
		if err != nil {
			findings = append(findings, finding{path: spec.path, reason: err.Error()})
			continue
		}
		var got []string
		declarations := 0
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Name.Name != spec.function {
				continue
			}
			declarations++
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch selector.Sel.Name {
				case "processKittyOutcomes", "processSixelOutcomes", "processITermOutcomes":
					got = append(got, renderSlice62aNode(fset, call))
				}
				return true
			})
		}
		if declarations != 1 || strings.Join(got, "|") != strings.Join(spec.want, "|") {
			findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("%s protocol caller order=%v declarations=%d want exact Kitty->Sixel->iTerm %v", spec.function, got, declarations, spec.want)})
		}
	}
	return findings
}

func checkSlice62bKnownDefects() []finding {
	const root = "internal/mux"
	want := map[string]string{
		"TestKnownDefect_L3_09_ErasedSchedulerResultRequiresRuntimeRouting": "expires Slice 4.8",
		"TestKnownDefect_L3_09_MuxClockDoesNotReachTermimageStore":          "expires Slice 4.8",
	}
	seen := make(map[string]int)
	entries, err := os.ReadDir(root)
	if err != nil {
		return []finding{{path: root, reason: err.Error()}}
	}
	var findings []finding
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.ToSlash(filepath.Join(root, entry.Name()))
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			findings = append(findings, finding{path: path, reason: parseErr.Error()})
			continue
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || !strings.HasPrefix(function.Name.Name, "TestKnownDefect_L3_09_") {
				continue
			}
			seen[function.Name.Name]++
			expiry, exact := want[function.Name.Name]
			comment := ""
			if function.Doc != nil {
				comment = function.Doc.Text()
			}
			if !exact || !strings.Contains(comment, expiry) {
				findings = append(findings, finding{path: path, reason: function.Name.Name + " must be one of the exact two known L3-09 tests expiring Slice 4.8"})
			}
		}
	}
	if !mapsEqualStringInt(seen, map[string]int{
		"TestKnownDefect_L3_09_ErasedSchedulerResultRequiresRuntimeRouting": 1,
		"TestKnownDefect_L3_09_MuxClockDoesNotReachTermimageStore":          1,
	}) {
		findings = append(findings, finding{path: "internal/mux/mux_protocol_scheduling_test.go", reason: fmt.Sprintf("known L3-09 inventory=%v want exactly two Slice 4.8 tests", seen)})
	}
	return findings
}

func checkSlice62bCommitsAndPaths() []finding {
	const (
		base        = "801285e56fe85c503f0de3a0f459df8c7356286a"
		tCommit     = "ac708cab6c2f468cc04c2e762a97c2cf19375ca3"
		aCommit     = "fb30dff9de599c2043c06f96b6466377fa3482fa"
		mCommit     = "64d407e23e0a857d0638b6460e69a15a32ee03ef"
		wCommit     = "4ba1b3eb83577f02d555e8d23f059e89bd2de9b5"
		gSubject    = "refactor(mux): guard protocol scheduling controller delegation"
		sliceBranch = "arch/l3-01b-mux-protocol-scheduling"
	)
	stages := []struct{ class, commit, parent, subject string }{
		{"T", tCommit, base, "test(mux): characterize protocol scheduling parity"},
		{"A", aCommit, tCommit, "refactor(mux): add protocol scheduling controller seam"},
		{"M", mCommit, aCommit, "refactor(mux): split protocol scheduling adapters"},
		{"W", wCommit, mCommit, "refactor(mux): wire protocol scheduling controller"},
	}
	branch, _ := gitText("symbolic-ref", "--quiet", "--short", "HEAD")
	head, _ := gitText("rev-parse", "HEAD")
	worktree, _ := gitText("status", "--porcelain=v1", "--untracked-files=all")
	if shallow, _ := gitText("rev-parse", "--is-shallow-repository"); shallow == "true" {
		missing := false
		for _, commit := range []string{base, tCommit, aCommit, mCommit, wCommit} {
			if _, err := gitText("cat-file", "-e", commit+"^{commit}"); err != nil {
				missing = true
				break
			}
		}
		if missing {
			gCommit, _ := findMaturitySliceCommitBySubject(gSubject)
			gParent := ""
			if gCommit != "" {
				raw, err := gitText("cat-file", "-p", gCommit)
				if err == nil {
					var parents []string
					for _, line := range strings.Split(raw, "\n") {
						if strings.HasPrefix(line, "parent ") {
							parents = append(parents, strings.TrimSpace(strings.TrimPrefix(line, "parent ")))
						}
					}
					if len(parents) == 1 {
						gParent = parents[0]
					}
				}
			}
			return checkSlice62bShallowFallback(branch == sliceBranch, head, wCommit, gCommit, gParent, worktree)
		}
	}
	var findings []finding
	for _, stage := range stages {
		identity, err := gitFields("show", "-s", "--format=%H%x00%P%x00%s", stage.commit)
		parent, parentErr := gitText("rev-parse", stage.parent+"^{commit}")
		if err != nil || len(identity) != 3 || parentErr != nil || identity[0] != stage.commit || identity[1] != parent || identity[2] != stage.subject {
			findings = append(findings, finding{path: "git:" + stage.class, reason: "unexpected exact Slice 6.2b identity/parent/subject"})
		}
	}
	gCommit, _ := findMaturitySliceCommit(gSubject, wCommit)
	end := gCommit
	includeWorktree := false
	if gCommit == "" {
		if head != wCommit {
			findings = append(findings, finding{path: "git:G", reason: "before Slice 6.2b G, HEAD must be exact immutable W 4ba1b3e"})
			return findings
		}
		end = wCommit
		includeWorktree = true
	} else {
		identity, err := gitFields("show", "-s", "--format=%H%x00%P%x00%s", gCommit)
		if err != nil || len(identity) != 3 || identity[1] != wCommit || identity[2] != gSubject {
			findings = append(findings, finding{path: "git:G", reason: "G must have exact W parent and subject"})
		}
		findings = append(findings, checkSlice62bPostGActiveState(branch == sliceBranch, head, gCommit, worktree)...)
	}
	return append(findings, checkSlice62bExactPaths(base, end, includeWorktree)...)
}

func checkSlice62bExactPaths(base, end string, includeWorktree bool) []finding {
	pathsText, err := gitText("diff", "--name-only", base+".."+end)
	if err != nil {
		return []finding{{path: "git:paths", reason: err.Error()}}
	}
	paths := nonEmptyLines(pathsText)
	if includeWorktree {
		for _, args := range [][]string{{"diff", "--name-only"}, {"diff", "--cached", "--name-only"}, {"ls-files", "--others", "--exclude-standard"}} {
			text, _ := gitText(args...)
			paths = append(paths, nonEmptyLines(text)...)
		}
	}
	actual := make(map[string]bool, len(paths))
	for _, path := range paths {
		actual[filepath.ToSlash(path)] = true
	}
	expected := make(map[string]bool, len(slice62bAllowedPaths))
	var findings []finding
	for _, path := range slice62bAllowedPaths {
		expected[path] = true
		if !actual[path] {
			findings = append(findings, finding{path: path, reason: "missing from exact Slice 6.2b changed-path set"})
		}
	}
	for path := range actual {
		if !expected[path] {
			findings = append(findings, finding{path: path, reason: "outside exact Slice 6.2b changed-path allowlist"})
		}
	}
	return findings
}

func checkSlice62bPostGActiveState(active bool, head, gCommit, worktree string) []finding {
	if !active {
		return nil
	}
	var findings []finding
	if head != gCommit {
		findings = append(findings, finding{path: "git:G", reason: "after G exists on active Slice 6.2b branch, HEAD must equal G exactly"})
	}
	if strings.TrimSpace(worktree) != "" {
		findings = append(findings, finding{path: "git:worktree", reason: "after G exists on active Slice 6.2b branch, nonignored worktree must be clean"})
	}
	return findings
}

func checkSlice62bPostGPolicySelfTest() []finding {
	if got := checkSlice62bPostGActiveState(true, "later", "g", ""); len(got) != 1 || got[0].path != "git:G" {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2b post-G self-test did not reject active-branch commit after G"}}
	}
	if got := checkSlice62bPostGActiveState(true, "g", "g", " M dirty.go"); len(got) != 1 || got[0].path != "git:worktree" {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2b post-G self-test did not reject dirty active branch"}}
	}
	if got := checkSlice62bPostGActiveState(false, "later", "g", " M unrelated.go"); len(got) != 0 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2b post-G self-test rejected later main history"}}
	}
	return nil
}

func slice62bDocumentRequirements() []string {
	return []string{
		"| T | `ac708ca` |", "| A | `fb30dff` |", "| M | `64d407e` |", "| W | `4ba1b3e` |", "| G | pending |",
		"refactor(mux): guard protocol scheduling controller delegation",
		"A introduced one combined `dispatch` controller method", "exact sole parent to be W",
		"L3-01 remains **partial**", "L3-09 remains open to Slice 4.8", "6.2d is deferred",
	}
}

func slice62bDocumentFindings(text string) []finding {
	const path = "docs/validation/architecture-maturity-slice-6.2b.md"
	var findings []finding
	for _, required := range slice62bDocumentRequirements() {
		if !strings.Contains(text, required) {
			findings = append(findings, finding{path: path, reason: "shallow checkout is missing documented commit/closure contract " + required})
		}
	}
	for _, allowed := range slice62bAllowedPaths {
		if !strings.Contains(text, "\n"+allowed+"\n") {
			findings = append(findings, finding{path: path, reason: "shallow checkout allowlist is missing " + allowed})
		}
	}
	return findings
}

func checkSlice62bDocumentedSequence() []finding {
	const path = "docs/validation/architecture-maturity-slice-6.2b.md"
	data, err := os.ReadFile(path)
	if err != nil {
		return []finding{{path: path, reason: err.Error()}}
	}
	return slice62bDocumentFindings(string(data))
}

func checkSlice62bShallowFallback(active bool, head, wCommit, gCommit, gParent, worktree string) []finding {
	const path = "docs/validation/architecture-maturity-slice-6.2b.md"
	data, err := os.ReadFile(path)
	if err != nil {
		return []finding{{path: path, reason: err.Error()}}
	}
	return slice62bShallowFallbackFindings(string(data), active, head, wCommit, gCommit, gParent, worktree)
}

func slice62bShallowFallbackFindings(document string, active bool, head, wCommit, gCommit, gParent, worktree string) []finding {
	findings := slice62bDocumentFindings(document)
	if gCommit != "" {
		if gParent == "" {
			if active {
				findings = append(findings, finding{path: "git:G", reason: "history-limited active slice cannot prove identifiable G parentage"})
			}
		} else if gParent != wCommit {
			findings = append(findings, finding{path: "git:G", reason: "history-limited G must expose exact immutable W as its sole parent"})
		}
		if active {
			findings = append(findings, checkSlice62bPostGActiveState(true, head, gCommit, worktree)...)
		}
		return findings
	}
	if !active {
		return findings
	}
	if head != wCommit {
		findings = append(findings, finding{path: "git:G", reason: "history-limited active Slice 6.2b branch without identifiable G must remain at documented immutable W"})
	}
	return findings
}

func checkSlice62bCombinedShallowPostGSelfTest() []finding {
	valid := "\n" + strings.Join(slice62bDocumentRequirements(), "\n") + "\n" + strings.Join(slice62bAllowedPaths, "\n") + "\n"
	if got := slice62bDocumentFindings(valid); len(got) != 0 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2b shallow-doc self-test rejected complete synthetic contract"}}
	}
	missingContract := strings.Replace(valid, slice62bDocumentRequirements()[0], "", 1)
	if got := slice62bDocumentFindings(missingContract); len(got) != 1 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2b shallow-doc self-test did not reject missing identity"}}
	}
	missingPath := strings.Replace(valid, "\n"+slice62bAllowedPaths[0]+"\n", "\n", 1)
	if got := slice62bDocumentFindings(missingPath); len(got) != 1 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2b shallow-doc self-test did not reject missing allowlist path"}}
	}
	postG := slice62bShallowFallbackFindings(valid, true, "later", "w", "g", "w", " M dirty.go")
	if len(postG) != 2 || postG[0].path != "git:G" || postG[1].path != "git:worktree" {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2b combined shallow/post-G self-test did not enforce HEAD/clean policy"}}
	}
	if got := slice62bShallowFallbackFindings(valid, true, "g", "w", "g", "wrong-parent", ""); len(got) != 1 || got[0].path != "git:G" {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2b shallow post-G self-test did not reject wrong G parent"}}
	}
	if got := slice62bShallowFallbackFindings(valid, false, "later-main", "w", "", "", " M unrelated.go"); len(got) != 0 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2b shallow fallback rejected later main when G metadata was unavailable"}}
	}
	if got := slice62bShallowFallbackFindings(valid, false, "later-main", "w", "g", "", " M unrelated.go"); len(got) != 0 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2b shallow fallback rejected later main when identifiable G parent metadata was unavailable"}}
	}
	if got := slice62bShallowFallbackFindings(valid, true, "later", "w", "", "", ""); len(got) != 1 || got[0].path != "git:G" {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2b shallow pre-G self-test did not reject commit beyond W"}}
	}
	return nil
}

func checkSlice62bBypassSelfTest() []finding {
	fixtures := []struct {
		name   string
		source string
	}{
		{"alias bypass", `package mux
			type protocolSchedulingController[T, U any] struct{}
			type Mux struct { protocolScheduling protocolSchedulingController[int, int] }
			func (m *Mux) processKittyOutcomes() { alias := m.protocolScheduling; alias.dispatchKitty(nil, nil) }`},
		{"alternate field", `package mux
			type protocolSchedulingController[T, U any] struct{}
			type holder struct { controller protocolSchedulingController[int, int] }`},
		{"direct adapter bypass", `package mux
			type muxProtocolSchedulingApplyOperationAdapter struct{}
			func bypass(a muxProtocolSchedulingApplyOperationAdapter) { a.applyExpiry(nil) }`},
		{"nested alias and field retention", `package mux
			type protocolSchedulingController[T, U any] struct{}
			type muxProtocolSchedulingDispatchOperationAdapter struct{}
			type controllerAlias = protocolSchedulingController[int, int]
			type pointerAlias *controllerAlias
			type sliceAlias []pointerAlias
			type arrayAlias [1]muxProtocolSchedulingDispatchOperationAdapter
			type mapAlias map[string]struct { controllers sliceAlias; adapters *arrayAlias }
			type holder struct { nested mapAlias; anonymous struct { hidden []controllerAlias } }`},
		{"inferred nested composite retention", `package mux
			type muxProtocolSchedulingApplyOperationAdapter struct{}
			var hidden = map[string][]*muxProtocolSchedulingApplyOperationAdapter{}`},
	}
	var findings []finding
	for _, fixture := range fixtures {
		file, err := parser.ParseFile(token.NewFileSet(), "internal/mux/fixture.go", fixture.source, 0)
		if err != nil {
			findings = append(findings, finding{path: "scripts/check-maturity-gates.go", reason: "cannot parse Slice 6.2b bypass self-test " + fixture.name})
			continue
		}
		got := checkSlice62bSyntheticSurface(map[string]*ast.File{"internal/mux/fixture.go": file})
		if len(got) == 0 {
			findings = append(findings, finding{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2b bypass self-test did not reject " + fixture.name})
		}
	}
	return findings
}

func checkSlice62bSyntheticSurface(files map[string]*ast.File) []finding {
	retainedTypeNames := slice62bRetainedTypeNames(files)
	var findings []finding
	for path, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.StructType:
				for _, field := range node.Fields.List {
					if slice62bRetainedType(field.Type, retainedTypeNames) {
						findings = append(findings, finding{path: path, reason: "controller/adapter retained recursively under alternate field"})
					}
				}
			case *ast.TypeSpec:
				if !slice62bCanonicalRetainedType(node.Name.Name) && retainedTypeNames[node.Name.Name] {
					findings = append(findings, finding{path: path, reason: "controller/adapter retained through alias or named container " + node.Name.Name})
				}
			case *ast.CompositeLit:
				if slice62bRetainedType(node.Type, retainedTypeNames) {
					findings = append(findings, finding{path: path, reason: "controller/adapter retained recursively in inferred composite"})
				}
			case *ast.CallExpr:
				if identifier, ok := node.Fun.(*ast.Ident); ok && (identifier.Name == "new" || identifier.Name == "make") {
					for _, argument := range node.Args {
						if slice62bRetainedType(argument, retainedTypeNames) {
							findings = append(findings, finding{path: path, reason: "controller/adapter retained recursively behind " + identifier.Name})
						}
					}
				}
			case *ast.SelectorExpr:
				if node.Sel.Name == "protocolScheduling" {
					findings = append(findings, finding{path: path, reason: "controller field alias/bypass"})
				}
				if _, reserved := slice62bExactMethodSignatures[node.Sel.Name]; reserved {
					findings = append(findings, finding{path: path, reason: "reserved controller/adapter method bypass " + node.Sel.Name})
				}
			}
			return true
		})
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
