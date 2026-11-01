package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The tests in cmd_conformance_test.go call runConformance from inside a test binary. These run
// the real compiled CLI, because a `go test` process behaves differently from a plain program.
func buildCLI(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "acton-plugin")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("building the CLI: %v\n%s", err, out)
	}
	return bin
}

func buildProviderWasm(t *testing.T, pkg string) string {
	t.Helper()
	wasm := filepath.Join(t.TempDir(), "plugin.wasm")
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", wasm, pkg)
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building %s: %v\n%s", pkg, err, out)
	}
	return wasm
}

func TestCLIRefusesToCertifyABrokenProvider(t *testing.T) {
	bin := buildCLI(t)
	wasm := buildProviderWasm(t, "./testdata/brokenmail")
	out, err := exec.Command(bin, "conformance", "--wasm", wasm, "--manifest", "testdata/brokenmail/manifest.json").CombinedOutput()
	if err == nil {
		t.Fatalf("the CLI certified a provider that lists an op outside the ABI:\n%s", out)
	}
	if !strings.Contains(string(out), "acton_not_an_export") {
		t.Errorf("the output should name the offending op, got:\n%s", out)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(wasm), "conformance.json")); statErr == nil {
		t.Error("a failed run must not write conformance.json")
	}
}

func TestCLICertifiesAConformingProvider(t *testing.T) {
	bin := buildCLI(t)
	wasm := buildProviderWasm(t, "../../plugins/mail/gmail")
	if out, err := exec.Command(bin, "conformance", "--wasm", wasm, "--manifest", "../../plugins/mail/gmail/manifest.json").CombinedOutput(); err != nil {
		t.Fatalf("conformance failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(wasm), "conformance.json")); err != nil {
		t.Errorf("a passing run must write conformance.json: %v", err)
	}
}
