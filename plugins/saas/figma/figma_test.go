package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/plugin-sdk/host"
)

func TestFigmaPluginWasm(t *testing.T) {
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

	mockHost.SetAllowedDomains([]string{"api.figma.com"})
	mockHost.SetVaultSecret("figma_access_token", "figd_mock_personal_token_123")

	mockHost.MockHTTPRoute("https://api.figma.com/v1/files/sample_key_123", host.HTTPMockResponse{
		Status: 200,
		Body: `{
			"name": "ActonOS Device UI Mockup",
			"lastModified": "2026-08-24T00:00:00Z"
		}`,
	})

	mockHost.MockHTTPRoute("https://api.figma.com/v1/files/sample_key_123/comments", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"id": "comm_101", "message": "LGTM! Approved for design tokens generation"}`,
	})

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	defer runner.Close()

	// 1. Test dispatch connector action: get_file
	res, err := runner.DispatchConnectorAction("get_file", FigmaGetFileInput{FileKey: "sample_key_123"})
	if err != nil {
		t.Fatalf("get_file failed: %v", err)
	}
	t.Logf("get_file output: %v", res)

	// 2. Test Agent Tool invocation bridged from connector: connector_figma_post_comment
	postInput := []byte(`{"file_key":"sample_key_123","message":"LGTM! Approved for design tokens generation","x":120.5,"y":340.0}`)
	toolRes, err := runner.ExecuteTool("connector_figma_post_comment", postInput)
	if err != nil || toolRes.Error != "" {
		t.Fatalf("tool execution failed: %v, error: %s", err, toolRes.Error)
	}
	t.Logf("Tool execution result: %s", toolRes.Content)
}
