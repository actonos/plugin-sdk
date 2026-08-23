package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/acton-plugin-sdk/host"
)

func TestLinearPluginWasm(t *testing.T) {
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

	mockHost.SetAllowedDomains([]string{"api.linear.app"})
	mockHost.SetVaultSecret("linear_api_key", "lin_api_mock_secret_key")

	mockHost.MockHTTPRoute("https://api.linear.app/graphql", host.HTTPMockResponse{
		Status: 200,
		Body: `{
			"data": {
				"issues": {
					"nodes": [
						{"id": "iss_101", "identifier": "ACT-101", "title": "Build Wazero Sandbox"}
					]
				}
			}
		}`,
	})

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	defer runner.Close()

	// 1. Test dispatch connector action: list_issues
	res, err := runner.DispatchConnectorAction("list_issues", LinearListIssuesInput{Limit: 5})
	if err != nil {
		t.Fatalf("list_issues failed: %v", err)
	}
	t.Logf("list_issues output: %v", res)

	// 2. Test Agent Tool invocation bridged from connector: connector_linear_create_issue
	createInput := []byte(`{"team_id":"team_eng","title":"Support WebAssembly Plugin ABI","priority":1}`)
	toolRes, err := runner.ExecuteTool("connector_linear_create_issue", createInput)
	if err != nil || toolRes.Error != "" {
		t.Fatalf("tool execution failed: %v, error: %s", err, toolRes.Error)
	}
	t.Logf("Tool execution result: %s", toolRes.Content)
}
