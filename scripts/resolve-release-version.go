//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const releaseVersionPattern = `^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$`

func main() {
	version := flag.String("version", os.Getenv("GITHUB_REF_NAME"), "release version to validate")
	envName := flag.String("env-name", "RELEASE_VERSION", "environment variable name to export")
	githubEnv := flag.String("github-env", os.Getenv("GITHUB_ENV"), "GitHub Actions environment file")
	flag.Parse()
	if err := resolveReleaseVersion(*version, *envName, *githubEnv); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func resolveReleaseVersion(version, envName, githubEnv string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return fmt.Errorf("release version is empty")
	}
	if !regexp.MustCompile(releaseVersionPattern).MatchString(version) {
		return fmt.Errorf("unsafe release version %q; expected vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-prerelease", version)
	}
	if envName == "" {
		return fmt.Errorf("environment variable name is empty")
	}
	line := fmt.Sprintf("%s=%s\n", envName, version)
	if githubEnv == "" {
		fmt.Print(line)
		return nil
	}
	file, err := os.OpenFile(githubEnv, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(line)
	return err
}
