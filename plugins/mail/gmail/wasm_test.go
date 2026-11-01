package main_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/plugin-sdk/host"
	"github.com/actonos/plugin-sdk/sdk"
	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
	"github.com/actonos/plugin-sdk/sdk/mailcal/conformance"
)

func load(t *testing.T) (*host.MockHost, mailcal.Provider) {
	t.Helper()
	wasm := filepath.Join(t.TempDir(), "plugin.wasm")
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", wasm, ".")
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the plugin: %v\n%s", err, b)
	}
	ctx := context.Background()
	h, err := host.NewMockHost(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	r, err := h.LoadPluginFromFile(ctx, wasm)
	if err != nil {
		t.Fatalf("loading the plugin: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return h, mailcal.NewBridgeProvider(r)
}

func TestManifestAsksForOnlyWhatItNeeds(t *testing.T) {
	raw, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m sdk.PluginManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.ID != "mailcal-gmail" || m.OAuth == nil || m.OAuth.Provider != "google" ||
		len(m.Permissions.OAuth) != 1 || m.Permissions.OAuth[0] != "google" {
		t.Fatalf("id/oauth block: %q %+v %+v", m.ID, m.OAuth, m.Permissions)
	}
	if len(m.Permissions.Secrets) != 0 || m.Permissions.Storage || m.Permissions.Workspace || len(m.Permissions.BusEvents) != 0 {
		t.Fatalf("a provider needs no secrets, storage, workspace or bus: %+v", m.Permissions)
	}
	for _, s := range m.OAuth.Scopes {
		if s == "https://mail.google.com/" {
			t.Fatal("the full-mailbox scope (permanent delete) must not be requested")
		}
	}
}

func TestCompiledPluginDeclaresMailAndCalendarOperations(t *testing.T) {
	_, p := load(t)
	caps, err := p.Capabilities(context.Background(), abi.AccountRef{ID: "probe"})
	if err != nil || !caps.Supports(abi.ExportMailSend) || !caps.Supports(abi.ExportCalFreeBusy) {
		t.Fatalf("%+v %v", caps, err)
	}
}

// With no token the plugin must fail with a coded error, not crash or hang.
func TestCompiledPluginWithoutATokenFailsWithACodedError(t *testing.T) {
	_, p := load(t)
	if _, err := p.MailList(context.Background(), abi.AccountRef{ID: "probe"}, abi.MailListParams{}); mailcal.AsError(err) == nil {
		t.Fatalf("expected a coded error, got %v", err)
	}
}

func TestCompiledPluginPassesTheOfflineConformanceSuite(t *testing.T) {
	_, p := load(t)
	conformance.RunShape(t, p, abi.AccountRef{ID: "probe"})
}
