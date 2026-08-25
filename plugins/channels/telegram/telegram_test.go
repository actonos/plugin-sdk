package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/plugin-sdk/host"
	"github.com/actonos/plugin-sdk/sdk"
)

func TestTelegramPluginWasm(t *testing.T) {
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

	mockHost.SetAllowedDomains([]string{"api.telegram.org"})
	mockHost.SetVaultSecret("telegram_bot_token", "test_bot_token_123")

	mockHost.MockHTTPRoute("https://api.telegram.org/bottest_bot_token_123/getUpdates", host.HTTPMockResponse{
		Status: 200,
		Body: `{
			"ok": true,
			"result": [
				{
					"update_id": 1001,
					"message": {
						"message_id": 42,
						"from": {"id": 999, "first_name": "Alice", "username": "alice_dev"},
						"chat": {"id": 888},
						"text": "@coder review pull request"
					}
				}
			]
		}`,
	})

	mockHost.MockHTTPRoute("https://api.telegram.org/bottest_bot_token_123/sendMessage", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"ok": true, "result": {"message_id": 43}}`,
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
	if msgs[0].TargetAgent != "coder" {
		t.Fatalf("expected target agent 'coder', got '%s'", msgs[0].TargetAgent)
	}
	if msgs[0].Content != "review pull request" {
		t.Fatalf("expected cleaned content 'review pull request', got '%s'", msgs[0].Content)
	}
	if msgs[0].ChatID != "888" {
		t.Fatalf("expected ChatID '888', got '%s'", msgs[0].ChatID)
	}
	if msgs[0].MessageID != "42" {
		t.Fatalf("expected MessageID '42', got '%s'", msgs[0].MessageID)
	}
	if msgs[0].Kind != sdk.MessageKindText {
		t.Fatalf("expected kind text, got '%s'", msgs[0].Kind)
	}

	// 2. Send outbound message
	outMsg := sdk.NewOutboundMessage("telegram", "888", "Hello Alice from ActonOS Agent!")
	if err := runner.SendChannelMessage(outMsg); err != nil {
		t.Fatalf("send message failed: %v", err)
	}
}
