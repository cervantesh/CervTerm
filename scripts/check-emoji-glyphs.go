//go:build ignore

// check-emoji-glyphs validates real glyph rasterization coverage for Unicode emoji.
// It performs no network access; download emoji-test.txt explicitly, then run:
//
//	go run ./scripts/check-emoji-glyphs.go .tmp/emoji-test-latest.txt
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"cervterm/internal/fontglyph"
	"cervterm/internal/unicodecluster"
)

type failure struct {
	label  string
	reason string
	face   string
}

type counts struct {
	total       int
	rasterized  int
	visible     int
	color       int
	flags       int
	flagsNoto   int
	flagsOther  int
	nonFlags    int
	faces       map[string]int
	faceColors  map[string]int
	faceVisible map[string]int
}

func main() {
	maxFailures := flag.Int("max-failures", 50, "maximum failure rows to print")
	allowFlagFallback := flag.Bool("allow-flag-fallback", false, "allow flags to rasterize from non-Noto emoji fonts")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/check-emoji-glyphs.go [flags] path/to/emoji-test.txt")
		flag.PrintDefaults()
		os.Exit(2)
	}

	backend, err := fontglyph.NewOpenTypeBackend(fontglyph.Spec{Family: "Go Mono", Size: 18, DPI: 96})
	if err != nil {
		fmt.Fprintf(os.Stderr, "create font backend: %v\n", err)
		os.Exit(1)
	}
	fontDiag := fontglyph.DiagnoseEmojiFonts()
	for _, warning := range fontDiag.Warnings {
		fmt.Fprintf(os.Stderr, "WARN emoji font: %s\n", warning)
	}

	file, err := os.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open emoji test file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	stats := counts{faces: map[string]int{}, faceColors: map[string]int{}, faceVisible: map[string]int{}}
	var failures []failure
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "; fully-qualified") {
			continue
		}
		sequence, label, err := parseEmojiTestLine(line)
		if err != nil {
			failures = append(failures, failure{label: line, reason: "parse: " + err.Error()})
			continue
		}
		stats.total++
		clusters := unicodecluster.Segment(sequence)
		if len(clusters) != 1 || !clusters[0].IsEmoji || clusters[0].Width != 2 || clusters[0].Text != sequence {
			failures = append(failures, failure{label: label, reason: fmt.Sprintf("bad cluster model: %#v", clusters)})
			continue
		}

		info := backend.InspectClusterGlyph(sequence, 2)
		face := normalizedFace(info.FaceSource)
		stats.faces[face]++
		if info.Rasterized {
			stats.rasterized++
		}
		if info.HasVisible {
			stats.visible++
			stats.faceVisible[face]++
		}
		if info.HasColor {
			stats.color++
			stats.faceColors[face]++
		}

		if unicodecluster.IsFlagString(sequence) {
			stats.flags++
			if isNotoColorEmoji(face) {
				stats.flagsNoto++
			} else {
				stats.flagsOther++
				if !*allowFlagFallback {
					failures = append(failures, failure{label: label, reason: "flag did not use Noto Color Emoji", face: face})
				}
			}
		} else {
			stats.nonFlags++
		}

		if !info.Rasterized {
			failures = append(failures, failure{label: label, reason: "RasterizeCluster returned false", face: face})
			continue
		}
		if !info.HasVisible {
			failures = append(failures, failure{label: label, reason: "rasterized image has no visible pixels", face: face})
		}
		if !info.HasColor {
			failures = append(failures, failure{label: label, reason: "rasterized without color glyph path", face: face})
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "read emoji test file: %v\n", err)
		os.Exit(1)
	}

	printStats(stats)
	if stats.flags > 0 && stats.flagsNoto != stats.flags {
		fmt.Fprintf(os.Stderr, "WARN emoji glyph coverage: %d/%d flags used non-Noto fallback; flags may render as regional letters\n", stats.flagsOther, stats.flags)
	}
	if stats.color != stats.total {
		fmt.Fprintf(os.Stderr, "WARN emoji glyph coverage: %d/%d sequences did not use a color glyph path\n", stats.total-stats.color, stats.total)
	}
	if len(failures) > 0 {
		limit := min(len(failures), *maxFailures)
		fmt.Fprintf(os.Stderr, "\nfailures (%d total, showing %d):\n", len(failures), limit)
		for _, failure := range failures[:limit] {
			face := failure.face
			if face == "" {
				face = "<none>"
			}
			fmt.Fprintf(os.Stderr, "FAIL %-42s | %-44s | %s\n", failure.label, failure.reason, face)
		}
		os.Exit(1)
	}
	fmt.Println("emoji glyph coverage ok")
}

func parseEmojiTestLine(line string) (sequence string, label string, err error) {
	fields := strings.SplitN(line, "#", 2)
	left := fields[0]
	if len(fields) == 2 {
		label = strings.TrimSpace(fields[1])
	} else {
		label = strings.TrimSpace(line)
	}
	codepoints := strings.Fields(strings.SplitN(left, ";", 2)[0])
	var b strings.Builder
	for _, codepoint := range codepoints {
		value, parseErr := strconv.ParseInt(codepoint, 16, 32)
		if parseErr != nil {
			return "", label, parseErr
		}
		b.WriteRune(rune(value))
	}
	return b.String(), label, nil
}

func printStats(stats counts) {
	fmt.Println("emoji glyph coverage:")
	fmt.Printf("  total fully-qualified: %d\n", stats.total)
	fmt.Printf("  rasterized:            %d\n", stats.rasterized)
	fmt.Printf("  visible:               %d\n", stats.visible)
	fmt.Printf("  color glyphs:          %d\n", stats.color)
	fmt.Printf("  flags:                 %d\n", stats.flags)
	fmt.Printf("  flags via Noto:        %d\n", stats.flagsNoto)
	fmt.Printf("  flags via other:       %d\n", stats.flagsOther)
	fmt.Println("  fonts:")
	faces := make([]string, 0, len(stats.faces))
	for face := range stats.faces {
		faces = append(faces, face)
	}
	sort.Strings(faces)
	for _, face := range faces {
		fmt.Printf("    %-28s total=%4d visible=%4d color=%4d\n", face, stats.faces[face], stats.faceVisible[face], stats.faceColors[face])
	}
}

func normalizedFace(path string) string {
	if path == "" {
		return "<none>"
	}
	return filepath.Base(path)
}

func isNotoColorEmoji(face string) bool {
	face = strings.ToLower(face)
	return face == "notocoloremoji.ttf" || face == "noto-color-emoji.ttf"
}
