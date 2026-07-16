//go:build ignore

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"cervterm/internal/buildinfo"
)

func main() {
	arch := flag.String("arch", "amd64", "target architecture")
	tool := flag.String("tool", "goversioninfo", "goversioninfo executable")
	// In release CI this comes from the tag via RELEASE_VERSION; for local dev
	// builds it falls back to the compiled-in dev version so the embedded
	// resource is always a valid x.y.z string goversioninfo can parse.
	version := flag.String("version", os.Getenv("RELEASE_VERSION"), "release version to embed (e.g. v1.2.3-beta.1); empty falls back to buildinfo.Version")
	flag.Parse()
	if strings.TrimSpace(*version) == "" {
		*version = buildinfo.Version
	}
	if err := generateWindowsResource(*arch, *tool, *version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generateWindowsResource(arch, tool, version string) error {
	if _, err := exec.LookPath(tool); err != nil {
		return fmt.Errorf("%s not found. Install with: go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest", tool)
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	src := filepath.Join(root, "packaging", "windows")
	outName := "resource_windows_" + arch + ".syso"
	outPath := filepath.Join(root, "cmd", "cervterm", outName)
	tmp := filepath.Join(root, "dist", "resource-"+arch)
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	for _, name := range []string{"versioninfo.json", "cervterm.ico", "cervterm.manifest"} {
		if err := copyFile(filepath.Join(src, name), filepath.Join(tmp, name)); err != nil {
			return err
		}
	}
	if strings.TrimSpace(version) != "" {
		if err := patchVersionInfo(filepath.Join(tmp, "versioninfo.json"), version); err != nil {
			return err
		}
	}
	args := []string{"-platform-specific"}
	if strings.Contains(arch, "64") || arch == "amd64" || arch == "arm64" {
		args = append(args, "-64")
	}
	cmd := exec.Command(tool, args...)
	cmd.Dir = tmp
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	generated := filepath.Join(tmp, outName)
	if _, err := os.Stat(generated); err != nil {
		return fmt.Errorf("goversioninfo did not produce %s", outName)
	}
	if err := copyFile(generated, outPath); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", outPath)
	return nil
}

// patchVersionInfo rewrites the FileVersion/ProductVersion fields of a
// versioninfo.json so the embedded Windows resource matches the release tag
// instead of a hardcoded value. The numeric FixedFileInfo fields require an
// x.y.z form, so the leading "v" and any prerelease/build suffix are stripped
// for those; the StringFileInfo fields keep the full x.y.z(-prerelease) string
// (also without the leading "v", which goversioninfo cannot parse).
func patchVersionInfo(path, version string) error {
	major, minor, patch, build, stringVersion, err := parseVersion(version)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	var fixed map[string]json.RawMessage
	if err := json.Unmarshal(doc["FixedFileInfo"], &fixed); err != nil {
		return fmt.Errorf("parse FixedFileInfo in %s: %w", path, err)
	}
	numeric, err := json.Marshal(map[string]int{
		"Major": major, "Minor": minor, "Patch": patch, "Build": build,
	})
	if err != nil {
		return err
	}
	fixed["FileVersion"] = numeric
	fixed["ProductVersion"] = numeric
	fixedRaw, err := json.Marshal(fixed)
	if err != nil {
		return err
	}
	doc["FixedFileInfo"] = fixedRaw

	var str map[string]json.RawMessage
	if err := json.Unmarshal(doc["StringFileInfo"], &str); err != nil {
		return fmt.Errorf("parse StringFileInfo in %s: %w", path, err)
	}
	quoted, err := json.Marshal(stringVersion)
	if err != nil {
		return err
	}
	str["FileVersion"] = quoted
	str["ProductVersion"] = quoted
	strRaw, err := json.Marshal(str)
	if err != nil {
		return err
	}
	doc["StringFileInfo"] = strRaw

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

var versionPattern = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)(?:-(.+))?$`)

// parseVersion splits a release version such as "v0.8.0-beta.1" into the numeric
// components for the Windows resource and the string form without the leading
// "v". Build is taken from a trailing integer in the prerelease suffix (e.g. the
// "1" in "beta.1"), defaulting to 0.
func parseVersion(version string) (major, minor, patch, build int, stringVersion string, err error) {
	m := versionPattern.FindStringSubmatch(strings.TrimSpace(version))
	if m == nil {
		return 0, 0, 0, 0, "", fmt.Errorf("unsupported version %q; expected vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-prerelease", version)
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	patch, _ = strconv.Atoi(m[3])
	if prerelease := m[4]; prerelease != "" {
		if trailing := regexp.MustCompile(`([0-9]+)$`).FindString(prerelease); trailing != "" {
			build, _ = strconv.Atoi(trailing)
		}
	}
	return major, minor, patch, build, strings.TrimPrefix(strings.TrimSpace(version), "v"), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = out.ReadFrom(in)
	return err
}
