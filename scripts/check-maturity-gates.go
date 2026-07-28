//go:build ignore

// check-maturity-gates enforces lightweight beta-maturity guardrails that are
// cheap enough to run in CI. It intentionally checks repository structure and
// documentation promises rather than external services.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
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
	"docs/validation/architecture-maturity-slice-6.2c.md",
	"docs/validation/architecture-maturity-slice-5.5a.md",
	"docs/validation/architecture-maturity-slice-5.5b.md",
	"docs/validation/architecture-maturity-slice-5.5c.md",
	"scripts/capture-phase15-benchmarks.go",
	"scripts/capture-phase15-process.py",
	"scripts/check-phase15-recovery.go",
	"scripts/daily-driver-smoke.go",
	"scripts/package-beta.go",
	"scripts/release-preflight.go",
	"scripts/smoke-installed-package.go",
}

var largeGoAllowlist = map[string]string{
	filepath.ToSlash("internal/fontglyph/backend.go"):         "known font fallback/raster orchestration split target",
	filepath.ToSlash("internal/fontglyph/discovery/index.go"): "bounded discovery implementation extracted in Slice 5.5a; split target remains 5.5b/5.5c-neutral",
	filepath.ToSlash("internal/mux/mux.go"):                   "L3-01 preparatory facade; formal split target Slice 6.2d",
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
	findings = append(findings, checkRewrittenHistoryEvidence()...)
	findings = append(findings, checkSlice63cGuard()...)
	findings = append(findings, checkSlice62aGuard()...)
	findings = append(findings, checkSlice62bGuard()...)
	findings = append(findings, checkSlice62cGuard()...)
	findings = append(findings, checkSlice55aGuard()...)
	findings = append(findings, checkSlice55bGuard()...)
	findings = append(findings, checkSlice55cGuard()...)
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

type rewrittenHistoryEvidence struct {
	label, commit, tree, patchID string
}

// Each stable patch ID below was compared with its retained pre-rewrite commit,
// and each old/current changed-path set was exact. The corresponding commit trees
// differed only by the intentional removal of .agent. Pinning the authoritative
// current tree and retained stable diff identity prevents a silent SHA-only repin.
var rewrittenHistoryPins = []rewrittenHistoryEvidence{
	{"6.2a/base", "d16e8c88d20b67f6f1568ca47b017032c5fd3309", "ff76d5b1574884c6546259cd6af31d912aa64555", ""},
	{"6.2a/T", "068e3f0693797e266e9697fed85df9e1814fccb5", "276c6034d52f74061cf72f485123f6962fd0e729", "4276a6ca64ef2b035950662cbdf5eeae1fcd446e"},
	{"6.2a/A", "b15312e565ab84e479c5d1fd591687137197d70e", "00d3d80ff3ca6e2985803de581356e5c0dd5c774", "8ff77428379c1bc7bcce8fe1612fa887bfae9b4e"},
	{"6.2a/M", "e738b94ecc1f0d238504f2dc16f88e2b9ffe655c", "89519f13bfe887f2c23a2f4c7af7848cefd6c7e0", "41ce87914ddcb95ccce5e5c4a7d16e4aa11c1bb6"},
	{"6.2a/W", "fbc7fec58cbcae5977859f4317a9dcc45a7315f1", "40c4769cb902b874cbc9df39b33c78345a1a14f7", "a0d5b0c485ffa19df2fa7aeeb273284da92544d6"},
	{"6.2b/base", "dfc62d62e047d5a46e95f9d8f930fca3b95b70b3", "d05b848dfac38c6ccacca6febaba4dd3f96fb227", ""},
	{"6.2b/T", "051709757f008da8bafb25f89567925d8d19c92f", "afd441935fccb708977a01c54b531fc644020a8e", "c16e27ad207896b660789e9e32d6681c2bc22691"},
	{"6.2b/A", "71d21bbbf43dd54fd7a855d95e5e37122f437002", "1dfb935fe3db39b3cc6db4fcdf6322e5f09e464e", "43c9f332272e1f63084973dd6dbbe3314aaa8b74"},
	{"6.2b/M", "2f918e72b89064da2c8782699c5d00a794c2a635", "c25b68c355634e84bcf349d5a5d733d4c2a0c948", "8fae7f0ac9c587926e5904d72a9fcfb2aa7754a8"},
	{"6.2b/W", "caaa979cf85c7d19ed25aa9655dffe0e5beb7409", "52eab1e8cfa3bfa4fa38cf06c53dbfdb19d3cd1e", "5187cf894ad03b8b62870a7ec26f5d866de692fb"},
	{"6.2c/base", "005da8f19e0227e0a133b7d9b50dce4a191d66ee", "122e47eae55ddc0d351e15b6d7983e48c445fce1", ""},
	{"6.2c/T", "e126a428330e3d13b309ccb03286f76a9f3e00a7", "45088533a38dba6e0916f4792365a12a71066a28", "47ce219ca5a1ba82d63c3a8950cd9ee0543123e2"},
	{"6.2c/A", "780a36658d9c02ea51ac259545e1e6a2cc25b55b", "b69cbfc2ab26154e79c1b64a1c5455b2f89c1cf5", "7c03226764aa90b7df29fc79f4e9ddf8bf1d5c33"},
	{"6.2c/M", "3f75bf11f5440f00664665b08a85e07ceae27bdd", "b7b37e99f9d5d87dde04702bbe445d299d05a521", "49ce8c2a0229be97874a365ebf1f295bab38f8fc"},
	{"6.2c/W", "62d3b959e7e1b3934a231ccf43bb0021661f10c3", "f8b6c8a41401b87ba8abf623bdea066889baacd9", "77c2d3b7768c15db15c9269e42ecec5809123ec7"},
	{"5.5a/base", "c027fd1228af792203d3361a671a9b01017b2e23", "600e5c86ad52bf654c4a12d6af8fed549ac61197", ""},
	{"5.5a/T", "35243d7f7672f28ddb39ac55cd404e5fa96ed990", "45ce7f9e853f58f86a1dda5aef1382b53363e049", "e29d84b7808e8e27816adc72ddea4b0ad48785b1"},
	{"5.5a/A", "2485ea931a8c1a781b17cc26851ff325c0c7ceb2", "4784aea789d97725fbc2b2b060cff34ffa2613d5", "cf2f9f8cddfc630f047649266e1e62aa62cae113"},
	{"5.5a/M", "1ecc9cda5cccedb86ecca8d5a8803143c88d11d5", "43feed9cc1501723c3372cc68a04243b58fe0f22", "ebd2371abb4e09ffe419f2c618866b9ae92b33ce"},
	{"5.5a/W", "28326fa5bb05850a0d12c31afe0f334fa329b636", "f55cace39032fbac046a83ceee12fe71bd71b5bf", "facd079fb8e5f0e633fa057775946b3f4b80e612"},
	{"5.5b/base", "10857a0a53815e375894bebebc7326c119fda350", "851a7e1cabc99f1053b539985bc08202fefa4f81", ""},
	{"5.5b/T", "09da2261e2965e2bbe0d56f49f1b20eab7ca4e13", "46bf2630e747dc90b1680f5effd239b59e752757", "cbcbe27db2aff28b6af6d6260e720adf42ed57e2"},
	{"5.5b/A", "b8d603295e82cc581b86711f06208af04afe911e", "993172bbec5c27d4afae1d4358970daaee36ca38", "5ca61693bac7c790c79d26932ad9db5f6d555150"},
	{"5.5b/M", "2b3686a029a611a74be621dc8b09dc8bd28e5826", "44eb6d1b24f3565aee621c2e9057a4bdd1b1e1ab", "70a155851f2c43ed2dca62f1ba53e10146e88006"},
	{"5.5b/W", "b98ee43fe500f40fbd6c16799d1b58551e85cfb7", "8d0e13a065c720ce33dd728cb3ea6a88a54f718c", "6251cab6ac181b0e4942320cdb962fc6cf72d443"},
	{"5.5b/G", "92fa34a2d18455581a8916ef1a2bcb5af195c881", "ff21b8405fb00a91e926e7f5fa255dce837e9e52", "dae9df1275a06a6ff982df86864ca9abd74cb541"},
	{"5.5c/base", "09ebf4f3e668c0bba4c94f1aa7bf2e2c3e55883d", "ff21b8405fb00a91e926e7f5fa255dce837e9e52", ""},
	{"5.5c/T", "95f7269bc3ce438579c3fe256bf87edbff9e5cc3", "05d336f4db28d40761611c8a5dd195b2c5227995", "6cc9a079a2e7dda396077ec753945d16c92aad7d"},
	{"5.5c/A", "d9885d18d3980ae1e90a7aa4df0ffaa97284167b", "99bc8d25b238878cad165d6ed7251efc0caa3f7d", "534cab5bf7d6e448e887b46ba7900914ca1f7c24"},
	{"5.5c/M", "5c9714e9da95a7685c37aca318d8e0486c5a9413", "d3482a330312b0cb62447aff277509771f23c361", "fa0ab9636d70d471f49462c2babc754d05ac5fa0"},
	{"5.5c/W", "b6d724c363bd7aa28119fd4ed208ab0b0d11f650", "386886309a600617cbc28330e264bfc3d2102ea8", "506dcf4db32de716de8dc654be5250dd08623bc4"},
	{"5.5c/G", "83be41ff1ff0c6dbc1cd67b0af1ca87f41a58def", "d690b83cf09e029a01ee35c0da1fce90702c7969", "1f63a2c8af0e4b6df434c97d2eec290256185eb7"},
}

func checkRewrittenHistoryEvidence() []finding {
	shallow, _ := gitText("rev-parse", "--is-shallow-repository")
	var findings []finding
	for _, pin := range rewrittenHistoryPins {
		if !slice55aCommitExists(pin.commit) {
			if shallow != "true" {
				findings = append(findings, finding{path: "git:" + pin.label, reason: "authoritative rewritten commit is unavailable in full history"})
			}
			continue
		}
		tree, treeErr := gitText("rev-parse", pin.commit+"^{tree}")
		if treeErr != nil || tree != pin.tree {
			findings = append(findings, finding{path: "git:" + pin.label, reason: fmt.Sprintf("rewritten tree=%q want exact %s", tree, pin.tree)})
		}
		patchID, patchErr := rewrittenStablePatchID(pin.commit)
		if patchErr != nil || patchID != pin.patchID {
			findings = append(findings, finding{path: "git:" + pin.label, reason: fmt.Sprintf("rewritten stable patch=%q want retained %s", patchID, pin.patchID)})
		}
	}
	return findings
}

func rewrittenStablePatchID(commit string) (string, error) {
	patch, err := exec.Command("git", "show", "--pretty=format:", "--no-ext-diff", commit).Output()
	if err != nil {
		return "", err
	}
	command := exec.Command("git", "patch-id", "--stable")
	command.Stdin = bytes.NewReader(patch)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		return "", nil
	}
	if len(fields) != 2 {
		return "", fmt.Errorf("unexpected git patch-id output %q", strings.TrimSpace(string(output)))
	}
	return fields[0], nil
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
		base        = "1f1a8b957fc411f67376672b3769ecda83dd74ea"
		tCommit     = "c201129eb583edad3e98db1372e93f62fa76e124"
		aCommit     = "260f3cf145d36b236370bb25ce7c2c92ebfd862c"
		mCommit     = "dabc1f5f6e0959731fcbe8f79d09bfdad65cfaf4"
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
	log, err := gitText("log", "--format=%H%x00%P%x00%s", "HEAD", "--all")
	if err != nil {
		return "", err
	}
	return uniqueMaturitySliceCommit(log, subject, parentFull)
}

