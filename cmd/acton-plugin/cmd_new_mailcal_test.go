package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMailcalScaffoldBuildsAndPassesConformanceUntouched(t *testing.T) {
	dir := filepath.Join("..", "..", "plugins", "mail", "zz-scaffold-test")
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := runNew([]string{"--type=mailcal", "--dir", dir, "zz-scaffold-test"}); err != nil {
		t.Fatal(err)
	}

	// the scaffold's own test runs the shape suite natively
	if out, err := exec.Command("go", "test", "./"+filepath.ToSlash(dir)+"/...").CombinedOutput(); err != nil {
		t.Fatalf("scaffold tests failed: %v\n%s", err, out)
	}

	wasm := filepath.Join(t.TempDir(), "plugin.wasm")
	build := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", wasm, "./"+filepath.ToSlash(dir))
	build.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("scaffold does not compile to wasm: %v\n%s", err, b)
	}
	if err := runValidate([]string{"--manifest", filepath.Join(dir, "manifest.json")}); err != nil {
		t.Fatalf("scaffold manifest invalid: %v", err)
	}
	if err := runConformance([]string{"--wasm", wasm, "--manifest", filepath.Join(dir, "manifest.json")}); err != nil {
		t.Fatalf("scaffold fails conformance: %v", err)
	}
}

func TestNewRejectsAnUnknownType(t *testing.T) {
	if err := runNew([]string{"--type=bogus", "--dir", t.TempDir(), "x"}); err == nil {
		t.Fatal("expected an error for an unknown plugin type")
	}
}
