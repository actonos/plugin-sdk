package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/plugin-sdk/host"
)

func TestNotionPluginWasm(t *testing.T) {
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

	mockHost.SetAllowedDomains([]string{"api.notion.com"})
	mockHost.SetVaultSecret("notion_api_key", "secret_notion_api_key_123")

	mockHost.MockHTTPRoute("https://api.notion.com/v1/search", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"results": [{"id": "page_actonos_roadmap", "object": "page", "url": "https://notion.so/roadmap"}]}`,
	})

	mockHost.MockHTTPRoute("https://api.notion.com/v1/pages", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"id": "page_actonos_new_doc", "object": "page", "url": "https://notion.so/new_doc"}`,
	})

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	defer runner.Close()

	// 1. Test dispatch connector action: search_pages
	res, err := runner.DispatchConnectorAction("search_pages", SearchPagesInput{Query: "roadmap"})
	if err != nil {
		t.Fatalf("search_pages failed: %v", err)
	}
	t.Logf("search_pages output: %v", res)

	// 2. Test Agent Tool invocation bridged from connector: connector_notion_create_page
	pageInput := []byte(`{"parent_page_id":"page_123","title":"Sprint 42 Goals","content":"Ship ActonOS Plugins"}`)
	toolRes, err := runner.ExecuteTool("connector_notion_create_page", pageInput)
	if err != nil || toolRes.Error != "" {
		t.Fatalf("tool execution failed: %v, error: %s", err, toolRes.Error)
	}
	t.Logf("Tool execution result: %s", toolRes.Content)
}