func uniqueMaturitySliceCommit(log, subject, parent string) (string, error) {
	var matches []string
	for _, line := range strings.Split(log, "\n") {
		parts := strings.Split(line, "\x00")
		if len(parts) == 3 && parts[1] == parent && parts[2] == subject {
			matches = append(matches, parts[0])
		}
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("commit cardinality with exact subject %q and parent %s is %d, want 1", subject, parent, len(matches))
	}
	return matches[0], nil
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
		base        = "d16e8c88d20b67f6f1568ca47b017032c5fd3309"
		tCommit     = "068e3f0693797e266e9697fed85df9e1814fccb5"
		aCommit     = "b15312e565ab84e479c5d1fd591687137197d70e"
		mCommit     = "e738b94ecc1f0d238504f2dc16f88e2b9ffe655c"
		wCommit     = "fbc7fec58cbcae5977859f4317a9dcc45a7315f1"
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
		base        = "dfc62d62e047d5a46e95f9d8f930fca3b95b70b3"
		tCommit     = "051709757f008da8bafb25f89567925d8d19c92f"
		aCommit     = "71d21bbbf43dd54fd7a855d95e5e37122f437002"
		mCommit     = "2f918e72b89064da2c8782699c5d00a794c2a635"
		wCommit     = "caaa979cf85c7d19ed25aa9655dffe0e5beb7409"
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

type slice62cControllerSpec struct {
	path       string
	controller string
	budgetName string
	budget     int
	maxMethods int
	ports      map[string][]string
}

var slice62cAllowedPaths = []string{
	"docs/architecture-maturity/implementation-plan.md",
	"docs/architecture.md",
	"docs/validation/architecture-maturity-slice-6.2c.md",
	"docs/validation/architecture-maturity-slice-6.2c/benchmarks-base.txt",
	"docs/validation/architecture-maturity-slice-6.2c/benchmarks-candidate.txt",
	"docs/validation/architecture-maturity-slice-6.2c/gates.txt",
	"docs/validation/architecture-maturity-slice-6.2c/scope-and-commits.txt",
	"internal/mux/fresh_session.go",
	"internal/mux/mux.go",
	"internal/mux/mux_restore.go",
	"internal/mux/mux_restore_characterization_test.go",
	"internal/mux/mux_restore_test.go",
	"internal/mux/mux_restore_wiring_test.go",
	"internal/mux/restore_coordinator.go",
	"internal/mux/restore_coordinator_test.go",
	"scripts/check-maturity-gates.go",
}

var slice62cGStagePaths = []string{
	"docs/architecture-maturity/implementation-plan.md",
	"docs/architecture.md",
	"docs/validation/architecture-maturity-slice-6.2c.md",
	"docs/validation/architecture-maturity-slice-6.2c/benchmarks-base.txt",
	"docs/validation/architecture-maturity-slice-6.2c/benchmarks-candidate.txt",
	"docs/validation/architecture-maturity-slice-6.2c/gates.txt",
	"docs/validation/architecture-maturity-slice-6.2c/scope-and-commits.txt",
	"internal/mux/mux_restore_wiring_test.go",
	"scripts/check-maturity-gates.go",
}

var slice62cWStagePaths = []string{
	"internal/mux/fresh_session.go",
	"internal/mux/mux.go",
	"internal/mux/mux_restore.go",
	"internal/mux/mux_restore_characterization_test.go",
	"internal/mux/mux_restore_test.go",
	"internal/mux/mux_restore_wiring_test.go",
	"internal/mux/restore_coordinator.go",
	"internal/mux/restore_coordinator_test.go",
}

var slice62cController = slice62cControllerSpec{
	path:       "internal/mux/restore_coordinator.go",
	controller: "restoreCoordinator",
	budgetName: "restoreCoordinatorPortBudget",
	budget:     5,
	maxMethods: 3,
	ports: map[string][]string{
		"restorePreparationPort": {
			"freshSessionSnapshot() (FreshSessionSnapshot, error)",
			"prepareRestore() (*RestoreCandidate, error)",
		},
		"restorePublicationPort": {
			"restoreWindowIDs(*RestoreCandidate) ([]WindowID, error)",
			"commitRestore(*RestoreCandidate) ([]Event, error)",
			"abortRestore(*RestoreCandidate) error",
		},
	},
}

var slice62cExactMethodSignatures = map[string]string{
	"freshSessionSnapshot": "func(port preparationPort) (FreshSessionSnapshot, error)",
	"prepareRestore":       "func(port preparationPort) (*RestoreCandidate, error)",
	"restoreWindowIDs":     "func(candidate *RestoreCandidate, port publicationPort) ([]WindowID, error)",
	"commitRestore":        "func(candidate *RestoreCandidate, port publicationPort) ([]Event, error)",
	"abortRestore":         "func(candidate *RestoreCandidate, port publicationPort) error",
}

var slice62cExactMethodBodies = map[string]string{
	"freshSessionSnapshot": "{\n\treturn port.freshSessionSnapshot()\n}",
	"prepareRestore":       "{\n\treturn port.prepareRestore()\n}",
	"restoreWindowIDs":     "{\n\treturn port.restoreWindowIDs(candidate)\n}",
	"commitRestore":        "{\n\treturn port.commitRestore(candidate)\n}",
	"abortRestore":         "{\n\treturn port.abortRestore(candidate)\n}",
}

type slice62cFacadeSpec struct {
	path      string
	signature string
	body      string
}

var slice62cExactFacades = map[string]slice62cFacadeSpec{
	"FreshSessionSnapshot": {
		path: "internal/mux/fresh_session.go", signature: "func() (FreshSessionSnapshot, error)",
		body: "{\n\treturn m.restoreCoordinator.freshSessionSnapshot(muxRestorePreparationOperationAdapter{mux: m})\n}",
	},
	"PrepareRestore": {
		path: "internal/mux/mux_restore.go", signature: "func(blueprint layoutrestore.Blueprint, geometries []RestoreWindowGeometry) (*RestoreCandidate, error)",
		body: "{\n\treturn m.restoreCoordinator.prepareRestore(muxRestorePreparationOperationAdapter{mux: m, blueprint: blueprint, geometries: geometries})\n}",
	},
	"RestoreWindowIDs": {
		path: "internal/mux/mux_restore.go", signature: "func(candidate *RestoreCandidate) ([]WindowID, error)",
		body: "{\n\treturn m.restoreCoordinator.restoreWindowIDs(candidate, muxRestorePublicationOperationAdapter{mux: m})\n}",
	},
	"CommitRestore": {
		path: "internal/mux/mux_restore.go", signature: "func(candidate *RestoreCandidate) ([]Event, error)",
		body: "{\n\treturn m.restoreCoordinator.commitRestore(candidate, muxRestorePublicationOperationAdapter{mux: m})\n}",
	},
	"AbortRestore": {
		path: "internal/mux/mux_restore.go", signature: "func(candidate *RestoreCandidate) error",
		body: "{\n\treturn m.restoreCoordinator.abortRestore(candidate, muxRestorePublicationOperationAdapter{mux: m})\n}",
	},
}

var slice62cKnownDefectBodyHashes = map[string]string{
	"TestKnownDefect_L3_02_RestoreAcceptsDifferentOwnerThread":  "5575edb0c3abb1a852d08c02bdc96ba626090a9bfdebf7ed5f4aaeff9111e830",
	"TestKnownDefect_L3_07_FreshSessionUsesObservedTerminalCWD": "ed6009cb25a396219c5d9c313468feb6b4a8665c4cbdc3205059c13fa1c8fc35",
}

const slice62cRestoreCommitBodyFingerprint = "913e0679cd7012d1d1763bbce4dfb9600c4a6aac652c17fe4c8f9e4688c06d14"

func checkSlice62cGuard() []finding {
	var findings []finding
	findings = append(findings, checkSlice62cPostGPolicySelfTest()...)
	findings = append(findings, checkSlice62cFullHistoryPreGSelfTest()...)
	findings = append(findings, checkSlice62cCombinedShallowPostGSelfTest()...)
	findings = append(findings, checkSlice62cBypassSelfTest()...)
	findings = append(findings, checkSlice62cRestoreOrderSelfTest()...)
	findings = append(findings, checkSlice62cKnownDefectBodySelfTest()...)
	findings = append(findings, checkSlice62cDocumentedSequence()...)
	findings = append(findings, checkSlice62cController(slice62cController)...)
	findings = append(findings, checkSlice62cProductionSurface()...)
	findings = append(findings, checkSlice62cRestoreOrder()...)
	findings = append(findings, checkSlice62cKnownDefects()...)
	findings = append(findings, checkSlice62cCommitsAndPaths()...)
	return findings
}

func checkSlice62cController(spec slice62cControllerSpec) []finding {
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
		return append(findings, finding{path: spec.path, reason: "cannot parse restore coordinator: " + err.Error()})
	}
	if len(file.Imports) != 0 {
		findings = append(findings, finding{path: spec.path, reason: "generic restore coordinator must remain import-free"})
	}
	wantDeclarations := map[string]int{
		spec.budgetName: 1, "restorePreparationPort": 1, "restorePublicationPort": 1,
		spec.controller: 1, "newRestoreCoordinator": 1,
		"method:freshSessionSnapshot": 1, "method:prepareRestore": 1, "method:restoreWindowIDs": 1,
		"method:commitRestore": 1, "method:abortRestore": 1,
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
						want := []string{"preparationPort:restorePreparationPort", "publicationPort:restorePublicationPort"}
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
								findings = append(findings, forbiddenSlice62cType(spec.path, node.Name.Name, typeText)...)
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
				if declaration.Name.Name == "newRestoreCoordinator" {
					want := "func[preparationPort restorePreparationPort, publicationPort restorePublicationPort]() restoreCoordinator[preparationPort, publicationPort]"
					if got := renderSlice62aNode(fset, declaration.Type); compactSlice62bGoText(got) != compactSlice62bGoText(want) {
						findings = append(findings, finding{path: spec.path, reason: "constructor signature changed: " + got})
					}
					wantBody := "{\n\treturn restoreCoordinator[preparationPort, publicationPort]{}\n}"
					if got := renderSlice62aNode(fset, declaration.Body); got != wantBody {
						findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("constructor body=%q want exact zero-value body", got)})
					}
				}
				continue
			}
			key := "method:" + declaration.Name.Name
			declarations[key]++
			methodInventory[declaration.Name.Name]++
			wantSignature, known := slice62cExactMethodSignatures[declaration.Name.Name]
			if !known {
				continue
			}
			gotReceiver := strings.Join(renderedUnnamedFields(fset, declaration.Recv), "|")
			if gotReceiver != "restoreCoordinator[preparationPort, publicationPort]" || renderSlice62aNode(fset, declaration.Type) != wantSignature {
				findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("%s receiver/signature changed", declaration.Name.Name)})
			}
			if got := renderSlice62aNode(fset, declaration.Body); got != slice62cExactMethodBodies[declaration.Name.Name] {
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
	for name := range slice62cExactMethodSignatures {
		if methodInventory[name] != 1 {
			findings = append(findings, finding{path: spec.path, reason: "controller method inventory missing/duplicates " + name})
		}
	}
	if portCount != spec.budget {
		findings = append(findings, finding{path: spec.path, reason: fmt.Sprintf("aggregate port methods=%d budget=%d", portCount, spec.budget)})
	}
	return findings
}

func forbiddenSlice62cType(path, owner, typeText string) []finding {
	lower := strings.ToLower(typeText)
	for _, forbidden := range []string{"*mux", "*model", "*localsessionregistry", "*pane", "map[", "func(", "chan ", "interface{}", "any"} {
		if strings.Contains(lower, forbidden) {
			return []finding{{path: path, reason: owner + " has forbidden retained-owner/state type " + typeText}}
		}
	}
	return nil
}

func slice62cRetainedTypeNames(files map[string]*ast.File) map[string]bool {
	return slice62bExpandRetainedTypeNames(files, map[string]bool{
		"restoreCoordinator":                    true,
		"muxRestorePreparationOperationAdapter": true,
		"muxRestorePublicationOperationAdapter": true,
	})
}

func slice62cCanonicalRetainedType(name string) bool {
	return name == "restoreCoordinator" || name == "muxRestorePreparationOperationAdapter" || name == "muxRestorePublicationOperationAdapter"
}

