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

func TestDiscordPluginWasm(t *testing.T) {
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

	mockHost.SetAllowedDomains([]string{"discord.com"})
	mockHost.SetVaultSecret("discord_bot_token", "discord_bot_secret_123")

	mockHost.MockHTTPRoute("https://discord.com/api/v10/channels/default/messages", host.HTTPMockResponse{
		Status: 200,
		Body: `[
			{
				"id": "msg_999",
				"channel_id": "default",
				"content": "<@!11223344> /agent devops deploy to production",
				"author": {
					"id": "user_456",
					"username": "bob_admin",
					"bot": false
				}
			}
		]`,
	})

	mockHost.MockHTTPRoute("https://discord.com/api/v10/channels/12345/messages", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"id": "msg_1000", "channel_id": "12345"}`,
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
	if msgs[0].Content != "deploy to production" {
		t.Fatalf("expected cleaned content 'deploy to production', got '%s'", msgs[0].Content)
	}
	if msgs[0].ChatID != "default" {
		t.Fatalf("expected ChatID 'default', got '%s'", msgs[0].ChatID)
	}
	if msgs[0].MessageID != "msg_999" {
		t.Fatalf("expected MessageID 'msg_999', got '%s'", msgs[0].MessageID)
	}

	// 2. Send outbound message
	outMsg := sdk.NewOutboundMessage("discord", "12345", "Deployment completed successfully!")
	if err := runner.SendChannelMessage(outMsg); err != nil {
		t.Fatalf("send message failed: %v", err)
	}
}
