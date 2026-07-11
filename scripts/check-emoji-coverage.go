//go:build ignore

// check-emoji-coverage validates CervTerm's Unicode cluster model against a
// Unicode emoji-test.txt file. It performs no network access; download the file
// explicitly, then run:
//
//	go run ./scripts/check-emoji-coverage.go .tmp/emoji-test-latest.txt
package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"cervterm/internal/unicodecluster"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/check-emoji-coverage.go path/to/emoji-test.txt")
		os.Exit(2)
	}
	file, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "open emoji test file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	var checked int
	var failures []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "; fully-qualified") {
			continue
		}
		sequence, label, err := parseEmojiTestLine(line)
		if err != nil {
			failures = append(failures, fmt.Sprintf("parse %q: %v", line, err))
			continue
		}
		checked++
		clusters := unicodecluster.Segment(sequence)
		if len(clusters) != 1 || !clusters[0].IsEmoji || clusters[0].Width != 2 || clusters[0].Text != sequence {
			failures = append(failures, fmt.Sprintf("%s => clusters=%#v", label, clusters))
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "read emoji test file: %v\n", err)
		os.Exit(1)
	}
	if len(failures) > 0 {
		for _, failure := range failures[:min(len(failures), 50)] {
			fmt.Fprintln(os.Stderr, failure)
		}
		if len(failures) > 50 {
			fmt.Fprintf(os.Stderr, "... and %d more failures\n", len(failures)-50)
		}
		fmt.Fprintf(os.Stderr, "emoji cluster coverage failed: %d/%d failed\n", len(failures), checked)
		os.Exit(1)
	}
	fmt.Printf("emoji cluster coverage ok: %d fully-qualified sequences\n", checked)
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