func checkSlice62cProductionSurface() []finding {
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
	retainedTypeNames := slice62cRetainedTypeNames(files)
	controllerTypeNames := slice62bExpandRetainedTypeNames(files, map[string]bool{"restoreCoordinator": true})
	controllerMethods := 0
	muxControllerFields := 0
	muxInitializers := 0
	facadeDeclarations := make(map[string]int)
	reservedCalls := make(map[string]int)
	restoreFieldSelectors := 0
	controllerComposites := 0
	adapterComposites := make(map[string]int)
	constructorCalls := 0
	for path, file := range files {
		findings = append(findings, slice62cFunctionRetentionFindings(path, file, retainedTypeNames)...)
	}
	for path, file := range files {
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				for _, item := range declaration.Specs {
					typeSpec, ok := item.(*ast.TypeSpec)
					if ok {
						if token.IsExported(typeSpec.Name.Name) && strings.Contains(strings.ToLower(typeSpec.Name.Name), "restorecoordinator") {
							findings = append(findings, finding{path: path, reason: "exported restore-coordinator type bypass " + typeSpec.Name.Name})
						}
						if !slice62cCanonicalRetainedType(typeSpec.Name.Name) && typeSpec.Name.Name != "Mux" && retainedTypeNames[typeSpec.Name.Name] {
							findings = append(findings, finding{path: path, reason: "restore controller/adapter alias or named container is forbidden: " + typeSpec.Name.Name})
						}
						structure, isStruct := typeSpec.Type.(*ast.StructType)
						if isStruct {
							for _, field := range structure.Fields.List {
								typeText := compactSlice62bGoText(renderSlice62aNode(fset, field.Type))
								canonical := typeSpec.Name.Name == "Mux" && len(field.Names) == 1 && field.Names[0].Name == "restoreCoordinator" && typeText == "restoreCoordinator[muxRestorePreparationOperationAdapter,muxRestorePublicationOperationAdapter]"
								if canonical {
									muxControllerFields++
									continue
								}
								if slice62bTypeExpressionRetains(field.Type, retainedTypeNames) {
									fieldName := "<anonymous>"
									if len(field.Names) != 0 {
										fieldName = field.Names[0].Name
									}
									findings = append(findings, finding{path: path, reason: "restore controller/adapter retained recursively under forbidden struct field " + typeSpec.Name.Name + "." + fieldName})
								}
							}
						}
					}
					valueSpec, ok := item.(*ast.ValueSpec)
					if ok && valueSpec.Type != nil && slice62bTypeExpressionRetains(valueSpec.Type, retainedTypeNames) {
						findings = append(findings, finding{path: path, reason: "retained restore controller/adapter variable is forbidden"})
					}
				}
			case *ast.FuncDecl:
				if token.IsExported(declaration.Name.Name) && strings.Contains(strings.ToLower(declaration.Name.Name), "restorecoordinator") {
					findings = append(findings, finding{path: path, reason: "exported restore-coordinator function/method bypass " + declaration.Name.Name})
				}
				if token.IsExported(declaration.Name.Name) {
					for _, list := range []*ast.FieldList{declaration.Recv, declaration.Type.Params, declaration.Type.Results} {
						if list == nil {
							continue
						}
						for _, field := range list.List {
							if slice62bTypeExpressionRetains(field.Type, retainedTypeNames) {
								findings = append(findings, finding{path: path, reason: "exported function/method exposes restore controller/adapter type " + declaration.Name.Name})
							}
						}
					}
				}
				if declaration.Recv != nil && len(declaration.Recv.List) == 1 && slice62aControllerTypeExpression(declaration.Recv.List[0].Type, controllerTypeNames) {
					controllerMethods++
					if path != slice62cController.path || slice62cExactMethodSignatures[declaration.Name.Name] == "" {
						findings = append(findings, finding{path: path, reason: "restoreCoordinator production method inventory permits only the exact five methods in restore_coordinator.go"})
					}
				}
				if facade, expected := slice62cExactFacades[declaration.Name.Name]; expected {
					facadeDeclarations[declaration.Name.Name]++
					if path != facade.path || !receiverNamed(declaration.Recv, "Mux") || renderSlice62aNode(fset, declaration.Type) != facade.signature || renderSlice62aNode(fset, declaration.Body) != facade.body {
						findings = append(findings, finding{path: path, reason: "public Mux restore facade " + declaration.Name.Name + " must retain exact signature and one-call body"})
					}
				}
				if declaration.Name.Name == "New" && declaration.Body != nil {
					ast.Inspect(declaration.Body, func(node ast.Node) bool {
						keyValue, ok := node.(*ast.KeyValueExpr)
						if !ok || !slice62aIdentifierNamed(keyValue.Key, "restoreCoordinator") {
							return true
						}
						call, ok := keyValue.Value.(*ast.CallExpr)
						if ok && len(call.Args) == 0 && compactSlice62bGoText(renderSlice62aNode(fset, call.Fun)) == "newRestoreCoordinator[muxRestorePreparationOperationAdapter,muxRestorePublicationOperationAdapter]" {
							muxInitializers++
						} else {
							findings = append(findings, finding{path: path, reason: "Mux restoreCoordinator initializer must remain exact"})
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
				case "restoreCoordinator[preparationPort,publicationPort]":
					controllerComposites++
				case "muxRestorePreparationOperationAdapter", "muxRestorePublicationOperationAdapter":
					adapterComposites[typeText]++
				default:
					if slice62bTypeExpressionRetains(node.Type, retainedTypeNames) {
						findings = append(findings, finding{path: path, reason: "restore controller/adapter hidden recursively in composite literal " + typeText})
					}
				}
			case *ast.CallExpr:
				if compactSlice62bGoText(renderSlice62aNode(fset, node.Fun)) == "newRestoreCoordinator[muxRestorePreparationOperationAdapter,muxRestorePublicationOperationAdapter]" {
					constructorCalls++
				}
				if identifier, ok := node.Fun.(*ast.Ident); ok && (identifier.Name == "new" || identifier.Name == "make") {
					for _, argument := range node.Args {
						if slice62bTypeExpressionRetains(argument, retainedTypeNames) {
							findings = append(findings, finding{path: path, reason: "restore controller/adapter hidden recursively behind " + identifier.Name})
						}
					}
				}
			case *ast.SelectorExpr:
				if node.Sel.Name == "restoreCoordinator" {
					restoreFieldSelectors++
				}
				if _, reserved := slice62cExactMethodSignatures[node.Sel.Name]; reserved {
					reservedCalls[node.Sel.Name]++
				}
			}
			return true
		})
	}
	if controllerMethods != 5 {
		findings = append(findings, finding{path: slice62cController.path, reason: fmt.Sprintf("production restoreCoordinator methods=%d want exactly five", controllerMethods)})
	}
	if muxControllerFields != 1 || muxInitializers != 1 {
		findings = append(findings, finding{path: "internal/mux/mux.go", reason: fmt.Sprintf("Mux restoreCoordinator fields/initializers=%d/%d want 1/1", muxControllerFields, muxInitializers)})
	}
	if controllerComposites != 1 || constructorCalls != 1 {
		findings = append(findings, finding{path: root, reason: fmt.Sprintf("restore controller composite/constructor calls=%d/%d want exact constructor body/New initializer only", controllerComposites, constructorCalls)})
	}
	if adapterComposites["muxRestorePreparationOperationAdapter"] != 2 || adapterComposites["muxRestorePublicationOperationAdapter"] != 3 {
		findings = append(findings, finding{path: root, reason: fmt.Sprintf("restore operation-adapter composite literals=%v want exact preparation/publication 2/3 in guarded facades", adapterComposites)})
	}
	if restoreFieldSelectors != 5 {
		findings = append(findings, finding{path: root, reason: fmt.Sprintf("production restoreCoordinator field selector uses=%d want exactly five facade-to-controller calls", restoreFieldSelectors)})
	}
	for name := range slice62cExactFacades {
		if facadeDeclarations[name] != 1 {
			findings = append(findings, finding{path: root, reason: fmt.Sprintf("public Mux restore facade %s declarations=%d want exactly one", name, facadeDeclarations[name])})
		}
	}
	wantReserved := map[string]int{"freshSessionSnapshot": 2, "prepareRestore": 2, "restoreWindowIDs": 2, "commitRestore": 2, "abortRestore": 5}
	for name, want := range wantReserved {
		if reservedCalls[name] != want {
			findings = append(findings, finding{path: root, reason: fmt.Sprintf("reserved restore selector calls %s=%d want=%d exact facade/controller/helper inventory", name, reservedCalls[name], want)})
		}
	}
	return findings
}

func checkSlice62cRestoreOrder() []finding {
	const path = "internal/mux/mux_restore.go"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return []finding{{path: path, reason: err.Error()}}
	}
	var findings []finding
	var commit, abort *ast.FuncDecl
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		receiver := strings.Join(renderedUnnamedFields(fset, function.Recv), "|")
		if function.Name.Name == "commitRestore" && receiver == "muxRestorePublicationOperationAdapter" {
			commit = function
		}
		if function.Name.Name == "abortRestore" && receiver == "*Mux" {
			abort = function
		}
	}
	if commit == nil {
		findings = append(findings, finding{path: path, reason: "missing restore publication operation"})
	} else {
		findings = append(findings, slice62cRestoreCommitOrderFindings(fset, path, commit.Body)...)
		wantHash := slice62cRestoreCommitBodyFingerprint
		if immutable, immutableErr := gitText("show", "62d3b959e7e1b3934a231ccf43bb0021661f10c3:"+path); immutableErr == nil {
			immutableHash, hashErr := slice62cRestoreCommitHashFromSource(path, immutable)
			if hashErr != nil {
				findings = append(findings, finding{path: path, reason: "cannot parse immutable W restore publication body: " + hashErr.Error()})
			} else {
				wantHash = immutableHash
				if slice62cRestoreCommitBodyFingerprint != immutableHash {
					findings = append(findings, finding{path: "scripts/check-maturity-gates.go", reason: fmt.Sprintf("embedded immutable W restore publication fingerprint=%q want %q", slice62cRestoreCommitBodyFingerprint, immutableHash)})
				}
			}
		}
		if gotHash := canonicalSlice62cNodeHash(fset, commit.Body); gotHash != wantHash {
			findings = append(findings, finding{path: path, reason: fmt.Sprintf("live restore publication body hash=%s want immutable W %s", gotHash, wantHash)})
		}
	}
	if abort == nil {
		findings = append(findings, finding{path: path, reason: "missing reverse abort operation"})
	} else {
		var phases []string
		var reverseLoop bool
		ast.Inspect(abort.Body, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.AssignStmt:
				for _, left := range node.Lhs {
					text := renderSlice62aNode(fset, left)
					if text == "candidate.aborted" || text == "m.pending" {
						phases = append(phases, "assign:"+text)
					}
				}
			case *ast.ForStmt:
				if renderSlice62aNode(fset, node.Init) == "i := len(candidate.panes) - 1" && renderSlice62aNode(fset, node.Cond) == "i >= 0" && renderSlice62aNode(fset, node.Post) == "i--" {
					reverseLoop = true
				}
			case *ast.CallExpr:
				if selector, ok := node.Fun.(*ast.SelectorExpr); ok {
					text := renderSlice62aNode(fset, selector)
					switch text {
					case "m.sessions.abort", "detached.pane.close", "p.close":
						phases = append(phases, "call:"+text)
					}
				}
			}
			return true
		})
		want := []string{"assign:candidate.aborted", "assign:m.pending", "call:m.sessions.abort", "call:detached.pane.close", "call:p.close"}
		if !reverseLoop || strings.Join(phases, "|") != strings.Join(want, "|") {
			findings = append(findings, finding{path: path, reason: fmt.Sprintf("restore abort reverse-order phases=%v reverseLoop=%v want exact %v", phases, reverseLoop, want)})
		}
	}
	return findings
}

func checkSlice62cKnownDefects() []finding {
	const (
		root    = "internal/mux"
		path    = "internal/mux/mux_restore_characterization_test.go"
		tCommit = "e126a428330e3d13b309ccb03286f76a9f3e00a7"
	)
	wantExpiry := map[string]string{
		"TestKnownDefect_L3_02_RestoreAcceptsDifferentOwnerThread":  "expires Slice 3.1",
		"TestKnownDefect_L3_07_FreshSessionUsesObservedTerminalCWD": "expires Slice 4.3",
	}
	current, inventory, findings := slice62cReadKnownDefectBodies(root, wantExpiry)
	if !mapsEqualStringInt(inventory, map[string]int{
		"TestKnownDefect_L3_02_RestoreAcceptsDifferentOwnerThread":  1,
		"TestKnownDefect_L3_07_FreshSessionUsesObservedTerminalCWD": 1,
	}) {
		findings = append(findings, finding{path: path, reason: fmt.Sprintf("restore known-defect inventory=%v want exact L3-02/L3-07 tests", inventory)})
	}
	for name, body := range current {
		if body.path != path {
			findings = append(findings, finding{path: body.path, reason: name + " must remain in the immutable T characterization file"})
		}
		if !body.validTest {
			findings = append(findings, finding{path: body.path, reason: name + " must remain a top-level func with exactly one *testing.T parameter and no results"})
		}
	}
	immutableSource, immutableErr := gitText("show", tCommit+":"+path)
	expected := slice62cKnownDefectBodyHashes
	if immutableErr == nil {
		immutable, parseFindings := slice62cKnownDefectBodiesFromSource(path, immutableSource, wantExpiry)
		findings = append(findings, parseFindings...)
		expected = make(map[string]string, len(immutable))
		for name, body := range immutable {
			expected[name] = body.hash
			if embedded := slice62cKnownDefectBodyHashes[name]; embedded != body.hash {
				findings = append(findings, finding{path: "scripts/check-maturity-gates.go", reason: fmt.Sprintf("embedded immutable T fingerprint %s=%q want %q", name, embedded, body.hash)})
			}
		}
	}
	for name, wantHash := range expected {
		body, ok := current[name]
		if !ok {
			continue
		}
		if body.hash != wantHash {
			findings = append(findings, finding{path: body.path, reason: fmt.Sprintf("%s canonical behavior body hash=%s want immutable T %s", name, body.hash, wantHash)})
		}
	}
	return findings
}

func checkSlice62cCommitsAndPaths() []finding {
	const (
		base        = "005da8f19e0227e0a133b7d9b50dce4a191d66ee"
		tCommit     = "e126a428330e3d13b309ccb03286f76a9f3e00a7"
		aCommit     = "780a36658d9c02ea51ac259545e1e6a2cc25b55b"
		mCommit     = "3f75bf11f5440f00664665b08a85e07ceae27bdd"
		wCommit     = "62d3b959e7e1b3934a231ccf43bb0021661f10c3"
		gSubject    = "refactor(mux): guard restore coordinator delegation"
		sliceBranch = "arch/l3-01c-mux-restore-coordinator"
	)
	stages := []struct{ class, commit, parent, subject string }{
		{"T", tCommit, base, "test(mux): characterize restore transaction parity"},
		{"A", aCommit, tCommit, "refactor(mux): add restore coordinator seam"},
		{"M", mCommit, aCommit, "refactor(mux): split restore operation adapters"},
		{"W", wCommit, mCommit, "refactor(mux): wire restore coordinator"},
	}
	branch, _ := gitText("symbolic-ref", "--quiet", "--short", "HEAD")
	head, _ := gitText("rev-parse", "HEAD")
	worktree, _ := gitRawText("status", "--porcelain=v1", "--untracked-files=all")
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
			return checkSlice62cShallowFallback(branch == sliceBranch, head, wCommit, gCommit, gParent, worktree)
		}
	}
	var findings []finding
	for _, stage := range stages {
		identity, err := gitFields("show", "-s", "--format=%H%x00%P%x00%s", stage.commit)
		parent, parentErr := gitText("rev-parse", stage.parent+"^{commit}")
		if err != nil || len(identity) != 3 || parentErr != nil || identity[0] != stage.commit || identity[1] != parent || identity[2] != stage.subject {
			findings = append(findings, finding{path: "git:" + stage.class, reason: "unexpected exact Slice 6.2c identity/parent/subject"})
		}
	}
	gCommit, _ := findMaturitySliceCommit(gSubject, wCommit)
	end := gCommit
	includeWorktree := false
	if gCommit == "" {
		if head != wCommit {
			findings = append(findings, finding{path: "git:G", reason: "before Slice 6.2c G, HEAD must be exact immutable W 294b2f2"})
			return findings
		}
		end = wCommit
		includeWorktree = true
		if branch == sliceBranch {
			findings = append(findings, checkSlice62cPreGWorktree(worktree)...)
		}
	} else {
		identity, err := gitFields("show", "-s", "--format=%H%x00%P%x00%s", gCommit)
		if err != nil || len(identity) != 3 || identity[1] != wCommit || identity[2] != gSubject {
			findings = append(findings, finding{path: "git:G", reason: "G must have exact W parent and subject"})
		}
		findings = append(findings, checkSlice62cPostGActiveState(branch == sliceBranch, head, gCommit, worktree)...)
	}
	return append(findings, checkSlice62cExactPaths(base, end, includeWorktree)...)
}

