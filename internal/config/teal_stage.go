package config

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type StagedTeal struct {
	SourcePath    string
	EvaluationLua string
	PublishedLua  string
}

func stageTealSource(sourcePath, stageRoot string) (StagedTeal, error) {
	tl, err := exec.LookPath("tl")
	if err != nil {
		return StagedTeal{}, fmt.Errorf("Teal config %q requires the `tl` command; install Teal: %w", sourcePath, err)
	}
	canonical, _, err := canonicalLocalFile(sourcePath)
	if err != nil {
		return StagedTeal{}, err
	}
	sourceDir := filepath.Dir(canonical)
	sourceBase := filepath.Base(canonical)
	check := exec.Command(tl, "check", "-I", sourceDir, sourceBase)
	check.Dir = sourceDir
	if out, err := check.CombinedOutput(); err != nil {
		return StagedTeal{}, fmt.Errorf("tl check failed for %q: %s: %w", canonical, strings.TrimSpace(string(out)), err)
	}
	content, err := os.ReadFile(canonical)
	if err != nil {
		return StagedTeal{}, fmt.Errorf("read Teal source %q: %w", canonical, err)
	}
	hash := sha256.Sum256([]byte(canonicalIdentity(canonical)))
	stageDir := filepath.Join(stageRoot, fmt.Sprintf("%x", hash[:8]))
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		return StagedTeal{}, fmt.Errorf("create Teal staging directory: %w", err)
	}
	stagedSource := filepath.Join(stageDir, sourceBase)
	if err := os.WriteFile(stagedSource, content, 0o600); err != nil {
		return StagedTeal{}, fmt.Errorf("stage Teal source %q: %w", canonical, err)
	}
	generate := exec.Command(tl, "gen", "-I", sourceDir, sourceBase)
	generate.Dir = stageDir
	if out, err := generate.CombinedOutput(); err != nil {
		return StagedTeal{}, fmt.Errorf("tl gen failed for %q: %s: %w", canonical, strings.TrimSpace(string(out)), err)
	}
	luaBase := strings.TrimSuffix(sourceBase, filepath.Ext(sourceBase)) + ".lua"
	evaluationLua := filepath.Join(stageDir, luaBase)
	info, err := os.Stat(evaluationLua)
	if err != nil || !info.Mode().IsRegular() {
		return StagedTeal{}, fmt.Errorf("tl gen for %q did not produce staged output %q", canonical, evaluationLua)
	}
	return StagedTeal{
		SourcePath: canonical, EvaluationLua: evaluationLua,
		PublishedLua: filepath.Join(sourceDir, strings.TrimSuffix(sourceBase, filepath.Ext(sourceBase))+".lua"),
	}, nil
}
