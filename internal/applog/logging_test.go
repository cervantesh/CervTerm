package applog

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePathUsesEnvOverride(t *testing.T) {
	t.Setenv(EnvLogFile, filepath.Join(t.TempDir(), "custom.log"))
	got := ResolvePath("")
	if !strings.HasSuffix(got, "custom.log") {
		t.Fatalf("ResolvePath env override = %q", got)
	}
}

func TestResolvePathAllowsStderrOnly(t *testing.T) {
	for _, value := range []string{"-", "stderr", "none", " STDERR "} {
		if got := ResolvePath(value); got != "" {
			t.Fatalf("ResolvePath(%q) = %q, want stderr-only", value, got)
		}
	}
}

func TestSetupAppendsToLogFile(t *testing.T) {
	var previous bytes.Buffer
	log.SetOutput(&previous)
	defer log.SetOutput(&previous)
	path := filepath.Join(t.TempDir(), "nested", "cervterm.log")
	file, err := Setup(path)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	log.Print("diagnostic marker")
	if err := file.Close(); err != nil {
		t.Fatalf("close log: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Contains(data, []byte("diagnostic marker")) {
		t.Fatalf("log file missing marker: %q", string(data))
	}
	if bytes.Contains(data, []byte(path)) || bytes.Contains(data, []byte(filepath.Dir(path))) {
		t.Fatalf("log setup leaked private path: %q", string(data))
	}
}

func TestSetupFailureIsClassifiedWithoutPrivatePath(t *testing.T) {
	blockedParent := filepath.Join(t.TempDir(), "SECRET-DIRECTORY")
	if err := os.WriteFile(blockedParent, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Setup(filepath.Join(blockedParent, "SECRET-LOG.log"))
	if !errors.Is(err, ErrSetup) {
		t.Fatalf("Setup error=%v want ErrSetup", err)
	}
	for _, leaked := range []string{blockedParent, "SECRET-DIRECTORY", "SECRET-LOG"} {
		if strings.Contains(err.Error(), leaked) {
			t.Fatalf("setup error leaked %q: %v", leaked, err)
		}
	}
}

func TestFormatPanicReportRedactsValuesContextsAndSourcePaths(t *testing.T) {
	panicValue := "SECRET-PANIC-VALUE"
	stack := []byte("goroutine 1 [running]:\nprivate.package.call()\n\tC:\\Users\\private\\SECRET-DIRECTORY\\panic_file.go:42 +0x25\nother.call()\n\t/home/private/SECRET-DIRECTORY/other.go:7 +0x10")
	report := formatPanicReport("SECRET-CONTEXT", errors.New(panicValue), stack)
	for _, leaked := range []string{panicValue, "SECRET-CONTEXT", "SECRET-DIRECTORY", `C:\Users\private`, "/home/private"} {
		if strings.Contains(report, leaked) {
			t.Fatalf("panic report leaked %q: %s", leaked, report)
		}
	}
	for _, want := range []string{"panic context=unknown class=error", "private.package.call()", "<source>/panic_file.go:42 +0x25", "<source>/other.go:7 +0x10"} {
		if !strings.Contains(report, want) {
			t.Fatalf("panic report missing %q: %s", want, report)
		}
	}
}

func TestSafePanicContextAndClass(t *testing.T) {
	if safePanicContext("headless main") != "headless-main" || safePanicContext("glfw main") != "glfw-main" {
		t.Fatal("known panic contexts changed")
	}
	if panicClass("message") != "string" || panicClass(errors.New("message")) != "error" || panicClass(42) != "value" {
		t.Fatal("panic classification changed")
	}
}
