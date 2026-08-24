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

func TestZaloBotPlatformPluginWasm(t *testing.T) {
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

	mockHost.SetAllowedDomains([]string{"bot-api.zaloplatforms.com", "openapi.zalo.me"})
	mockHost.SetVaultSecret("zalo_bot_token", "test_zalo_bot_token_123456")

	// 1. Mock sendMessage endpoint
	mockHost.MockHTTPRoute("https://bot-api.zaloplatforms.com/bottest_zalo_bot_token_123456/sendMessage", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"ok": true, "result": {"message_id": "zalo_msg_888", "date": 1749632637199}}`,
	})

	// 2. Mock sendChatAction endpoint
	mockHost.MockHTTPRoute("https://bot-api.zaloplatforms.com/bottest_zalo_bot_token_123456/sendChatAction", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"ok": true, "result": true}`,
	})

	// 3. Mock getUpdates endpoint
	mockHost.MockHTTPRoute("https://bot-api.zaloplatforms.com/bottest_zalo_bot_token_123456/getUpdates", host.HTTPMockResponse{
		Status: 200,
		Body: `{
			"ok": true,
			"result": [
				{
					"event_name": "message.text.received",
					"message": {
						"message_id": "update_msg_001",
						"date": 1750316131602,
						"from": {
							"id": "user_zalo_ted",
							"display_name": "Ted Nguyen",
							"is_bot": false
						},
						"chat": {
							"id": "user_zalo_ted",
							"chat_type": "PRIVATE"
						},
						"text": "@support Xin chào, tôi cần trợ giúp kỹ thuật"
					}
				}
			]
		}`,
	})

	// 4. Sample webhook queue payload
	webhookJSON := `[
		{
			"ok": true,
			"result": {
				"event_name": "message.text.received",
				"message": {
					"message_id": "webhook_msg_999",
					"date": 1750316131602,
					"from": {
						"id": "user_zalo_alice",
						"display_name": "Alice Pham",
						"is_bot": false
					},
					"chat": {
						"id": "chat_group_123",
						"chat_type": "GROUP"
					},
					"text": "@coder Hãy giúp tôi giải thích kiến trúc ActonOS"
				}
			}
		}
	]`

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	defer runner.Close()

	// 1. Test SendMessage
	outMsg := sdk.NewOutboundMessage("zalo", "user_zalo_ted", "Chào bạn! Đây là tin nhắn từ **ActonOS AI Assistant**.")
	if err := runner.SendChannelMessage(outMsg); err != nil {
		t.Fatalf("send message failed: %v", err)
	}

	// 2. Test Typing indicator
	typingMsg := sdk.NewOutboundMessage("zalo", "user_zalo_ted", "")
	typingMsg.Metadata["typing"] = "true"
	if err := runner.SendChannelMessage(typingMsg); err != nil {
		t.Fatalf("send typing indicator failed: %v", err)
	}

	// 3. Test PollMessages (from pending webhook storage + getUpdates)
	mockHost.SetStorage("pending_zalo_webhook", webhookJSON)
	msgs, err := runner.PollChannelMessages()
	if err != nil {
		t.Fatalf("poll channel messages failed: %v", err)
	}

	// Expecting 2 messages: 1 from webhook queue + 1 from getUpdates
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	// Check message 1 (from webhook queue)
	if msgs[0].TargetAgent != "coder" {
		t.Errorf("msg 0: expected target agent 'coder', got '%s'", msgs[0].TargetAgent)
	}
	if msgs[0].SenderName != "Alice Pham" {
		t.Errorf("msg 0: expected sender name 'Alice Pham', got '%s'", msgs[0].SenderName)
	}
	if msgs[0].Metadata["chat_type"] != "GROUP" {
		t.Errorf("msg 0: expected chat_type 'GROUP', got '%s'", msgs[0].Metadata["chat_type"])
	}

	// Check message 2 (from getUpdates polling)
	if msgs[1].TargetAgent != "support" {
		t.Errorf("msg 1: expected target agent 'support', got '%s'", msgs[1].TargetAgent)
	}
	if msgs[1].SenderName != "Ted Nguyen" {
		t.Errorf("msg 1: expected sender name 'Ted Nguyen', got '%s'", msgs[1].SenderName)
	}
}
