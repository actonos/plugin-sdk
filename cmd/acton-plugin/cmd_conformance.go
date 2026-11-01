package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/actonos/plugin-sdk/host"
	"github.com/actonos/plugin-sdk/sdk"
	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
	"github.com/actonos/plugin-sdk/sdk/mailcal/certificate"
	"github.com/actonos/plugin-sdk/sdk/mailcal/conformance"
)

// runConformance runs the offline contract suite on a compiled mail/calendar provider and, on
// success, writes conformance.json next to the .wasm. The run needs no token and no network:
// every operation that needs an account must answer with a coded error.
func runConformance(args []string) error {
	fs := flag.NewFlagSet("conformance", flag.ContinueOnError)
	wasmPath := fs.String("wasm", "dist/plugin.wasm", "Path to the compiled .wasm file")
	manifestPath := fs.String("manifest", "manifest.json", "Path to manifest.json")
	account := fs.String("account", "conformance", "Account id used for the probe calls")
	if err := fs.Parse(args); err != nil {
		return err
	}

	raw, err := os.ReadFile(*manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}
	var manifest sdk.PluginManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}
	if !isMailcal(manifest) {
		return fmt.Errorf("%q does not declare mail@1 or calendar@1; conformance only applies to mail/calendar providers", manifest.ID)
	}
	if errs, _ := validateMailcal(manifest); len(errs) > 0 {
		return fmt.Errorf("manifest is not valid for a mail/calendar provider: %v", errs)
	}

	wasm, err := os.ReadFile(*wasmPath)
	if err != nil {
		return fmt.Errorf("reading wasm: %w", err)
	}

	ctx := context.Background()
	mock, err := host.NewMockHost(ctx)
	if err != nil {
		return err
	}
	defer mock.Close()
	mock.SetAllowedDomains(nil) // offline: no network at all
	runner, err := mock.LoadPluginFromBytes(ctx, wasm)
	if err != nil {
		return fmt.Errorf("loading the plugin: %w", err)
	}
	defer runner.Close()
	provider := mailcal.NewBridgeProvider(runner)
	acct := abi.AccountRef{ID: *account}

	caps, capsErr := provider.Capabilities(ctx, acct)
	testing.Init() // RunTests needs the testing flags registered when called outside `go test`
	passed := testing.RunTests(func(pat, str string) (bool, error) { return true, nil }, []testing.InternalTest{{
		Name: "shape",
		F:    func(t *testing.T) { conformance.RunShape(t, provider, acct) },
	}})
	if capsErr != nil || !passed {
		return fmt.Errorf("conformance FAILED for %s (see the output above)", manifest.ID)
	}

	cert := certificate.Certificate{
		PluginID:   manifest.ID,
		Version:    manifest.Version,
		ABI:        caps.ABI,
		Ops:        caps.Ops,
		WasmSHA256: certificate.HashWasm(wasm),
		ShapeOK:    true,
		At:         time.Now().UTC(),
		Tool:       "acton-plugin " + sdk.Version,
	}
	out := filepath.Join(filepath.Dir(*wasmPath), certificate.CertificateFile)
	if err := certificate.Write(out, cert); err != nil {
		return err
	}
	fmt.Printf("OK: %s passed the offline conformance suite -> %s\n", manifest.ID, out)
	return nil
}
