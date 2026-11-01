package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// scaffoldMailcal writes a read-only mail provider that passes the conformance suite as generated.
// It must be created inside this module (for example --dir plugins/mail/<name>) because it
// imports the SDK by its module path, like the other scaffolds.
func scaffoldMailcal(name, dir string) error {
	modulePath, err := moduleImportPath(dir)
	if err != nil {
		return err
	}
	files := map[string]string{
		"manifest.json":                    mailcalManifest(name),
		"main.go":                          mailcalMain(modulePath),
		"internal/backend/backend.go":      mailcalBackend,
		"internal/backend/backend_test.go": mailcalBackendTest(modulePath),
		"README.md":                        fmt.Sprintf("# %s\n\nA mail provider plugin for ActonOS. See docs/MAILCAL_PLUGIN_GUIDE.md.\n", name),
		".gitignore":                       "dist/\n*.wasm\nconformance.json\n",
	}
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	_ = exec.Command("gofmt", "-w", dir).Run() // best effort: keep generated code formatted

	fmt.Printf("Created mail provider '%s' in '%s'.\n\nNext steps:\n", name, dir)
	fmt.Printf("  acton-plugin validate --manifest %s/manifest.json\n", dir)
	fmt.Printf("  acton-plugin build --source %s --output %s/dist/plugin.wasm\n", dir, dir)
	fmt.Printf("  acton-plugin conformance --wasm %s/dist/plugin.wasm --manifest %s/manifest.json\n", dir, dir)
	return nil
}

func mailcalManifest(name string) string {
	return fmt.Sprintf(`{
  "id": "mailcal-%s",
  "name": "%s",
  "version": "1.0.0",
  "description": "Mail provider for ActonOS.",
  "author": "ActonOS Community",
  "license": "MIT",
  "capabilities": ["mail@1"],
  "permissions": {}
}
`, name, strings.Title(strings.ReplaceAll(name, "-", " ")))
}

func mailcalMain(modulePath string) string {
	return `//go:build wasip1

package main

import (
	"` + modulePath + `/internal/backend"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
	"github.com/actonos/plugin-sdk/sdk/mailcal/guest"
)

func main() {}

var b = backend.Backend{}

//go:wasmexport acton_alloc
func actonAlloc(size uint32) uint32 { return guest.Alloc(size) }

//go:wasmexport acton_free
func actonFree(ptr uint32) { guest.Free(ptr) }

//go:wasmexport acton_plugin_init
func actonPluginInit(ptr, length uint32) uint32 { return 0 }

//go:wasmexport acton_mailcal_capabilities
func capabilities(p, l uint32) uint64 { return guest.Serve(b, abi.ExportCapabilities, p, l) }

//go:wasmexport acton_mail_list
func mailList(p, l uint32) uint64 { return guest.Serve(b, abi.ExportMailList, p, l) }
`
}

const mailcalBackend = `// Package backend is where a provider translates ABI calls into its service's API.
package backend

import (
	"context"

	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
)

// Backend embeds mailcal.Base, so every operation you do not override answers "unsupported".
// List exactly the operations you implement in Capabilities().Ops.
type Backend struct{ mailcal.Base }

func (Backend) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{
		ABI: []string{abi.CapMail},
		Ops: []string{abi.ExportCapabilities, abi.ExportMailList},
	}, nil
}

func (Backend) MailList(context.Context, abi.AccountRef, abi.MailListParams) (abi.MailListResult, error) {
	return abi.MailListResult{}, nil
}
`

func mailcalBackendTest(modulePath string) string {
	return `package backend_test

import (
	"context"
	"testing"

	"` + modulePath + `/internal/backend"
	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
	"github.com/actonos/plugin-sdk/sdk/mailcal/conformance"
)

type caller struct{ b mailcal.Backend }

func (c caller) CallJSON(ctx context.Context, export string, req []byte) ([]byte, error) {
	return mailcal.Dispatch(ctx, c.b, export, req), nil
}

func TestBackendPassesTheShapeConformanceSuite(t *testing.T) {
	conformance.RunShape(t, mailcal.NewBridgeProvider(caller{backend.Backend{}}), abi.AccountRef{ID: "acct"})
}
`
}

// moduleImportPath returns the Go import path of dir by locating the enclosing go.mod.
func moduleImportPath(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for root := abs; ; root = filepath.Dir(root) {
		raw, err := os.ReadFile(filepath.Join(root, "go.mod"))
		if err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				if mod, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
					rel, _ := filepath.Rel(root, abs)
					return strings.TrimSpace(mod) + "/" + filepath.ToSlash(rel), nil
				}
			}
			return "", fmt.Errorf("no module line in %s", filepath.Join(root, "go.mod"))
		}
		if filepath.Dir(root) == root {
			return "", fmt.Errorf("%s is not inside a Go module; create mail plugins inside the SDK (for example --dir plugins/mail/<name>)", dir)
		}
	}
}
