package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRunDoctorPrintsActionableSections(t *testing.T) {
	output := captureStdout(t, func() {
		if code := runDoctor(doctorOptions{LogPath: "-"}); code != 0 {
			t.Fatalf("runDoctor exit code = %d, want 0", code)
		}
	})

	for _, want := range []string{
		"CervTerm doctor",
		"version:",
		"platform:",
		"config:",
		"diagnostics:",
		"environment:",
		"text-gamma: 1.40",
		"text-darken: 0.10",
		"support:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q\n%s", want, output)
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(&buf, reader)
		done <- err
	}()

	fn()

	_ = writer.Close()
	os.Stdout = old
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	return buf.String()
}
