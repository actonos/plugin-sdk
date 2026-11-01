package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/plugin-sdk/sdk/mailcal/certificate"
)

func TestConformanceCommandCertifiesGmailAndBindsTheCertificateToTheBuild(t *testing.T) {
	dir := t.TempDir()
	wasm := filepath.Join(dir, "plugin.wasm")
	build := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", wasm, "../../plugins/mail/gmail")
	build.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building: %v\n%s", err, b)
	}
	if err := runConformance([]string{"--wasm", wasm, "--manifest", "../../plugins/mail/gmail/manifest.json"}); err != nil {
		t.Fatalf("conformance failed: %v", err)
	}
	c, err := certificate.Read(filepath.Join(dir, certificate.CertificateFile))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(wasm)
	if !certificate.Verify(c, raw) || c.PluginID != "mailcal-gmail" || !c.ShapeOK || c.Live {
		t.Fatalf("bad certificate: %+v", c)
	}
}

func TestConformanceCommandRejectsAPluginThatIsNotAMailProvider(t *testing.T) {
	err := runConformance([]string{"--wasm", "nope.wasm", "--manifest", "../../plugins/saas/github/manifest.json"})
	if err == nil {
		t.Fatal("a connector plugin must not be certified as a mail provider")
	}
}
