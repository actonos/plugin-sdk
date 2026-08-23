package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/acton-plugin-sdk/host"
)

func TestJiraPluginWasm(t *testing.T) {
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

	mockHost.SetAllowedDomains([]string{"api.atlassian.com"})
	mockHost.SetVaultSecret("jira_api_token", "jira_mock_api_token_123")
	mockHost.SetVaultSecret("jira_cloud_id", "actonos-cloud-id")

	mockHost.MockHTTPRoute("https://api.atlassian.com/ex/jira/actonos-cloud-id/rest/api/3/search", host.HTTPMockResponse{
		Status: 200,
		Body: `{
			"issues": [
				{"key": "ACT-1", "fields": {"summary": "Implement Wazero HAL driver"}}
			]
		}`,
	})

	mockHost.MockHTTPRoute("https://api.atlassian.com/ex/jira/actonos-cloud-id/rest/api/3/issue", host.HTTPMockResponse{
		Status: 201,
		Body:   `{"key": "ACT-100", "id": "10001"}`,
	})

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	defer runner.Close()

	// 1. Test dispatch connector action: search_issues_jql
	res, err := runner.DispatchConnectorAction("search_issues_jql", JiraSearchJQLInput{JQL: "project = 'ACT'"})
	if err != nil {
		t.Fatalf("search_issues_jql failed: %v", err)
	}
	t.Logf("search_issues_jql output: %v", res)

	// 2. Test Agent Tool invocation bridged from connector: connector_jira_create_issue
	createInput := []byte(`{"project_key":"ACT","summary":"Memory bounds check for WASM","issue_type":"Bug"}`)
	toolRes, err := runner.ExecuteTool("connector_jira_create_issue", createInput)
	if err != nil || toolRes.Error != "" {
		t.Fatalf("tool execution failed: %v, error: %s", err, toolRes.Error)
	}
	t.Logf("Tool execution result: %s", toolRes.Content)
}