func checkSlice62cExactPaths(base, end string, includeWorktree bool) []finding {
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
	return slice62cExactPathSetFindings(paths)
}

func slice62cExactPathSetFindings(paths []string) []finding {
	actual := make(map[string]bool, len(paths))
	for _, path := range paths {
		actual[filepath.ToSlash(path)] = true
	}
	expected := make(map[string]bool, len(slice62cAllowedPaths))
	var findings []finding
	for _, path := range slice62cAllowedPaths {
		expected[path] = true
		if !actual[path] {
			findings = append(findings, finding{path: path, reason: "missing from exact Slice 6.2c changed-path set"})
		}
	}
	for path := range actual {
		if !expected[path] {
			findings = append(findings, finding{path: path, reason: "outside exact Slice 6.2c changed-path allowlist"})
		}
	}
	return findings
}

func checkSlice62cPostGActiveState(active bool, head, gCommit, worktree string) []finding {
	if !active {
		return nil
	}
	var findings []finding
	if head != gCommit {
		findings = append(findings, finding{path: "git:G", reason: "after G exists on active Slice 6.2c branch, HEAD must equal G exactly"})
	}
	if strings.TrimSpace(worktree) != "" {
		findings = append(findings, finding{path: "git:worktree", reason: "after G exists on active Slice 6.2c branch, nonignored worktree must be clean"})
	}
	return findings
}

func slice62cFullHistoryPreGPathFindings(committedPaths []string, worktree string) []finding {
	worktreePaths, _ := slice62cPorcelainPaths(worktree)
	combined := append([]string(nil), committedPaths...)
	for path := range worktreePaths {
		combined = append(combined, path)
	}
	findings := slice62cExactPathSetFindings(combined)
	return append(findings, checkSlice62cPreGWorktree(worktree)...)
}

func checkSlice62cFullHistoryPreGSelfTest() []finding {
	validWorktree := slice62cSyntheticPorcelain(slice62cGStagePaths)
	if got := slice62cFullHistoryPreGPathFindings(slice62cWStagePaths, validWorktree); len(got) != 0 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2c full-history pre-G self-test rejected exact G-stage worktree"}}
	}
	omittedOverlap := make([]string, 0, len(slice62cGStagePaths)-1)
	for _, path := range slice62cGStagePaths {
		if path != "internal/mux/mux_restore_wiring_test.go" {
			omittedOverlap = append(omittedOverlap, path)
		}
	}
	got := slice62cFullHistoryPreGPathFindings(slice62cWStagePaths, slice62cSyntheticPorcelain(omittedOverlap))
	for _, item := range got {
		if item.path == "internal/mux/mux_restore_wiring_test.go" {
			return nil
		}
	}
	return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2c full-history pre-G self-test did not reject omitted G modification already present in W"}}
}

func checkSlice62cPostGPolicySelfTest() []finding {
	if got := checkSlice62cPostGActiveState(true, "later", "g", ""); len(got) != 1 || got[0].path != "git:G" {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2c post-G self-test did not reject active-branch commit after G"}}
	}
	if got := checkSlice62cPostGActiveState(true, "g", "g", " M dirty.go"); len(got) != 1 || got[0].path != "git:worktree" {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2c post-G self-test did not reject dirty active branch"}}
	}
	if got := checkSlice62cPostGActiveState(false, "later", "g", " M unrelated.go"); len(got) != 0 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2c post-G self-test rejected later main history"}}
	}
	return nil
}

func slice62cDocumentRequirements() []string {
	return []string{
		"| T | `bec3126` |", "| A | `d073df1` |", "| M | `fe91ce0` |", "| W | `294b2f2` |", "| G | pending |",
		"refactor(mux): guard restore coordinator delegation", "exact sole parent to be W",
		"L3-01 remains **partial**", "L3-02/L3-04/L3-07/L3-09/L3-10 remain open", "6.2d is deferred",
	}
}

func slice62cDocumentFindings(text string) []finding {
	const path = "docs/validation/architecture-maturity-slice-6.2c.md"
	var findings []finding
	for _, required := range slice62cDocumentRequirements() {
		if !strings.Contains(text, required) {
			findings = append(findings, finding{path: path, reason: "shallow checkout is missing documented commit/closure contract " + required})
		}
	}
	for _, allowed := range slice62cAllowedPaths {
		if !strings.Contains(text, "\n"+allowed+"\n") {
			findings = append(findings, finding{path: path, reason: "shallow checkout allowlist is missing " + allowed})
		}
	}
	return findings
}

func checkSlice62cDocumentedSequence() []finding {
	const path = "docs/validation/architecture-maturity-slice-6.2c.md"
	data, err := os.ReadFile(path)
	if err != nil {
		return []finding{{path: path, reason: err.Error()}}
	}
	return slice62cDocumentFindings(string(data))
}

func checkSlice62cShallowFallback(active bool, head, wCommit, gCommit, gParent, worktree string) []finding {
	const path = "docs/validation/architecture-maturity-slice-6.2c.md"
	data, err := os.ReadFile(path)
	if err != nil {
		return []finding{{path: path, reason: err.Error()}}
	}
	return slice62cShallowFallbackFindings(string(data), active, head, wCommit, gCommit, gParent, worktree)
}

func slice62cShallowFallbackFindings(document string, active bool, head, wCommit, gCommit, gParent, worktree string) []finding {
	findings := slice62cDocumentFindings(document)
	if gCommit != "" {
		if gParent == "" {
			if active {
				findings = append(findings, finding{path: "git:G", reason: "history-limited active slice cannot prove identifiable G parentage"})
			}
		} else if gParent != wCommit {
			findings = append(findings, finding{path: "git:G", reason: "history-limited G must expose exact immutable W as its sole parent"})
		}
		if active {
			findings = append(findings, checkSlice62cPostGActiveState(true, head, gCommit, worktree)...)
		}
		return findings
	}
	if !active {
		return findings
	}
	if head != wCommit {
		findings = append(findings, finding{path: "git:G", reason: "history-limited active Slice 6.2c branch without identifiable G must remain at documented immutable W"})
	}
	findings = append(findings, checkSlice62cPreGWorktree(worktree)...)
	return findings
}

func checkSlice62cCombinedShallowPostGSelfTest() []finding {
	valid := "\n" + strings.Join(slice62cDocumentRequirements(), "\n") + "\n" + strings.Join(slice62cAllowedPaths, "\n") + "\n"
	if got := slice62cDocumentFindings(valid); len(got) != 0 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2c shallow-doc self-test rejected complete synthetic contract"}}
	}
	missingContract := strings.Replace(valid, slice62cDocumentRequirements()[0], "", 1)
	if got := slice62cDocumentFindings(missingContract); len(got) != 1 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2c shallow-doc self-test did not reject missing identity"}}
	}
	missingPath := strings.Replace(valid, "\n"+slice62cAllowedPaths[0]+"\n", "\n", 1)
	if got := slice62cDocumentFindings(missingPath); len(got) != 1 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2c shallow-doc self-test did not reject missing allowlist path"}}
	}
	postG := slice62cShallowFallbackFindings(valid, true, "later", "w", "g", "w", " M dirty.go")
	if len(postG) != 2 || postG[0].path != "git:G" || postG[1].path != "git:worktree" {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2c combined shallow/post-G self-test did not enforce HEAD/clean policy"}}
	}
	if got := slice62cShallowFallbackFindings(valid, true, "g", "w", "g", "wrong-parent", ""); len(got) != 1 || got[0].path != "git:G" {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2c shallow post-G self-test did not reject wrong G parent"}}
	}
	if got := slice62cShallowFallbackFindings(valid, false, "later-main", "w", "", "", " M unrelated.go"); len(got) != 0 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2c shallow fallback rejected later main without G metadata"}}
	}
	if got := slice62cShallowFallbackFindings(valid, false, "later-main", "w", "g", "", " M unrelated.go"); len(got) != 0 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2c shallow fallback rejected later main with incomplete G metadata"}}
	}
	validPreGWorktree := slice62cSyntheticPorcelain(slice62cGStagePaths)
	if got := slice62cShallowFallbackFindings(valid, true, "later", "w", "", "", validPreGWorktree); len(got) != 1 || got[0].path != "git:G" {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2c shallow pre-G self-test did not reject commit beyond W"}}
	}
	if got := slice62cShallowFallbackFindings(valid, true, "w", "w", "", "", validPreGWorktree); len(got) != 0 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2c shallow pre-G self-test rejected exact G-stage worktree"}}
	}
	for _, fixture := range []struct {
		name     string
		worktree string
		wantPath string
	}{
		{name: "dirty outside path", worktree: validPreGWorktree + "\n M outside.go", wantPath: "outside.go"},
		{name: "staged outside path", worktree: validPreGWorktree + "\nM  staged-outside.go", wantPath: "staged-outside.go"},
		{name: "untracked outside path", worktree: validPreGWorktree + "\n?? untracked-outside.go", wantPath: "untracked-outside.go"},
		{name: "missing required G path", worktree: slice62cSyntheticPorcelain(slice62cGStagePaths[1:]), wantPath: slice62cGStagePaths[0]},
	} {
		got := checkSlice62cPreGWorktree(fixture.worktree)
		found := false
		for _, item := range got {
			if item.path == fixture.wantPath {
				found = true
				break
			}
		}
		if !found {
			return []finding{{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2c shallow pre-G self-test did not reject " + fixture.name}}
		}
	}
	return nil
}

func checkSlice62cBypassSelfTest() []finding {
	fixtures := []struct{ name, source string }{
		{"alias bypass", `package mux
			type restoreCoordinator[T, U any] struct{}
			type Mux struct { restoreCoordinator restoreCoordinator[int, int] }
			func (m *Mux) PrepareRestore() { alias := m.restoreCoordinator; alias.prepareRestore(nil) }`},
		{"alternate field", `package mux
			type restoreCoordinator[T, U any] struct{}
			type holder struct { coordinator restoreCoordinator[int, int] }`},
		{"direct adapter bypass", `package mux
			type muxRestorePublicationOperationAdapter struct{}
			func bypass(a muxRestorePublicationOperationAdapter) { a.commitRestore(nil) }`},
		{"nested alias and field retention", `package mux
			type restoreCoordinator[T, U any] struct{}
			type muxRestorePreparationOperationAdapter struct{}
			type coordinatorAlias = restoreCoordinator[int, int]
			type pointerAlias *coordinatorAlias
			type sliceAlias []pointerAlias
			type arrayAlias [1]muxRestorePreparationOperationAdapter
			type mapAlias map[string]struct { coordinators sliceAlias; adapters *arrayAlias }
			type holder struct { nested mapAlias; anonymous struct { hidden []coordinatorAlias } }`},
		{"inferred nested composite retention", `package mux
			type muxRestorePublicationOperationAdapter struct{}
			var hidden = map[string][]*muxRestorePublicationOperationAdapter{}`},
		{"exported retained seam", `package mux
			type restoreCoordinator[T, U any] struct{}
			type ExportedRestoreCoordinator = restoreCoordinator[int, int]
			func ExportRestoreCoordinator() ExportedRestoreCoordinator { return ExportedRestoreCoordinator{} }`},
		{"function literal parameter retention", `package mux
			type muxRestorePublicationOperationAdapter struct{}
			var escape = func(a muxRestorePublicationOperationAdapter) { _ = a }`},
		{"function literal result retention", `package mux
			type restoreCoordinator[T, U any] struct{}
			var escape = func() restoreCoordinator[int, int] { return restoreCoordinator[int, int]{} }`},
		{"function literal inferred body retention", `package mux
			type muxRestorePreparationOperationAdapter struct{}
			func bypass() { escape := func() { hidden := []any{muxRestorePreparationOperationAdapter{}}; _ = hidden }; _ = escape }`},
		{"escaping closure captures adapter", `package mux
			type muxRestorePublicationOperationAdapter struct{}
			func bypass(a muxRestorePublicationOperationAdapter) func() { return func() { _ = a } }`},
		{"local inferred adapter container", `package mux
			type muxRestorePreparationOperationAdapter struct{}
			func bypass(a muxRestorePreparationOperationAdapter) { hidden := []any{a}; _ = hidden }`},
		{"package-scope nested function literal composite retention", `package mux
			type muxRestorePublicationOperationAdapter struct{}
			var hidden = []any{func(adapter muxRestorePublicationOperationAdapter) { _ = adapter }}`},
		{"local nested function literal composite retention", `package mux
			type muxRestorePreparationOperationAdapter struct{}
			func bypass() { hidden := []any{func(adapter muxRestorePreparationOperationAdapter) { _ = adapter }}; _ = hidden }`},
	}
	var findings []finding
	for _, fixture := range fixtures {
		file, err := parser.ParseFile(token.NewFileSet(), "internal/mux/fixture.go", fixture.source, 0)
		if err != nil {
			findings = append(findings, finding{path: "scripts/check-maturity-gates.go", reason: "cannot parse Slice 6.2c bypass self-test " + fixture.name})
			continue
		}
		got := checkSlice62cSyntheticSurface(map[string]*ast.File{"internal/mux/fixture.go": file})
		if len(got) == 0 {
			findings = append(findings, finding{path: "scripts/check-maturity-gates.go", reason: "Slice 6.2c bypass self-test did not reject " + fixture.name})
		}
	}
	return findings
}

func checkSlice62cSyntheticSurface(files map[string]*ast.File) []finding {
	retainedTypeNames := slice62cRetainedTypeNames(files)
	var findings []finding
	for path, file := range files {
		findings = append(findings, slice62cFunctionRetentionFindings(path, file, retainedTypeNames)...)
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.StructType:
				for _, field := range node.Fields.List {
					if slice62bTypeExpressionRetains(field.Type, retainedTypeNames) {
						findings = append(findings, finding{path: path, reason: "restore controller/adapter retained recursively under alternate field"})
					}
				}
			case *ast.TypeSpec:
				if !slice62cCanonicalRetainedType(node.Name.Name) && node.Name.Name != "Mux" && retainedTypeNames[node.Name.Name] {
					findings = append(findings, finding{path: path, reason: "restore controller/adapter retained through alias or named container " + node.Name.Name})
				}
				if token.IsExported(node.Name.Name) && strings.Contains(strings.ToLower(node.Name.Name), "restorecoordinator") {
					findings = append(findings, finding{path: path, reason: "exported restore coordinator seam"})
				}
			case *ast.CompositeLit:
				if slice62bTypeExpressionRetains(node.Type, retainedTypeNames) {
					findings = append(findings, finding{path: path, reason: "restore controller/adapter retained recursively in inferred composite"})
				}
			case *ast.SelectorExpr:
				if node.Sel.Name == "restoreCoordinator" {
					findings = append(findings, finding{path: path, reason: "restore controller field alias/bypass"})
				}
				if _, reserved := slice62cExactMethodSignatures[node.Sel.Name]; reserved {
					findings = append(findings, finding{path: path, reason: "reserved restore controller/adapter method bypass " + node.Sel.Name})
				}
			case *ast.FuncDecl:
				if token.IsExported(node.Name.Name) && strings.Contains(strings.ToLower(node.Name.Name), "restorecoordinator") {
					findings = append(findings, finding{path: path, reason: "exported restore coordinator function"})
				}
			}
			return true
		})
	}
	return findings

}

