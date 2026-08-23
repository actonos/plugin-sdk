package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/plugin-sdk/host"
)

func TestGitHubPluginWasm(t *testing.T) {
	wasmPath := filepath.Join(t.TempDir(), "plugin.wasm")

	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", wasmPath, ".")
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\nOutput:\n%s", err, string(out))
	}

	ctx := context.Background()
	mockHost, err := host.NewMockHost(ctx)
	if err != nil {
		t.Fatalf("mock host init: %v", err)
	}
	defer mockHost.Close()

	mockHost.SetAllowedDomains([]string{"api.github.com"})
	mockHost.SetVaultSecret("github_access_token", "gho_mock_oauth_token_123")

	mockHost.MockHTTPRoute("https://api.github.com/user/repos", host.HTTPMockResponse{
		Status: 200,
		Body:   `[{"name": "ActonOS", "full_name": "actonos/actonos"}, {"name": "ActonOS-Plugin-SDK", "full_name": "actonos/ActonOS-Plugin-SDK"}]`,
	})

	mockHost.MockHTTPRoute("https://api.github.com/repos/actonos/actonos/issues", host.HTTPMockResponse{
		Status: 201,
		Body:   `{"number": 42, "title": "Add Plugin Marketplace", "state": "open"}`,
	})

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	defer runner.Close()

	// 1. Test dispatch connector action: list_repos
	res, err := runner.DispatchConnectorAction("list_repos", ListReposInput{Limit: 5})
	if err != nil {
		t.Fatalf("list_repos failed: %v", err)
	}
	t.Logf("list_repos output: %v", res)

	// 2. Test Agent Tool invocation bridged from connector: connector_github_create_issue
	issueInput := []byte(`{"owner":"actonos","repo":"actonos","title":"Add Plugin Marketplace","body":"Need marketplace for WASM plugins"}`)
	toolRes, err := runner.ExecuteTool("connector_github_create_issue", issueInput)
	if err != nil || toolRes.Error != "" {
		t.Fatalf("tool execution failed: %v, error: %s", err, toolRes.Error)
	}
	t.Logf("Tool execution result: %s", toolRes.Content)
}
