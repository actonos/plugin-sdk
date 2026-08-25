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

func TestWhatsAppPluginWasm(t *testing.T) {
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

	mockHost.SetAllowedDomains([]string{"graph.facebook.com"})
	mockHost.SetVaultSecret("whatsapp_access_token", "eaab_mock_token")
	mockHost.SetVaultSecret("whatsapp_phone_number_id", "10987654321")

	// Pre-seed pending webhook payload in storage
	webhookJSON := `{
		"entry": [
			{
				"changes": [
					{
						"value": {
							"contacts": [{"profile": {"name": "Charlie"}, "wa_id": "+84901234567"}],
							"messages": [
								{
									"from": "+84901234567",
									"id": "wamid.HBgL",
									"text": {"body": "@researcher summarize the latest AI news"}
								}
							]
						}
					}
				]
			}
		]
	}`

	mockHost.MockHTTPRoute("https://graph.facebook.com/v21.0/10987654321/messages", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"messaging_product": "whatsapp", "contacts": [{"input": "+84901234567", "wa_id": "+84901234567"}], "messages": [{"id": "wamid.HBgL_OUT"}]}`,
	})

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	defer runner.Close()

	// 1. Send outbound message
	outMsg := sdk.NewOutboundMessage("whatsapp", "+84901234567", "Here is your AI news summary!")
	if err := runner.SendChannelMessage(outMsg); err != nil {
		t.Fatalf("send message failed: %v", err)
	}

	// 2. Poll webhook queue
	mockHost.SetStorage("pending_webhook_queue", webhookJSON)
	msgs, err := runner.PollChannelMessages()
	if err != nil {
		t.Fatalf("poll channel failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message from queue, got %d", len(msgs))
	}
	if msgs[0].TargetAgent != "researcher" {
		t.Fatalf("expected target agent 'researcher', got '%s'", msgs[0].TargetAgent)
	}
	if msgs[0].Content != "summarize the latest AI news" {
		t.Fatalf("expected clean content, got '%s'", msgs[0].Content)
	}
	if msgs[0].ChatID != "+84901234567" {
		t.Fatalf("expected ChatID '+84901234567', got '%s'", msgs[0].ChatID)
	}
	if msgs[0].MessageID != "wamid.HBgL" {
		t.Fatalf("expected MessageID 'wamid.HBgL', got '%s'", msgs[0].MessageID)
	}
}
