package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/actonos/plugin-sdk/sdk"
	"github.com/actonos/plugin-sdk/sdk/mailcal/certificate"
)

// build-all compiles with paths relative to the repository root.
func chdirToRepoRoot(t *testing.T) {
	t.Helper()
	old, _ := os.Getwd()
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestBuildAllPublishesTheMailPluginsWithTheirOAuthBlockAndACertificate(t *testing.T) {
	chdirToRepoRoot(t)
	dist := t.TempDir()
	if err := runBuildAll([]string{"--dir", "plugins/mail", "--out", dist}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dist, "plugin-registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var reg PluginRegistry
	if err := json.Unmarshal(raw, &reg); err != nil {
		t.Fatal(err)
	}
	byID := map[string]PluginRegistryEntry{}
	for _, p := range reg.Plugins {
		byID[p.ID] = p
	}
	for id, provider := range map[string]string{"mailcal-gmail": "google", "mailcal-graph": "microsoft"} {
		e, ok := byID[id]
		if !ok {
			t.Fatalf("%s is missing from the registry", id)
		}
		if len(e.Capabilities) != 2 || e.Capabilities[0] != sdk.CapabilityMail || e.Capabilities[1] != sdk.CapabilityCalendar {
			t.Errorf("%s capabilities = %v", id, e.Capabilities)
		}
		if e.Category != "connector" {
			t.Errorf("%s category = %q, want connector", id, e.Category)
		}

		pkg := filepath.Join(dist, e.Filename)
		pkgBytes, err := os.ReadFile(pkg)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(pkgBytes)
		if e.SHA256 != hex.EncodeToString(sum[:]) || e.SizeBytes != int64(len(pkgBytes)) {
			t.Errorf("%s: registry hash/size do not describe %s", id, e.Filename)
		}

		zr, err := zip.OpenReader(pkg)
		if err != nil {
			t.Fatal(err)
		}
		defer zr.Close()
		var manifest sdk.PluginManifest
		var wasm []byte
		for _, f := range zr.File {
			rc, _ := f.Open()
			data, _ := io.ReadAll(rc)
			rc.Close()
			switch f.Name {
			case "manifest.json":
				if err := json.Unmarshal(data, &manifest); err != nil {
					t.Fatal(err)
				}
			case "plugin.wasm":
				wasm = data
			}
		}
		if manifest.OAuth == nil || manifest.OAuth.Provider != provider || len(manifest.OAuth.Scopes) == 0 {
			t.Errorf("%s: the package manifest lost its oauth block: %+v", id, manifest.OAuth)
		}

		cert, err := certificate.Read(filepath.Join(dist, id+".conformance.json"))
		if err != nil {
			t.Fatalf("%s: build-all must certify mail providers: %v", id, err)
		}
		if !certificate.Verify(cert, wasm) {
			t.Errorf("%s: the certificate does not match the packaged plugin.wasm", id)
		}
	}
}