type slice62cKnownDefectBody struct {
	path      string
	signature string
	validTest bool
	hash      string
}

func canonicalSlice62cNodeHash(fset *token.FileSet, node ast.Node) string {
	var rendered bytes.Buffer
	if err := format.Node(&rendered, fset, node); err != nil {
		return "format-error:" + err.Error()
	}
	sum := sha256.Sum256(rendered.Bytes())
	return fmt.Sprintf("%x", sum)
}

func slice62cValidKnownDefectTest(function *ast.FuncDecl) bool {
	if function == nil || function.Recv != nil || function.Body == nil {
		return false
	}
	if function.Type.TypeParams != nil && len(function.Type.TypeParams.List) != 0 {
		return false
	}
	params := function.Type.Params
	if params == nil || len(params.List) != 1 {
		return false
	}
	parameter := params.List[0]
	parameterCount := len(parameter.Names)
	if parameterCount == 0 {
		parameterCount = 1
	}
	if parameterCount != 1 {
		return false
	}
	pointer, ok := parameter.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := pointer.X.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "T" {
		return false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	if !ok || qualifier.Name != "testing" {
		return false
	}
	return function.Type.Results == nil || len(function.Type.Results.List) == 0
}

func slice62cKnownDefectBodiesFromSource(path, source string, wantExpiry map[string]string) (map[string]slice62cKnownDefectBody, []finding) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, parser.ParseComments)
	if err != nil {
		return nil, []finding{{path: path, reason: err.Error()}}
	}
	bodies := make(map[string]slice62cKnownDefectBody)
	var findings []finding
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || (!strings.HasPrefix(function.Name.Name, "TestKnownDefect_L3_02_Restore") && !strings.HasPrefix(function.Name.Name, "TestKnownDefect_L3_07_FreshSession")) {
			continue
		}
		expiry, exact := wantExpiry[function.Name.Name]
		comment := ""
		if function.Doc != nil {
			comment = function.Doc.Text()
		}
		if !exact || !strings.Contains(comment, expiry) {
			findings = append(findings, finding{path: path, reason: function.Name.Name + " must be one of the exact two restore known-defect tests with exact expiry"})
		}
		validTest := slice62cValidKnownDefectTest(function)
		if !validTest {
			findings = append(findings, finding{path: path, reason: function.Name.Name + " must be a top-level func with exactly one *testing.T parameter and no results"})
		}
		bodies[function.Name.Name] = slice62cKnownDefectBody{
			path: path, signature: renderSlice62aNode(fset, function.Type), validTest: validTest, hash: canonicalSlice62cNodeHash(fset, function.Body),
		}
	}
	if len(bodies) != 0 {
		for _, group := range file.Comments {
			for _, comment := range group.List {
				text := strings.TrimSpace(comment.Text)
				if strings.HasPrefix(text, "//go:build") || strings.HasPrefix(text, "// +build") {
					findings = append(findings, finding{path: path, reason: "restore known-defect characterization file must remain buildable without exclusion tags"})
				}
			}
		}
	}
	return bodies, findings
}

func slice62cReadKnownDefectBodies(root string, wantExpiry map[string]string) (map[string]slice62cKnownDefectBody, map[string]int, []finding) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, []finding{{path: root, reason: err.Error()}}
	}
	bodies := make(map[string]slice62cKnownDefectBody)
	inventory := make(map[string]int)
	var findings []finding
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.ToSlash(filepath.Join(root, entry.Name()))
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			findings = append(findings, finding{path: path, reason: readErr.Error()})
			continue
		}
		parsed, parseFindings := slice62cKnownDefectBodiesFromSource(path, string(data), wantExpiry)
		findings = append(findings, parseFindings...)
		for name, body := range parsed {
			inventory[name]++
			bodies[name] = body
		}
	}
	return bodies, inventory, findings
}

func checkSlice62cKnownDefectBodySelfTest() []finding {
	const baseline = `package mux
		// TestKnownDefect_L3_02_RestoreAcceptsDifferentOwnerThread expires Slice 3.1.
		func TestKnownDefect_L3_02_RestoreAcceptsDifferentOwnerThread(t *testing.T) { t.Fatal("characterization") }
		// TestKnownDefect_L3_07_FreshSessionUsesObservedTerminalCWD expires Slice 4.3.
		func TestKnownDefect_L3_07_FreshSessionUsesObservedTerminalCWD(t *testing.T) { if got := "observed"; got != "observed" { t.Fatal(got) } }`
	wantExpiry := map[string]string{
		"TestKnownDefect_L3_02_RestoreAcceptsDifferentOwnerThread":  "expires Slice 3.1",
		"TestKnownDefect_L3_07_FreshSessionUsesObservedTerminalCWD": "expires Slice 4.3",
	}
	want, findings := slice62cKnownDefectBodiesFromSource("baseline.go", baseline, wantExpiry)
	if len(findings) != 0 || len(want) != 2 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "known-defect body self-test could not establish baseline"}}
	}
	fixtures := []struct{ name, source string }{
		{name: "t.Skip", source: strings.Replace(baseline, `t.Fatal("characterization")`, `t.Skip("disabled")`, 1)},
		{name: "no-op", source: strings.Replace(baseline, `{ t.Fatal("characterization") }`, `{}`, 1)},
		{name: "behavior mutation", source: strings.Replace(baseline, `got != "observed"`, `got == "observed"`, 1)},
	}
	disabled := "//go:build never\n\n" + baseline
	if _, disabledFindings := slice62cKnownDefectBodiesFromSource("disabled.go", disabled, wantExpiry); len(disabledFindings) == 0 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "known-defect body self-test did not reject build-tag exclusion"}}
	}
	methodSource := strings.Replace(baseline, "package mux", "package mux\n\t\ttype knownDefectMethodFixture struct{}", 1)
	methodSource = strings.Replace(methodSource,
		"func TestKnownDefect_L3_02_RestoreAcceptsDifferentOwnerThread",
		"func (knownDefectMethodFixture) TestKnownDefect_L3_02_RestoreAcceptsDifferentOwnerThread", 1)
	methodBodies, methodFindings := slice62cKnownDefectBodiesFromSource("method.go", methodSource, wantExpiry)
	methodBody, methodFound := methodBodies["TestKnownDefect_L3_02_RestoreAcceptsDifferentOwnerThread"]
	if !methodFound || methodBody.hash != want["TestKnownDefect_L3_02_RestoreAcceptsDifferentOwnerThread"].hash {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "known-defect method self-test did not preserve the canonical behavior hash"}}
	}
	if len(methodFindings) == 0 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "known-defect body self-test did not reject a method with the preserved behavior hash"}}
	}
	signatureFixtures := []struct {
		name   string
		source string
	}{
		{name: "wrong parameter type", source: strings.Replace(baseline, "(t *testing.T)", "(t *testing.B)", 1)},
		{name: "multiple parameters", source: strings.Replace(baseline, "(t *testing.T)", "(t *testing.T, extra int)", 1)},
		{name: "function result", source: strings.Replace(baseline, "(t *testing.T)", "(t *testing.T) error", 1)},
	}
	for _, fixture := range signatureFixtures {
		got, signatureFindings := slice62cKnownDefectBodiesFromSource("signature.go", fixture.source, wantExpiry)
		body, ok := got["TestKnownDefect_L3_02_RestoreAcceptsDifferentOwnerThread"]
		if !ok || body.hash != want["TestKnownDefect_L3_02_RestoreAcceptsDifferentOwnerThread"].hash {
			return []finding{{path: "scripts/check-maturity-gates.go", reason: "known-defect signature self-test changed behavior hash: " + fixture.name}}
		}
		if len(signatureFindings) == 0 {
			return []finding{{path: "scripts/check-maturity-gates.go", reason: "known-defect signature self-test did not reject " + fixture.name}}
		}
	}
	for _, fixture := range fixtures {
		got, parseFindings := slice62cKnownDefectBodiesFromSource("fixture.go", fixture.source, wantExpiry)
		if len(parseFindings) != 0 {
			return []finding{{path: "scripts/check-maturity-gates.go", reason: "known-defect mutation fixture did not parse: " + fixture.name}}
		}
		rejected := false
		for name, wantBody := range want {
			if gotBody, ok := got[name]; !ok || gotBody.hash != wantBody.hash {
				rejected = true
				break
			}
		}
		if !rejected {
			return []finding{{path: "scripts/check-maturity-gates.go", reason: "known-defect body self-test did not reject " + fixture.name}}
		}
	}
	return nil
}

func slice62cRestoreCommitHashFromSource(path, source string) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, 0)
	if err != nil {
		return "", err
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "commitRestore" && strings.Join(renderedUnnamedFields(fset, function.Recv), "|") == "muxRestorePublicationOperationAdapter" {
			return canonicalSlice62cNodeHash(fset, function.Body), nil
		}
	}
	return "", fmt.Errorf("missing muxRestorePublicationOperationAdapter.commitRestore")
}

func slice62cRestoreCommitOrderFindings(fset *token.FileSet, path string, body *ast.BlockStmt) []finding {
	if body == nil {
		return []finding{{path: path, reason: "restore publication operation has no live body"}}
	}
	eventLoop := `for _, p := range candidate.panes {
	address := Event{Window: candidate.paneWindows[p.id], Tab: candidate.paneTabs[p.id], Pane: p.id}
	address.Workspace, _ = m.WorkspaceForWindow(address.Window)
	started, geometry := address, address
	started.Kind = PaneStarted
	geometry.Kind, geometry.Geometry = PaneGeometryChanged, p.geometry
	events = append(events, started, geometry)
}`
	activationAppend := `events = append(events,
	Event{Kind: WorkspaceActivated, Workspace: workspace.ID},
	Event{Kind: WindowActivated, Workspace: workspace.ID, Window: window},
	Event{Kind: TabActivated, Workspace: workspace.ID, Window: window, Tab: tab},
	Event{Kind: PaneFocused, Workspace: workspace.ID, Window: window, Tab: tab, Pane: pane},
)`
	readerFailure := `if err != nil {
	cleanupErr := m.abortRestore(candidate)
	return nil, errors.Join(fmt.Errorf("prepare restore readers: %w", err), cleanupErr)
}`
	paletteLoop := `for _, p := range candidate.panes {
	p.terminal.SetPaletteBase(m.paletteBase)
}`
	want := []string{
		"launchReaders, err := m.sessions.prepareStarts(ids)",
		readerFailure,
		paletteLoop,
		"m.model = candidate.model",
		"m.paneMetrics = candidate.paneMetrics",
		"m.bounds = candidate.bounds",
		"m.bootstrapped = true",
		"m.pending = nil",
		"candidate.committed = true",
		"events := make([]Event, 0, len(candidate.panes)*2+4)",
		eventLoop,
		"workspace := m.model.ActiveWorkspace()",
		"window := m.model.activeWindow",
		"tab := m.model.TabID()",
		"pane := m.model.FocusedPane()",
		activationAppend,
		"launchReaders()",
		"return events, nil",
	}
	actual := make([]string, 0, len(body.List))
	for _, statement := range body.List {
		actual = append(actual, renderSlice62aNode(fset, statement))
	}
	last := -1
	var findings []finding
	for _, required := range want {
		index := -1
		count := 0
		for candidateIndex, statement := range actual {
			if statement == required {
				index = candidateIndex
				count++
			}
		}
		if count != 1 {
			findings = append(findings, finding{path: path, reason: fmt.Sprintf("restore publication exact live top-level statement %q count=%d want=1", required, count)})
			continue
		}
		if index <= last {
			findings = append(findings, finding{path: path, reason: fmt.Sprintf("restore publication top-level statement %q is out of order", required)})
		}
		last = index
	}
	return findings
}

