package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/actonos/plugin-sdk/host"
	"github.com/actonos/plugin-sdk/sdk"
	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
	"github.com/actonos/plugin-sdk/sdk/mailcal/certificate"
	"github.com/actonos/plugin-sdk/sdk/mailcal/conformance"
)

// certifyMailProvider runs the offline contract suite on a compiled mail/calendar provider and
// returns a certificate bound to exactly these bytes. The run needs no token and no network:
// every operation that needs an account must answer with a coded error. Failures are printed to w.
func certifyMailProvider(w io.Writer, wasm []byte, manifest sdk.PluginManifest, account string) (certificate.Certificate, error) {
	if !isMailcal(manifest) {
		return certificate.Certificate{}, fmt.Errorf("%q does not declare mail@1 or calendar@1; conformance only applies to mail/calendar providers", manifest.ID)
	}
	if errs, _ := validateMailcal(manifest); len(errs) > 0 {
		return certificate.Certificate{}, fmt.Errorf("manifest is not valid for a mail/calendar provider: %v", errs)
	}

	ctx := context.Background()
	mock, err := host.NewMockHost(ctx)
	if err != nil {
		return certificate.Certificate{}, err
	}
	defer mock.Close()
	mock.SetAllowedDomains(nil) // offline: no network at all
	runner, err := mock.LoadPluginFromBytes(ctx, wasm)
	if err != nil {
		return certificate.Certificate{}, fmt.Errorf("loading the plugin: %w", err)
	}
	defer runner.Close()
	provider := mailcal.NewBridgeProvider(runner)
	acct := abi.AccountRef{ID: account}

	caps, capsErr := provider.Capabilities(ctx, acct)
	if capsErr != nil {
		return certificate.Certificate{}, fmt.Errorf("conformance FAILED for %s: Capabilities: %w", manifest.ID, capsErr)
	}
	// Not testing.RunTests: outside `go test` it runs nothing and reports success.
	if !conformance.Verify(w, provider, acct) {
		return certificate.Certificate{}, fmt.Errorf("conformance FAILED for %s (see the output above)", manifest.ID)
	}
	return certificate.Certificate{
		PluginID:   manifest.ID,
		Version:    manifest.Version,
		ABI:        caps.ABI,
		Ops:        caps.Ops,
		WasmSHA256: certificate.HashWasm(wasm),
		ShapeOK:    true,
		At:         time.Now().UTC(),
		Tool:       "acton-plugin " + sdk.Version,
	}, nil
}

// runConformance certifies a compiled provider and, on success, writes conformance.json next to
// the .wasm. A failing run writes nothing and returns an error (non-zero exit).
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
	wasm, err := os.ReadFile(*wasmPath)
	if err != nil {
		return fmt.Errorf("reading wasm: %w", err)
	}

	cert, err := certifyMailProvider(os.Stderr, wasm, manifest, *account)
	if err != nil {
		return err
	}
	out := filepath.Join(filepath.Dir(*wasmPath), certificate.CertificateFile)
	if err := certificate.Write(out, cert); err != nil {
		return err
	}
	fmt.Printf("OK: %s passed the offline conformance suite -> %s\n", manifest.ID, out)
	return nil
}
