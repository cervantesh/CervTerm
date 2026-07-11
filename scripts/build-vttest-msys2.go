//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	msys2Bash := flag.String("msys2-bash", `C:\msys64\usr\bin\bash.exe`, "MSYS2 bash path")
	outDir := flag.String("outdir", filepath.Join("dist", "tools", "vttest-msys2"), "output directory")
	url := flag.String("url", "https://invisible-island.net/datafiles/release/vttest.tar.gz", "vttest source tarball URL")
	flag.Parse()
	if err := buildVttest(*msys2Bash, *outDir, *url); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildVttest(msys2Bash, outDir, url string) error {
	if _, err := os.Stat(msys2Bash); err != nil {
		return fmt.Errorf("MSYS2 bash not found at %s. Install MSYS2, then run: pacman -S --needed base-devel gcc make ncurses-devel", msys2Bash)
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	out := filepath.Join(root, outDir)
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	msysOut := toMsysPath(out)
	script := fmt.Sprintf(`set -euo pipefail
mkdir -p '%s'
cd '%s'
if [ ! -d src ]; then
  curl -L --fail -o vttest.tar.gz '%s'
  mkdir src
  tar -xzf vttest.tar.gz -C src --strip-components=1
fi
cd src
./configure --prefix='%s/install'
make -j2
make install
if [ -x '%s/install/bin/vttest.exe' ]; then
  echo '%s/install/bin/vttest.exe'
elif [ -x '%s/src/vttest.exe' ]; then
  echo '%s/src/vttest.exe'
else
  echo 'vttest build finished but executable was not found' >&2
  exit 1
fi
`, msysOut, msysOut, url, msysOut, msysOut, msysOut, msysOut, msysOut)
	cmd := exec.Command(msys2Bash, "-lc", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func toMsysPath(path string) string {
	s := filepath.ToSlash(path)
	if len(s) >= 3 && s[1] == ':' && s[2] == '/' {
		return "/" + strings.ToLower(s[:1]) + "/" + s[3:]
	}
	return s
}