func checkSlice62cRestoreOrderSelfTest() []finding {
	const path = "internal/mux/mux_restore.go"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return []finding{{path: path, reason: err.Error()}}
	}
	var bodyText string
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "commitRestore" && strings.Join(renderedUnnamedFields(fset, function.Recv), "|") == "muxRestorePublicationOperationAdapter" {
			bodyText = renderSlice62aNode(fset, function.Body)
			break
		}
	}
	if bodyText == "" {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "restore-order self-test could not read live adapter body"}}
	}
	mutations := []struct{ name, old, replacement string }{
		{name: "reader launch before activation", old: "events = append(events,\n\t\tEvent{Kind: WorkspaceActivated", replacement: "launchReaders()\n\tevents = append(events,\n\t\tEvent{Kind: WorkspaceActivated"},
		{name: "dead-code reader launch", old: "launchReaders()\n\treturn events, nil", replacement: "if false {\n\t\tlaunchReaders()\n\t}\n\treturn events, nil"},
		{name: "event helper substitution", old: "events = append(events, started, geometry)", replacement: "events = append(events, buildRestoreEvents(started, geometry)... )"},
		{name: "geometry mutation", old: "PaneGeometryChanged, p.geometry", replacement: "PaneStarted, p.geometry"},
	}
	for _, mutation := range mutations {
		mutated := strings.Replace(bodyText, mutation.old, mutation.replacement, 1)
		if mutated == bodyText {
			return []finding{{path: "scripts/check-maturity-gates.go", reason: "restore-order mutation fixture did not apply: " + mutation.name}}
		}
		fixtureSet := token.NewFileSet()
		fixture, parseErr := parser.ParseFile(fixtureSet, "fixture.go", "package mux\nfunc fixture() "+mutated, 0)
		if parseErr != nil {
			return []finding{{path: "scripts/check-maturity-gates.go", reason: "restore-order mutation fixture did not parse: " + mutation.name + ": " + parseErr.Error()}}
		}
		function := fixture.Decls[0].(*ast.FuncDecl)
		if got := slice62cRestoreCommitOrderFindings(fixtureSet, "fixture.go", function.Body); len(got) == 0 {
			return []finding{{path: "scripts/check-maturity-gates.go", reason: "restore-order self-test did not reject " + mutation.name}}
		}
	}
	return nil
}

func checkSlice62cPreGWorktree(worktree string) []finding {
	actual, parseFindings := slice62cPorcelainPaths(worktree)
	expected := make(map[string]bool, len(slice62cGStagePaths))
	findings := append([]finding(nil), parseFindings...)
	for _, path := range slice62cGStagePaths {
		expected[path] = true
		if !actual[path] {
			findings = append(findings, finding{path: path, reason: "missing required dirty G-stage path in active pre-G worktree"})
		}
	}
	for path := range actual {
		if !expected[path] {
			findings = append(findings, finding{path: path, reason: "active pre-G worktree path is outside exact G-stage allowlist"})
		}
	}
	return findings
}

func slice62cPorcelainPaths(worktree string) (map[string]bool, []finding) {
	paths := make(map[string]bool)
	var findings []finding
	for _, raw := range strings.Split(strings.ReplaceAll(worktree, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if len(raw) < 4 || raw[2] != ' ' {
			findings = append(findings, finding{path: "git:worktree", reason: "malformed porcelain v1 entry " + strconv.Quote(raw)})
			continue
		}
		path := strings.TrimSpace(raw[3:])
		if strings.Contains(path, " -> ") {
			findings = append(findings, finding{path: path, reason: "renamed/copied paths are forbidden in exact pre-G G-stage worktree"})
			continue
		}
		if strings.HasPrefix(path, `"`) {
			unquoted, err := strconv.Unquote(path)
			if err != nil {
				findings = append(findings, finding{path: "git:worktree", reason: "malformed quoted porcelain path " + path})
				continue
			}
			path = unquoted
		}
		path = filepath.ToSlash(path)
		paths[path] = true
	}
	return paths, findings
}

func slice62cSyntheticPorcelain(paths []string) string {
	lines := make([]string, 0, len(paths))
	for index, path := range paths {
		status := " M "
		if index >= 2 {
			status = "?? "
		}
		lines = append(lines, status+path)
	}
	return strings.Join(lines, "\n")
}

func slice62cFunctionRetentionFindings(path string, file *ast.File, retainedTypeNames map[string]bool) []finding {
	var findings []finding
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			if declaration.Body == nil {
				continue
			}
			seed := make(map[string]bool)
			for _, fields := range []*ast.FieldList{declaration.Recv, declaration.Type.Params, declaration.Type.Results} {
				slice62cSeedRetainedNames(fields, retainedTypeNames, seed)
			}
			findings = append(findings, slice62cInferRetainedLocals(path, declaration.Body, retainedTypeNames, seed)...)
		case *ast.GenDecl:
			for _, item := range declaration.Specs {
				value, ok := item.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, expression := range value.Values {
					if slice62cValueExpressionRetains(expression, retainedTypeNames, nil) {
						findings = append(findings, finding{path: path, reason: "package-scope value recursively retains/captures restore controller or adapter"})
					}
				}
			}
		}
	}
	return findings
}

func slice62cSeedRetainedNames(fields *ast.FieldList, retainedTypeNames, values map[string]bool) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		if !slice62bTypeExpressionRetains(field.Type, retainedTypeNames) {
			continue
		}
		for _, name := range field.Names {
			values[name.Name] = true
		}
	}
}

func slice62cInferRetainedLocals(path string, body *ast.BlockStmt, retainedTypeNames, seed map[string]bool) []finding {
	values := make(map[string]bool, len(seed))
	for name := range seed {
		values[name] = true
	}
	reported := make(map[string]bool)
	var findings []finding
	for changed := true; changed; {
		changed = false
		ast.Inspect(body, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.AssignStmt:
				for index, left := range node.Lhs {
					identifier, ok := left.(*ast.Ident)
					if !ok || identifier.Name == "_" || values[identifier.Name] || len(node.Rhs) == 0 {
						continue
					}
					right := node.Rhs[0]
					if len(node.Rhs) == len(node.Lhs) {
						right = node.Rhs[index]
					}
					if slice62cValueExpressionRetains(right, retainedTypeNames, values) {
						values[identifier.Name] = true
						changed = true
						key := fmt.Sprintf("assign:%d:%s", node.Pos(), identifier.Name)
						if !reported[key] {
							reported[key] = true
							findings = append(findings, finding{path: path, reason: "local inferred restore controller/adapter retention in " + identifier.Name})
						}
					}
				}
			case *ast.DeclStmt:
				generic, ok := node.Decl.(*ast.GenDecl)
				if !ok {
					break
				}
				for _, item := range generic.Specs {
					value, ok := item.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for index, name := range value.Names {
						if values[name.Name] {
							continue
						}
						retains := value.Type != nil && slice62bTypeExpressionRetains(value.Type, retainedTypeNames)
						if !retains && len(value.Values) != 0 {
							expression := value.Values[0]
							if len(value.Values) == len(value.Names) {
								expression = value.Values[index]
							}
							retains = slice62cValueExpressionRetains(expression, retainedTypeNames, values)
						}
						if retains {
							values[name.Name] = true
							changed = true
							key := fmt.Sprintf("decl:%d:%s", node.Pos(), name.Name)
							if !reported[key] {
								reported[key] = true
								findings = append(findings, finding{path: path, reason: "local restore controller/adapter container retention in " + name.Name})
							}
						}
					}
				}
			}
			return true
		})
	}
	ast.Inspect(body, func(node ast.Node) bool {
		literal, ok := node.(*ast.FuncLit)
		if !ok {
			return true
		}
		if slice62cFuncLitRetains(literal, retainedTypeNames, values) {
			key := fmt.Sprintf("funclit:%d", literal.Pos())
			if !reported[key] {
				reported[key] = true
				findings = append(findings, finding{path: path, reason: "function literal parameter/result/body retains or captures restore controller/adapter"})
			}
		}
		return true
	})
	return findings
}

func slice62cFuncLitRetains(literal *ast.FuncLit, retainedTypeNames, outerValues map[string]bool) bool {
	values := make(map[string]bool)
	for name := range outerValues {
		values[name] = true
	}
	slice62cSeedRetainedNames(literal.Type.Params, retainedTypeNames, values)
	slice62cSeedRetainedNames(literal.Type.Results, retainedTypeNames, values)
	if slice62bTypeExpressionRetains(literal.Type, retainedTypeNames) {
		return true
	}
	found := false
	ast.Inspect(literal.Body, func(node ast.Node) bool {
		if found {
			return false
		}
		switch node := node.(type) {
		case *ast.Ident:
			found = values[node.Name]
		case *ast.CompositeLit:
			found = slice62bTypeExpressionRetains(node.Type, retainedTypeNames)
		case *ast.FuncType:
			found = slice62bTypeExpressionRetains(node, retainedTypeNames)
		case *ast.SelectorExpr:
			found = node.Sel.Name == "restoreCoordinator"
		}
		return !found
	})
	return found
}

func slice62cValueExpressionRetains(expression ast.Expr, retainedTypeNames, values map[string]bool) bool {
	if expression == nil {
		return false
	}
	switch expression := expression.(type) {
	case *ast.Ident:
		return values[expression.Name]
	case *ast.CompositeLit:
		if slice62bTypeExpressionRetains(expression.Type, retainedTypeNames) {
			return true
		}
		for _, element := range expression.Elts {
			if slice62cValueExpressionRetains(element, retainedTypeNames, values) {
				return true
			}
		}
	case *ast.FuncLit:
		return slice62cFuncLitRetains(expression, retainedTypeNames, values)
	case *ast.KeyValueExpr:
		return slice62cValueExpressionRetains(expression.Key, retainedTypeNames, values) || slice62cValueExpressionRetains(expression.Value, retainedTypeNames, values)
	case *ast.ParenExpr:
		return slice62cValueExpressionRetains(expression.X, retainedTypeNames, values)
	case *ast.UnaryExpr:
		return slice62cValueExpressionRetains(expression.X, retainedTypeNames, values)
	case *ast.BinaryExpr:
		return slice62cValueExpressionRetains(expression.X, retainedTypeNames, values) || slice62cValueExpressionRetains(expression.Y, retainedTypeNames, values)
	case *ast.IndexExpr:
		return slice62cValueExpressionRetains(expression.X, retainedTypeNames, values) || slice62cValueExpressionRetains(expression.Index, retainedTypeNames, values)
	case *ast.IndexListExpr:
		if slice62cValueExpressionRetains(expression.X, retainedTypeNames, values) {
			return true
		}
		for _, index := range expression.Indices {
			if slice62cValueExpressionRetains(index, retainedTypeNames, values) {
				return true
			}
		}
	case *ast.SliceExpr:
		return slice62cValueExpressionRetains(expression.X, retainedTypeNames, values)
	case *ast.TypeAssertExpr:
		return slice62cValueExpressionRetains(expression.X, retainedTypeNames, values) || slice62bTypeExpressionRetains(expression.Type, retainedTypeNames)
	case *ast.CallExpr:
		for _, argument := range expression.Args {
			if slice62cValueExpressionRetains(argument, retainedTypeNames, values) {
				return true
			}
		}
	case *ast.SelectorExpr:
		return expression.Sel.Name == "restoreCoordinator"
	}
	return false
}

type slice55aStage struct {
	class, commit, parent, subject string
	paths                          []string
}

var slice55aStages = []slice55aStage{
	{class: "T", commit: "35243d7f7672f28ddb39ac55cd404e5fa96ed990", parent: "c027fd1228af792203d3361a671a9b01017b2e23", subject: "test(fontglyph): characterize discovery and cache extraction", paths: []string{
		"internal/fontglyph/discovery_cache_characterization_test.go",
	}},
	{class: "A", commit: "2485ea931a8c1a781b17cc26851ff325c0c7ceb2", parent: "35243d7f7672f28ddb39ac55cd404e5fa96ed990", subject: "refactor(fontglyph): add discovery cache and face seams", paths: []string{
		"internal/fontglyph/cache/contracts.go",
		"internal/fontglyph/cache/contracts_test.go",
		"internal/fontglyph/discovery/contracts.go",
		"internal/fontglyph/discovery/contracts_test.go",
		"internal/fontglyph/internal/face/owner.go",
		"internal/fontglyph/internal/face/owner_test.go",
	}},
	{class: "M", commit: "1ecc9cda5cccedb86ecca8d5a8803143c88d11d5", parent: "2485ea931a8c1a781b17cc26851ff325c0c7ceb2", subject: "refactor(fontglyph): copy discovery and cache implementations", paths: []string{
		"internal/fontglyph/cache/benchmark_test.go",
		"internal/fontglyph/cache/cache.go",
		"internal/fontglyph/cache/cache_test.go",
		"internal/fontglyph/cache/contracts.go",
		"internal/fontglyph/discovery/benchmark_test.go",
		"internal/fontglyph/discovery/index.go",
		"internal/fontglyph/discovery/index_test.go",
	}},
	{class: "W", commit: "28326fa5bb05850a0d12c31afe0f334fa329b636", parent: "1ecc9cda5cccedb86ecca8d5a8803143c88d11d5", subject: "refactor(fontglyph): wire discovery and parsed-face cache", paths: []string{
		"internal/fontglyph/backend.go",
		"internal/fontglyph/cache/cache.go",
		"internal/fontglyph/cache/cache_test.go",
		"internal/fontglyph/cache_facade.go",
		"internal/fontglyph/descriptor_backend_test.go",
		"internal/fontglyph/discovery/index.go",
		"internal/fontglyph/discovery/index_test.go",
		"internal/fontglyph/discovery_cache_characterization_test.go",
		"internal/fontglyph/face_resolver.go",
		"internal/fontglyph/face_resolver_test.go",
		"internal/fontglyph/fallback_resolver_test.go",
		"internal/fontglyph/font_cache.go",
		"internal/fontglyph/font_cache_test.go",
		"internal/fontglyph/font_install.go",
		"internal/fontglyph/fontindex.go",
		"internal/fontglyph/fontindex_test.go",
	}},
}

