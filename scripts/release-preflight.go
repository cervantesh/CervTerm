//go:build ignore

package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type check struct {
	Name     string
	Passed   bool
	Detail   string
	Required bool
}

func main() {
	version := flag.String("version", "v0.2.0-beta.1", "release/package version")
	outDir := flag.String("outdir", "dist", "output directory")
	windowsZip := flag.String("windows-zip", "", "Windows zip path")
	vttestExe := flag.String("vttest", "", "optional vttest executable path")
	requireSigning := flag.Bool("require-signing", false, "require code signing material")
	requireVttest := flag.Bool("require-vttest", false, "require vttest executable")
	requireWix := flag.Bool("require-wix", false, "require WiX CLI and MSI policy")
	flag.Parse()
	if err := runPreflight(*version, *outDir, *windowsZip, *vttestExe, *requireSigning, *requireVttest, *requireWix); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runPreflight(version, outDir, windowsZip, vttestExe string, requireSigning, requireVttest, requireWix bool) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if windowsZip == "" {
		windowsZip = filepath.Join(outDir, "cervterm-"+version+"-windows.zip")
	}
	var checks []check
	add := func(name string, passed bool, detail string, required ...bool) {
		req := true
		if len(required) > 0 {
			req = required[0]
		}
		checks = append(checks, check{name, passed, detail, req})
	}

	add("go toolchain", commandExists("go"), "go must be on PATH for builds and tests")
	add("git checkout", exists(filepath.Join(root, ".git")), "run preflight from the CervTerm repository root")
	add("release workflow", exists(filepath.Join(root, ".github/workflows/release.yml")), "tagged release workflow should exist")
	add("package tool", exists(filepath.Join(root, "scripts/package-beta.go")), "local beta package tool should exist")
	add("signing tool", exists(filepath.Join(root, "scripts/sign-windows-exe.go")), "Authenticode signing hook should exist")
	add("vttest build route", exists(filepath.Join(root, "scripts/build-vttest-msys2.go")), "MSYS2 vttest build helper should exist")
	add("vttest capture route", exists(filepath.Join(root, "scripts/capture-vttest.go")), "raw vttest capture helper should exist")
	add("WiX template", exists(filepath.Join(root, "packaging/wix/CervTerm.wxs")), "MSI template should exist")
	add("winget templates", exists(filepath.Join(root, "packaging/winget/T50Systems.CervTerm.yaml")), "portable winget templates should exist")
	add("installed package smoke tool", exists(filepath.Join(root, "scripts/smoke-installed-package.go")), "clean package smoke should be reusable locally and in CI")
	add("maturity gates script", exists(filepath.Join(root, "scripts/check-maturity-gates.go")), "maturity guardrails should be executable in CI")
	add("daily-driver smoke script", exists(filepath.Join(root, "scripts/daily-driver-smoke.go")), "daily-driver workflows should be smoke-gated")
	for _, item := range []struct{ name, path, detail string }{
		{"troubleshooting docs", "docs/troubleshooting.md", "user diagnostics workflow should be documented"},
		{"getting started docs", "docs/getting-started.md", "onboarding workflow should be documented"},
		{"daily-driver smoke docs", "docs/daily-driver-smoke.md", "daily-driver smoke matrix should be documented"},
		{"support docs", "SUPPORT.md", "support and beta scope should be documented"},
		{"bug issue template", ".github/ISSUE_TEMPLATE/bug_report.yml", "bug reports should request diagnostics"},
		{"Dependabot config", ".github/dependabot.yml", "dependency drift should be automated"},
		{"release trust docs", "docs/release-trust.md", "checksums, attestations, and unsigned beta status should be documented"},
		{"maturity improvement plan", "docs/maturity-improvement-plan.md", "DoE/DoDm maturity improvement plan should exist"},
	} {
		add(item.name, exists(filepath.Join(root, item.path)), item.detail)
	}

	signtool := findSigntool()
	if requireSigning {
		add("Authenticode secrets", os.Getenv("WINDOWS_CODESIGN_PFX_BASE64") != "" && os.Getenv("WINDOWS_CODESIGN_PASSWORD") != "", "set WINDOWS_CODESIGN_PFX_BASE64 and WINDOWS_CODESIGN_PASSWORD")
		add("signtool", signtool != "", "install Windows SDK or run release on windows-latest")
	} else {
		add("Authenticode signing", true, "intentionally deferred for free beta zip releases; SHA256SUMS and GitHub attestations remain the default", false)
	}

	vttestAvailable := false
	if vttestExe != "" {
		vttestAvailable = exists(vttestExe)
	} else {
		vttestAvailable = commandExists("vttest.exe") || commandExists("vttest") || exists(filepath.Join(root, "dist/tools/vttest-msys2/install/bin/vttest.exe"))
	}
	add("vttest executable", vttestAvailable, "provide -vttest, install vttest, or run go run ./scripts/build-vttest-msys2.go", requireVttest)
	add("MSYS2 bash", exists(`C:\msys64\usr\bin\bash.exe`), "install MSYS2 if building vttest locally on Windows", false)

	if requireWix {
		add("WiX CLI", commandExists("wix.exe"), "install WiXToolset.WiXToolset before publishing MSI artifacts")
		add("MSI policy", false, "decide per-user/per-machine, PATH/shortcut behavior, upgrade cadence, config install behavior, and signing policy before enabling MSI CI")
	} else {
		add("MSI/WiX publishing", true, "intentionally deferred; portable zip and winget templates are the beta distribution path", false)
	}

	if exists(windowsZip) {
		entries, err := zipEntries(windowsZip)
		if err != nil {
			add("Windows beta zip", false, err.Error(), false)
		} else {
			for _, required := range []string{"cervterm.exe", "cervterm.lua", "README.md", "CHANGELOG.md", "SUPPORT.md", "font-sources/NotoColorEmoji.ttf", "font-sources/NotoEmoji-LICENSE.txt", "docs/daily-driver-smoke.md", "docs/getting-started.md", "docs/product-roadmap.md", "docs/troubleshooting.md", "docs/release-trust.md", "docs/maturity-improvement-plan.md", "docs/product-ux-maintainability-to-9-plan.md", "docs/assets/cervterm-preview.png", "packaging/winget/README.md"} {
				add("zip contains "+required, entries[required], "check "+windowsZip+" package contents")
			}
		}
	} else {
		add("Windows beta zip", false, "run go run ./scripts/package-beta.go -version "+version+" -outdir "+outDir, false)
	}

	passed, failedRequired, failedOptional := 0, 0, 0
	for _, c := range checks {
		status := "PASS"
		if c.Passed {
			passed++
		} else if c.Required {
			status = "FAIL"
			failedRequired++
		} else {
			status = "WARN"
			failedOptional++
		}
		fmt.Printf("[%s] %s: %s\n", status, c.Name, c.Detail)
	}
	fmt.Println()
	fmt.Printf("Release preflight summary: %d pass, %d required fail, %d warning\n", passed, failedRequired, failedOptional)
	if failedRequired > 0 {
		return fmt.Errorf("release preflight failed")
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func zipEntries(path string) (map[string]bool, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	entries := map[string]bool{}
	for _, entry := range r.File {
		entries[strings.TrimPrefix(filepath.ToSlash(entry.Name), "./")] = true
	}
	return entries, nil
}

func findSigntool() string {
	if p, err := exec.LookPath("signtool.exe"); err == nil {
		return p
	}
	if runtime.GOOS != "windows" {
		return ""
	}
	root := os.Getenv("ProgramFiles(x86)")
	if root == "" {
		return ""
	}
	kits := filepath.Join(root, "Windows Kits", "10", "bin")
	var matches []string
	_ = filepath.WalkDir(kits, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.EqualFold(entry.Name(), "signtool.exe") && strings.Contains(strings.ToLower(filepath.ToSlash(path)), "/x64/") {
			matches = append(matches, path)
		}
		return nil
	})
	sort.Strings(matches)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1]
}
