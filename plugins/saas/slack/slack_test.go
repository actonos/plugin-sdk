package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/plugin-sdk/host"
)

func TestSlackSaaSPluginWasm(t *testing.T) {
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

	mockHost.SetAllowedDomains([]string{"slack.com"})
	mockHost.SetVaultSecret("slack_bot_token", "xoxb-mock-slack-saas-token")

	mockHost.MockHTTPRoute("https://slack.com/api/conversations.list", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"ok": true, "channels": [{"id": "C01", "name": "general"}]}`,
	})

	mockHost.MockHTTPRoute("https://slack.com/api/chat.postMessage", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"ok": true, "channel": "C01", "ts": "1710000000.000100"}`,
	})

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	defer runner.Close()

	// 1. Test dispatch connector action: list_channels
	res, err := runner.DispatchConnectorAction("list_channels", ListChannelsInput{Limit: 5})
	if err != nil {
		t.Fatalf("list_channels failed: %v", err)
	}
	t.Logf("list_channels output: %v", res)

	// 2. Test Agent Tool invocation bridged from connector: connector_slack_post_message
	postInput := []byte(`{"channel":"C01","text":"System alert: Backup completed successfully"}`)
	toolRes, err := runner.ExecuteTool("connector_slack_post_message", postInput)
	if err != nil || toolRes.Error != "" {
		t.Fatalf("tool execution failed: %v, error: %s", err, toolRes.Error)
	}
	t.Logf("Tool execution result: %s", toolRes.Content)
}