var slice55aGStagePaths = []string{
	"docs/architecture-maturity/implementation-plan.md",
	"docs/architecture.md",
	"docs/validation/architecture-maturity-slice-5.5a.md",
	"docs/validation/architecture-maturity-slice-5.5a/benchmark-binaries.txt",
	"docs/validation/architecture-maturity-slice-5.5a/benchmark-summary.txt",
	"docs/validation/architecture-maturity-slice-5.5a/benchmarks-base.txt",
	"docs/validation/architecture-maturity-slice-5.5a/benchmarks-candidate.txt",
	"docs/validation/architecture-maturity-slice-5.5a/benchmarks-interleaved.txt",
	"docs/validation/architecture-maturity-slice-5.5a/gates.txt",
	"docs/validation/architecture-maturity-slice-5.5a/platform-gates.txt",
	"docs/validation/architecture-maturity-slice-5.5a/scope-and-commits.txt",
	"docs/validation/architecture-maturity-slice-5.5a/source-manifest-base.txt",
	"docs/validation/architecture-maturity-slice-5.5a/source-manifest-candidate.txt",
	"internal/fontglyph/discovery/index.go",
	"internal/fontglyph/discovery_cache_characterization_test.go",
	"internal/fontglyph/face_resolver.go",
	"internal/fontglyph/font_cache.go",
	"internal/fontglyph/font_cache_test.go",
	"internal/fontglyph/font_install.go",
	"internal/fontglyph/fontindex.go",
	"internal/fontglyph/fontindex_test.go",
	"scripts/check-maturity-gates.go",
	"scripts/check-slice55a-evidence.go",
}

func checkSlice55aGuard() []finding {
	var findings []finding
	findings = append(findings, checkSlice55aPackageDAG()...)
	findings = append(findings, checkSlice55aOwnershipFacadeAndRemoval()...)
	findings = append(findings, checkSlice55aGuardSelfTests()...)
	findings = append(findings, checkSlice55aDedicatedGuard()...)
	findings = append(findings, checkSlice55aEvidence()...)
	findings = append(findings, checkSlice55aCommitsAndPaths()...)
	return findings
}

func checkSlice55aDedicatedGuard() []finding {
	command := exec.Command("go", "run", "./scripts/check-slice55a-evidence.go")
	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	text := strings.TrimSpace(string(output))
	if len(text) > 4_000 {
		text = text[:4_000] + "..."
	}
	return []finding{{path: "scripts/check-slice55a-evidence.go", reason: text}}
}

func checkSlice55aPackageDAG() []finding {
	allowed := map[string]map[string]bool{
		"internal/fontglyph/discovery": {"cervterm/internal/fontdesc": true},
		"internal/fontglyph/cache":     {"cervterm/internal/fontglyph/internal/face": true},
		"internal/fontglyph/shape": {
			"cervterm/internal/fontglyph/internal/face": true, "cervterm/internal/fontdesc": true,
			"cervterm/internal/unicodecluster": true, "cervterm/internal/unicodeprops": true,
		},
		"internal/fontglyph/raster": {
			"cervterm/internal/fontglyph/internal/face": true, "cervterm/internal/fontdesc": true,
			"cervterm/internal/unicodecluster": true, "cervterm/internal/unicodeprops": true,
		},
		"internal/fontglyph/platform": {
			"cervterm/internal/fontglyph/internal/face": true, "cervterm/internal/fontdesc": true,
			"cervterm/internal/unicodecluster": true, "cervterm/internal/unicodeprops": true,
		},
		"internal/fontglyph/internal/face": {"cervterm/internal/fontdesc": true},
	}
	required := map[string]bool{
		"internal/fontglyph/discovery":     true,
		"internal/fontglyph/cache":         true,
		"internal/fontglyph/internal/face": true,
	}
	var findings []finding
	for dir, edges := range allowed {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) && !required[dir] {
			continue
		}
		if err != nil {
			findings = append(findings, finding{path: dir, reason: err.Error()})
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.ToSlash(filepath.Join(dir, entry.Name()))
			file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if parseErr != nil {
				findings = append(findings, finding{path: path, reason: "cannot parse imports: " + parseErr.Error()})
				continue
			}
			for _, imported := range file.Imports {
				value, unquoteErr := strconv.Unquote(imported.Path.Value)
				if unquoteErr != nil || !strings.HasPrefix(value, "cervterm/internal/") {
					continue
				}
				if !edges[value] {
					findings = append(findings, finding{path: path, reason: "ADR-0021 forbids local import " + value})
				}
			}
		}
	}
	command := exec.Command("go", "list", "-deps", "-test", "./internal/fontglyph/...")
	if output, err := command.CombinedOutput(); err != nil {
		findings = append(findings, finding{path: "internal/fontglyph/...", reason: "package cycle/dependency check failed: " + strings.TrimSpace(string(output))})
	}
	return findings
}

type slice55aExactDecl struct {
	path, kind, name, source string
}

func checkSlice55aOwnershipFacadeAndRemoval() []finding {
	exact := []slice55aExactDecl{
		{path: "internal/fontglyph/fontindex.go", kind: "type", name: "FontIndexDiagnostics", source: `type FontIndexDiagnostics struct { Roots int; CandidateFiles int; SelectedFiles int; FilesTruncated int; FacesExamined int; FacesIndexed int; FacesTruncated int; FilesSkipped int; DuplicateFiles int; SymlinkDirectoriesSkipped int; SymlinkFilesSkipped int }`},
		{path: "internal/fontglyph/fontindex.go", kind: "type", name: "FontResolution", source: `type FontResolution struct { Configured string; Found bool; Regular string; Bold string; Italic string; BoldItalic string; FaceIndex int; RegularFaceIndex int; BoldFaceIndex int; ItalicFaceIndex int; BoldItalicFaceIndex int }`},
		{path: "internal/fontglyph/cache/contracts.go", kind: "type", name: "Lease", source: `type Lease[T any] struct { once sync.Once; manager *Manager[T]; entry *entry[T] }`},
		{path: "internal/fontglyph/cache/contracts.go", kind: "func", name: "Close", source: `func (l *Lease[T]) Close() { if l == nil { return }; l.once.Do(func() { manager := l.manager; manager.mu.Lock(); if l.entry.pins > 0 { l.entry.pins-- }; manager.mu.Unlock() }) }`},
		{path: "internal/fontglyph/internal/face/owner.go", kind: "type", name: "Owner", source: `type Owner[T any] struct { value T; closeOnce sync.Once; close func(T) }`},
		{path: "internal/fontglyph/internal/face/owner.go", kind: "func", name: "Close", source: `func (o *Owner[T]) Close() { if o == nil { return }; o.closeOnce.Do(func() { if o.close != nil { o.close(o.value) } }) }`},
	}
	var findings []finding
	for _, item := range exact {
		findings = append(findings, slice55aCheckExactDecl(item)...)
	}
	findings = append(findings, slice55aCheckExactInventory("internal/fontglyph/font_cache.go", map[string]bool{
		"type:parsedFontData": true, "value:errFontCacheCapacity": true, "value:errFontFileGrew": true, "func:parseFontData": true,
	})...)
	budget, err := os.ReadFile("internal/fontdesc/budget.go")
	if err != nil {
		findings = append(findings, finding{path: "internal/fontdesc/budget.go", reason: err.Error()})
	} else {
		for _, pin := range []string{"MaxDiscoveryFiles               = 20_000", "MaxDiscoveryFaces               = 65_536", "MaxFacesPerFile                 = 256", "MaxParsedFaces                  = 128", "MaxParsedBytes            int64 = 256 * 1024 * 1024"} {
			if strings.Count(string(budget), pin) != 1 {
				findings = append(findings, finding{path: "internal/fontdesc/budget.go", reason: "Slice 5.5a budget pin changed: " + pin})
			}
		}
	}
	findings = append(findings, slice55aCheckRootAntiRetention()...)
	return findings
}

func slice55aCheckExactDecl(item slice55aExactDecl) []finding {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, item.path, nil, 0)
	if err != nil {
		return []finding{{path: item.path, reason: err.Error()}}
	}
	wantFile, err := parser.ParseFile(token.NewFileSet(), "expected.go", "package p\n"+item.source, 0)
	if err != nil || len(wantFile.Decls) != 1 {
		return []finding{{path: "scripts/check-maturity-gates.go", reason: "invalid exact declaration fixture " + item.path + ":" + item.name}}
	}
	want := compactSlice62bGoText(renderSlice62aNode(token.NewFileSet(), wantFile.Decls[0]))
	count := 0
	for _, declaration := range file.Decls {
		match := false
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			match = item.kind == "func" && declaration.Name.Name == item.name
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if ok && item.kind == "type" && typeSpec.Name.Name == item.name {
					match = true
				}
			}
		}
		if !match {
			continue
		}
		count++
		got := compactSlice62bGoText(renderSlice62aNode(fset, declaration))
		if got != want {
			return []finding{{path: item.path, reason: "exact Slice 5.5a declaration/body changed: " + item.name}}
		}
	}
	if count != 1 {
		return []finding{{path: item.path, reason: fmt.Sprintf("exact Slice 5.5a declaration %s count=%d want 1", item.name, count)}}
	}
	return nil
}

func slice55aCheckExactInventory(path string, expected map[string]bool) []finding {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return []finding{{path: path, reason: err.Error()}}
	}
	actual := make(map[string]bool)
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			actual["func:"+declaration.Name.Name] = true
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					actual["type:"+spec.Name.Name] = true
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						actual["value:"+name.Name] = true
					}
				}
			}
		}
	}
	if len(actual) != len(expected) {
		return []finding{{path: path, reason: fmt.Sprintf("top-level inventory=%v want exact %v", actual, expected)}}
	}
	for key := range expected {
		if !actual[key] {
			return []finding{{path: path, reason: "missing exact top-level declaration " + key}}
		}
	}
	return nil
}

func slice55aCheckRootAntiRetention() []finding {
	files := make(map[string]*ast.File)
	var findings []finding
	entries, err := os.ReadDir("internal/fontglyph")
	if err != nil {
		return []finding{{path: "internal/fontglyph", reason: err.Error()}}
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		path := filepath.ToSlash(filepath.Join("internal/fontglyph", entry.Name()))
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			findings = append(findings, finding{path: path, reason: parseErr.Error()})
			continue
		}
		files[path] = file
	}
	for path, file := range files {
		findings = append(findings, slice55aAntiRetentionFindings(path, file)...)
	}
	return findings
}

