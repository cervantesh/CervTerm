//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	source := flag.String("source", "", "source font path")
	output := flag.String("output", "", "output subset font path")
	text := flag.String("text", "CervTerm 😀 👩‍💻 é अ م", "text to include in the subset")
	tool := flag.String("tool", "pyftsubset", "pyftsubset executable")
	flag.Parse()
	if *source == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "-source and -output are required")
		os.Exit(2)
	}
	if err := generateSubset(*source, *output, *text, *tool); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generateSubset(source, output, text, tool string) error {
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("source font not found: %s", source)
	}
	if _, err := exec.LookPath(tool); err != nil {
		return fmt.Errorf("pyftsubset not found. Install fonttools in a Python environment first")
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil && filepath.Dir(output) != "." {
		return err
	}
	fmt.Println("Only commit generated subsets when the source font license permits redistribution, for example OFL, Apache, or MIT.")
	cmd := exec.Command(tool,
		source,
		"--output-file="+output,
		"--text="+text,
		"--layout-features=*",
		"--glyph-names",
		"--symbol-cmap",
		"--legacy-cmap",
		"--notdef-glyph",
		"--notdef-outline",
		"--recommended-glyphs",
		"--name-IDs=*",
		"--name-legacy",
		"--name-languages=*",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", output)
	return nil
}
