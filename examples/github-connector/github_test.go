package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/acton-plugin-sdk/host"
)

func TestGitHubConnectorWasm(t *testing.T) {
	wasmPath := filepath.Join(t.TempDir(), "plugin.wasm")

	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", wasmPath, ".")
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compilation error: %v (out: %s)", err, string(out))
	}

	ctx := context.Background()
	mockHost, err := host.NewMockHost(ctx)
	if err != nil {
		t.Fatalf("mock host init: %v", err)
	}
	defer mockHost.Close()

	mockHost.SetVaultSecret("github_access_token", "ghp_mocktoken123456789")
	mockHost.MockHTTPRoute("https://api.github.com/user/repos", host.HTTPMockResponse{
		Status:  200,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `[{"name":"actonos","full_name":"actonos/actonos"},{"name":"acton-sdk","full_name":"actonos/acton-sdk"}]`,
	})

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	defer runner.Close()

	// Test action: list_repos
	outData, err := runner.DispatchConnectorAction("list_repos", ListReposInput{Limit: 5})
	if err != nil {
		t.Fatalf("dispatch action error: %v", err)
	}

	t.Logf("GitHub list_repos output: %+v", outData)
}