func slice55aAntiRetentionFindings(path string, file *ast.File) []finding {
	forbiddenTypes := map[string]bool{
		"fontCacheState": true, "fontSourceBlob": true, "fontCacheEntry": true, "fontCacheManager": true, "fontCacheStats": true, "parsedFontHandle": true,
		"legacyFaceInfo": true, "legacylegacyFontIndexDiagnostics": true, "legacyFontIndex": true, "legacyFontResolution": true,
		"selectedPath": true, "maxPathHeap": true, "topKPathSelector": true,
	}
	forbiddenTypes = slice62bExpandRetainedTypeNames(map[string]*ast.File{path: file}, forbiddenTypes)
	forbiddenFunctions := map[string]bool{
		"newFontCacheManager": true, "legacyCanonicalFontCacheSource": true, "legacyFontCacheKey": true, "checkedAddInt64": true, "readFontFileBounded": true,
		"legacyBuildlegacyFontIndex": true, "canonicalDiscoveryRoots": true, "discoveryPathKey": true, "compareDiscoveryPaths": true, "pathWithinRoots": true,
		"addDiscoveryCandidate": true, "newTopKPathSelector": true, "selectTopKPaths": true, "fontFaces": true, "fontFacesBounded": true, "readFaceMetadata": true,
		"readFontRange": true, "legacySystemFontDirs": true, "legacyLoadSystemlegacyFontIndex": true, "legacyResolveSystemFont": true, "fontName": true, "isFontFile": true, "classifySubfamily": true,
	}
	forbiddenTests := map[string]bool{
		"TestFontNameExtraction": true, "TestNormalizeFamily": true, "TestFontIndexLookup": true, "TestSelectTopKPathsIndependentOfTraversalOrder": true,
		"TestBuildFontIndexCanonicalizesAndDeduplicatesRoots": true, "TestBuildFontIndexSymlinkPolicy": true, "TestTTCMultiFaceIndexing": true,
		"TestGoMonoFaceMetadataFromOS2": true, "TestOS2NumericMetadataAndDefaults": true, "TestOS2ReservedObliqueBitIgnoredBeforeVersion4": true,
		"TestTTCFacesHaveIndependentOS2Metadata": true, "TestCorruptOS2BoundsSkipOnlyFace": true, "TestFontParseCacheConcurrentMissSingleLoad": true,
		"TestFontParseCacheFailureWakesWaitersAndRetries": true, "TestFontParseCachePinLRUAndBackendCloseIdempotent": true, "TestFontParseCachePinnedCapacityRefusal": true,
		"TestFontParseCacheOversizedAndOverflow": true, "TestFontParseOccursOutsideCacheLock": true, "TestFontParseCacheWaiterKeepsPublishedEntryPinned": true,
		"TestFontParseCacheUnknownSizeAdmissionIsPessimistic": true, "TestFontParseCacheSharesSourceBlobAcrossIndices": true,
		"TestFontParseCacheConcurrentDifferentIndicesShareLoad": true, "TestFontParseCacheSharedLoadFailureWakesIndicesAndRetries": true, "TestReadFontFileBoundedRejectsGrowthAndOversize": true,
	}
	var findings []finding
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			if forbiddenFunctions[declaration.Name.Name] || forbiddenTests[declaration.Name.Name] {
				findings = append(findings, finding{path: path, reason: "obsolete root discovery/cache declaration retained: " + declaration.Name.Name})
			}
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if ok && forbiddenTypes[typeSpec.Name.Name] {
					findings = append(findings, finding{path: path, reason: "obsolete root authority retained recursively through " + typeSpec.Name.Name})
				}
			}
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if ok && slice62bTypeExpressionRetains(literal.Type, forbiddenTypes) {
			findings = append(findings, finding{path: path, reason: "obsolete root authority retained through inferred composite literal"})
		}
		return true
	})
	for _, item := range slice62cFunctionRetentionFindings(path, file, forbiddenTypes) {
		item.reason = "obsolete root authority retained through inferred value/function literal/composite"
		findings = append(findings, item)
	}
	return findings
}

func checkSlice55aGuardSelfTests() []finding {
	fixtures := []struct{ name, source string }{
		{name: "transitive alias", source: `package fontglyph; type hidden = fontCacheManager`},
		{name: "generic container", source: `package fontglyph; type box[T any] struct{ value T }; type hidden = box[fontCacheManager]`},
		{name: "inferred alias", source: `package fontglyph; func f(x fontCacheManager) { y := x; _ = y }`},
		{name: "function literal", source: `package fontglyph; var f = func(x fontCacheManager) fontCacheManager { return x }`},
		{name: "inferred composite", source: `package fontglyph; func f() { _ = struct{ value fontCacheManager }{} }`},
	}
	var findings []finding
	for _, fixture := range fixtures {
		file, err := parser.ParseFile(token.NewFileSet(), "internal/fontglyph/fixture.go", fixture.source, 0)
		if err != nil {
			findings = append(findings, finding{path: "scripts/check-maturity-gates.go", reason: "cannot parse Slice 5.5a anti-retention fixture " + fixture.name})
			continue
		}
		if got := slice55aAntiRetentionFindings("internal/fontglyph/fixture.go", file); len(got) == 0 {
			findings = append(findings, finding{path: "scripts/check-maturity-gates.go", reason: "Slice 5.5a anti-retention self-test accepted " + fixture.name})
		}
	}
	mutated := slice55aExactDecl{path: "internal/fontglyph/fontindex.go", kind: "func", name: "BuildFontIndex", source: `func BuildFontIndex(dirs []string) *FontIndex { return nil }`}
	if len(slice55aCheckExactDecl(mutated)) == 0 {
		findings = append(findings, finding{path: "scripts/check-maturity-gates.go", reason: "Slice 5.5a facade-body self-test accepted mutation"})
	}
	duplicateSiblings := "g-one\x00w-parent\x00guard subject\ng-two\x00w-parent\x00guard subject"
	if _, err := uniqueMaturitySliceCommit(duplicateSiblings, "guard subject", "w-parent"); err == nil {
		findings = append(findings, finding{path: "scripts/check-maturity-gates.go", reason: "Slice 5.5a full-history lookup accepted duplicate G siblings"})
	}
	return findings
}

func checkSlice55aEvidence() []finding {
	required := map[string][]string{
		"docs/validation/architecture-maturity-slice-5.5a.md":                         {"T -> A -> M -> W -> G", "Status: closed", "128 faces", "256 MiB", "20,000 files", "65,536 faces", "10 interleaved samples", "headless proxy", "No GUI claim", "Package map"},
		"docs/validation/architecture-maturity-slice-5.5a/scope-and-commits.txt":      {"320deef1ecb16db212cfee692128591359bebc70", "295ef3f847c2be13f20fee250aeb06d39b67ccc7", "G status: closed", "refactor(fontglyph): guard discovery and cache extraction"},
		"docs/validation/architecture-maturity-slice-5.5a/benchmarks-interleaved.txt": {"sample=10 side=base", "sample=10 side=candidate", "BenchmarkL402DiscoveryTopK", "BenchmarkL402BuildIndexGoMono", "BenchmarkL402CacheHitLease", "BenchmarkPhase15TerminalStartupMemory", "BenchmarkPhase13TextOnlySnapshot", "BenchmarkPhase13DisabledDraw"},
		"docs/validation/architecture-maturity-slice-5.5a/benchmark-binaries.txt":     {"schema=2", "base_production_commit=320deef1ecb16db212cfee692128591359bebc70", "base_manifest_sha256=", "candidate_manifest_sha256=", "compile side=candidate name=discovery package=./internal/fontglyph/discovery", "run side=candidate name=fontglyph package=./internal/fontglyph", "GOMAXPROCS=1+go+test+-c+-trimpath"},
		"docs/validation/architecture-maturity-slice-5.5a/benchmark-summary.txt":      {"threshold_percent=3.000000", "result=PASS"},
		"docs/validation/architecture-maturity-slice-5.5a/platform-gates.txt":         {"linux_mode=execution", "platform name=linux_cache mode=execution", "platform name=linux_discovery mode=execution", "platform name=darwin_amd64 mode=compile-only", "platform name=darwin_arm64 mode=compile-only", "platform name=windows_amd64 mode=compile-only", "platform name=windows_arm64 mode=compile-only"},
		"docs/validation/architecture-maturity-slice-5.5a/gates.txt":                  {"go test ./...", "go vet ./...", "go run ./scripts/check-maturity-gates.go", "go run ./scripts/check-slice55a-evidence.go", "-tags glfw", "-race"},
	}
	var findings []finding
	for path, pins := range required {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		} // required docs and changed-path checks report absence.
		for _, pin := range pins {
			if !strings.Contains(string(data), pin) {
				findings = append(findings, finding{path: path, reason: "missing evidence pin: " + pin})
			}
		}
	}
	return findings
}

func checkSlice55aCommitsAndPaths() []finding {
	const base = "c027fd1228af792203d3361a671a9b01017b2e23"
	const wCommit = "28326fa5bb05850a0d12c31afe0f334fa329b636"
	const gSubject = "refactor(fontglyph): guard discovery and cache extraction"
	var findings []finding
	shallowText, _ := gitText("rev-parse", "--is-shallow-repository")
	shallow := shallowText == "true"
	for _, stage := range slice55aStages {
		if !slice55aCommitExists(stage.commit) {
			if !shallow {
				findings = append(findings, finding{path: "git:" + stage.class, reason: "pinned commit is unavailable in non-shallow history"})
			}
			continue
		}
		identity, err := gitFields("show", "-s", "--format=%H%x00%P%x00%s", stage.commit)
		if err != nil || len(identity) != 3 || identity[0] != stage.commit || identity[1] != stage.parent || identity[2] != stage.subject {
			findings = append(findings, finding{path: "git:" + stage.class, reason: "unexpected exact Slice 5.5a identity/parent/subject"})
		}
		changed, err := gitText("diff-tree", "--no-commit-id", "--name-only", "-r", stage.commit)
		if err == nil {
			findings = append(findings, slice55aComparePathSet("git:"+stage.class, nonEmptyLines(changed), stage.paths)...)
		}
	}
	head, _ := gitText("rev-parse", "HEAD")
	branch, _ := gitText("branch", "--show-current")
	active := branch == "arch/l4-02a-font-discovery-cache"
	worktree, _ := gitRawText("status", "--porcelain=v1", "--untracked-files=all")
	gCommit, gLookupErr := findMaturitySliceCommit(gSubject, wCommit)
	if gLookupErr != nil && strings.Contains(gLookupErr.Error(), "cardinality") && !strings.Contains(gLookupErr.Error(), " is 0,") {
		findings = append(findings, finding{path: "git:G", reason: gLookupErr.Error()})
	}
	if gCommit == "" && shallow {
		subject, _ := gitText("show", "-s", "--format=%s", head)
		parent, _ := slice55aCommitParent(head)
		if subject == gSubject && parent == wCommit {
			gCommit = head
		}
	}
	includeWorktree := false
	if gCommit == "" {
		if active && head == wCommit {
			includeWorktree = true
		} else if active && !shallow {
			findings = append(findings, finding{path: "git:G", reason: "before G, HEAD must equal immutable W"})
		}
	} else {
		subject, subjectErr := gitText("show", "-s", "--format=%s", gCommit)
		parent, parentErr := slice55aCommitParent(gCommit)
		if subjectErr != nil || parentErr != nil || parent != wCommit || subject != gSubject {
			findings = append(findings, finding{path: "git:G", reason: "G must have exact W parent and subject"})
		}
		if active && head != gCommit {
			findings = append(findings, finding{path: "git:G", reason: "after G, active branch HEAD must equal G"})
		}
		if active && strings.TrimSpace(worktree) != "" {
			findings = append(findings, finding{path: "git:worktree", reason: "after final G, active worktree must be clean"})
		}
		if slice55aCommitExists(base) {
			changed, err := gitText("diff", "--name-only", base+".."+gCommit)
			if err == nil {
				findings = append(findings, slice55aComparePathSet("git:overall", nonEmptyLines(changed), slice55aOverallPaths())...)
			}
		}
		if !shallow || slice55aCommitExists(wCommit) {
			changed, err := gitText("diff-tree", "--no-commit-id", "--name-only", "-r", gCommit)
			if err == nil {
				findings = append(findings, slice55aComparePathSet("git:G", nonEmptyLines(changed), slice55aGStagePaths)...)
			}
		}
	}
	if includeWorktree {
		var dirty []string
		for _, args := range [][]string{{"diff", "--name-only"}, {"diff", "--cached", "--name-only"}, {"ls-files", "--others", "--exclude-standard"}} {
			text, _ := gitText(args...)
			dirty = append(dirty, nonEmptyLines(text)...)
		}
		findings = append(findings, slice55aComparePathSet("git:G-dirty", dirty, slice55aGStagePaths)...)
		if slice55aCommitExists(base) {
			changed, _ := gitText("diff", "--name-only", base+".."+wCommit)
			findings = append(findings, slice55aComparePathSet("git:overall-dirty", append(nonEmptyLines(changed), dirty...), slice55aOverallPaths())...)
		}
	}
	return findings
}

func slice55aCommitParent(commit string) (string, error) {
	raw, err := gitText("cat-file", "-p", commit)
	if err != nil {
		return "", err
	}
	var parents []string
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "parent ") {
			parents = append(parents, strings.TrimSpace(strings.TrimPrefix(line, "parent ")))
		}
	}
	if len(parents) != 1 {
		return "", fmt.Errorf("commit %s has %d parents, want exactly one", commit, len(parents))
	}
	return parents[0], nil
}

func slice55aCommitExists(commit string) bool {
	command := exec.Command("git", "cat-file", "-e", commit+"^{commit}")
	return command.Run() == nil
}

func slice55aOverallPaths() []string {
	set := make(map[string]bool)
	for _, stage := range slice55aStages {
		for _, path := range stage.paths {
			set[path] = true
		}
	}
	for _, path := range slice55aGStagePaths {
		set[path] = true
	}
	var paths []string
	for path := range set {
		paths = append(paths, path)
	}
	return paths
}

func slice55aComparePathSet(label string, actualPaths, expectedPaths []string) []finding {
	actual, expected := make(map[string]bool), make(map[string]bool)
	for _, path := range actualPaths {
		actual[filepath.ToSlash(strings.TrimSpace(path))] = true
	}
	for _, path := range expectedPaths {
		expected[filepath.ToSlash(path)] = true
	}
	var findings []finding
	for path := range expected {
		if !actual[path] {
			findings = append(findings, finding{path: path, reason: "missing from exact " + label + " path set"})
		}
	}
	for path := range actual {
		if path != "" && !expected[path] {
			findings = append(findings, finding{path: path, reason: "outside exact " + label + " path allowlist"})
		}
	}
	return findings
}

func gitRawText(args ...string) (string, error) {
	command := exec.Command("git", args...)
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(output), nil
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

func checkSlice55bGuard() []finding {
	command := exec.Command("go", "run", "./scripts/check-slice55b-evidence.go")
	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	return []finding{{path: "scripts/check-slice55b-evidence.go", reason: strings.TrimSpace(string(output))}}
}

func checkSlice55cGuard() []finding {
	command := exec.Command("go", "run", "./scripts/check-slice55c-evidence.go")
	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	return []finding{{path: "scripts/check-slice55c-evidence.go", reason: strings.TrimSpace(string(output))}}
}
