package host_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/plugin-sdk/host"
	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
)

func buildMailWasm(t *testing.T, pkg string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "plugin.wasm")
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", out, pkg)
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building %s: %v\n%s", pkg, err, b)
	}
	return out
}

func newProbe(t *testing.T) (*host.MockHost, mailcal.Provider) {
	t.Helper()
	ctx := context.Background()
	h, err := host.NewMockHost(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	runner, err := h.LoadPluginFromFile(ctx, buildMailWasm(t, "./testdata/mailprobe"))
	if err != nil {
		t.Fatalf("loading the probe: %v", err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	return h, mailcal.NewBridgeProvider(runner)
}

var mailAcct = abi.AccountRef{ID: "acct-1"}

func TestProbeDeclaresItsCapabilities(t *testing.T) {
	_, p := newProbe(t)
	caps, err := p.Capabilities(context.Background(), mailAcct)
	if err != nil || !caps.Supports(abi.ExportMailList) {
		t.Fatalf("%+v %v", caps, err)
	}
}

func TestProbeUsesTheTokenTheHostGrants(t *testing.T) {
	h, p := newProbe(t)
	h.SetOAuthToken("acct-1", "tok-123")
	h.MockHTTPRoute("https://api.example.com/probe", host.HTTPMockResponse{Status: 200, Body: `{}`})
	res, err := p.MailList(context.Background(), mailAcct, abi.MailListParams{})
	if err != nil || len(res.Messages) != 1 || res.Messages[0].Subject != "tok-123|200" {
		t.Fatalf("got %+v, %v", res, err)
	}
}

func TestProbeMapsEveryOAuthRefusalToACodedError(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*host.MockHost)
		want  abi.ErrorCode
	}{
		{"no token configured", func(*host.MockHost) {}, abi.ErrTransient},
		{"sign-in expired", func(h *host.MockHost) { h.SetOAuthToken("acct-1", "t"); h.ExpireOAuthAccount("acct-1") }, abi.ErrAuthExpired},
		{"account belongs to another plugin", func(h *host.MockHost) { h.SetOAuthToken("acct-1", "t"); h.DenyOAuthAccount("acct-1") }, abi.ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, p := newProbe(t)
			tc.setup(h)
			if _, err := p.MailList(context.Background(), mailAcct, abi.MailListParams{}); !mailcal.IsCode(err, tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestAnExportThePluginLacksIsUnsupported(t *testing.T) {
	_, p := newProbe(t)
	if _, err := p.MailSend(context.Background(), mailAcct, abi.MailSendParams{}); !mailcal.IsCode(err, abi.ErrUnsupported) {
		t.Fatalf("want unsupported, got %v", err)
	}
}
