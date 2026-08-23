package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/acton-plugin-sdk/host"
	"github.com/actonos/acton-plugin-sdk/sdk"
)

func TestSlackPluginWasm(t *testing.T) {
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
	mockHost.SetVaultSecret("slack_bot_token", "xoxb-mock-bot-token")

	mockHost.MockHTTPRoute("https://slack.com/api/conversations.history", host.HTTPMockResponse{
		Status: 200,
		Body: `{
			"ok": true,
			"messages": [
				{
					"type": "message",
					"user": "U12345",
					"text": "/agent devops check cluster health",
					"ts": "1710000000.000100"
				}
			]
		}`,
	})

	mockHost.MockHTTPRoute("https://slack.com/api/chat.postMessage", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"ok": true, "ts": "1710000000.000200", "channel": "C0123"}`,
	})

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	defer runner.Close()

	// 1. Poll incoming message
	msgs, err := runner.PollChannelMessages()
	if err != nil {
		t.Fatalf("poll channel failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].TargetAgent != "devops" {
		t.Fatalf("expected target agent 'devops', got '%s'", msgs[0].TargetAgent)
	}
	if msgs[0].Content != "check cluster health" {
		t.Fatalf("expected clean content, got '%s'", msgs[0].Content)
	}

	// 2. Send outbound message
	outMsg := sdk.NewOutboundMessage("slack", "C0123", "All cluster nodes are healthy.")
	if err := runner.SendChannelMessage(outMsg); err != nil {
		t.Fatalf("send message failed: %v", err)
	}
}
