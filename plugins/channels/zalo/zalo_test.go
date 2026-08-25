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

	// 1. Mock getMe endpoint
	mockHost.MockHTTPRoute("https://bot-api.zaloplatforms.com/bottest_zalo_bot_token_123456/getMe", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"ok": true, "result": {"id": "bot_9999", "account_name": "ActonOS Assistant", "account_type": "bot", "can_join_groups": true}}`,
	})

	// 2. Mock sendMessage endpoint
	mockHost.MockHTTPRoute("https://bot-api.zaloplatforms.com/bottest_zalo_bot_token_123456/sendMessage", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"ok": true, "result": {"message_id": "zalo_msg_888", "date": 1749632637199}}`,
	})

	// 3. Mock sendPhoto endpoint
	mockHost.MockHTTPRoute("https://bot-api.zaloplatforms.com/bottest_zalo_bot_token_123456/sendPhoto", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"ok": true, "result": {"message_id": "zalo_photo_777", "date": 1749632637200}}`,
	})

	// 4. Mock sendDocument endpoint
	mockHost.MockHTTPRoute("https://bot-api.zaloplatforms.com/bottest_zalo_bot_token_123456/sendDocument", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"ok": true, "result": {"message_id": "zalo_doc_666", "date": 1749632637201}}`,
	})

	// 5. Mock sendVoice endpoint
	mockHost.MockHTTPRoute("https://bot-api.zaloplatforms.com/bottest_zalo_bot_token_123456/sendVoice", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"ok": true, "result": {"message_id": "zalo_voice_555", "date": 1749632637202}}`,
	})

	// 6. Mock sendChatAction endpoint
	mockHost.MockHTTPRoute("https://bot-api.zaloplatforms.com/bottest_zalo_bot_token_123456/sendChatAction", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"ok": true, "result": true}`,
	})

	// 7. Mock deleteWebhook endpoint
	mockHost.MockHTTPRoute("https://bot-api.zaloplatforms.com/bottest_zalo_bot_token_123456/deleteWebhook", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"ok": true, "result": true}`,
	})

	// 8. Mock getUpdates endpoint
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

	// 9. Sample webhook queue payload
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

	// 1. Test SendMessage (Text with markdown & quote reply)
	outMsg := sdk.NewOutboundMessage("zalo", "user_zalo_ted", "Chào bạn! Đây là tin nhắn từ **ActonOS AI Assistant**.")
	outMsg.Metadata["reply_to_message_id"] = "update_msg_001"
	if err := runner.SendChannelMessage(outMsg); err != nil {
		t.Fatalf("send message failed: %v", err)
	}

	// 2. Test Send Photo attachment
	photoMsg := sdk.NewOutboundMessage("zalo", "user_zalo_ted", "Ảnh kiến trúc hệ thống")
	photoMsg.Metadata["photo"] = "https://example.com/architecture.png"
	if err := runner.SendChannelMessage(photoMsg); err != nil {
		t.Fatalf("send photo failed: %v", err)
	}

	// 3. Test Send Document attachment
	docMsg := sdk.NewOutboundMessage("zalo", "user_zalo_ted", "Báo cáo tài liệu")
	docMsg.Metadata["document"] = "https://example.com/report.pdf"
	docMsg.Metadata["file_name"] = "report.pdf"
	if err := runner.SendChannelMessage(docMsg); err != nil {
		t.Fatalf("send document failed: %v", err)
	}

	// 4. Test Send Voice attachment
	voiceMsg := sdk.NewOutboundMessage("zalo", "user_zalo_ted", "Tin nhắn thoại")
	voiceMsg.Metadata["voice"] = "https://example.com/audio.ogg"
	if err := runner.SendChannelMessage(voiceMsg); err != nil {
		t.Fatalf("send voice failed: %v", err)
	}

	// 5. Test Explicit Typing indicator
	typingMsg := sdk.NewOutboundMessage("zalo", "user_zalo_ted", "")
	typingMsg.Metadata["typing"] = "true"
	if err := runner.SendChannelMessage(typingMsg); err != nil {
		t.Fatalf("send typing indicator failed: %v", err)
	}

	// 6. Test PollMessages (from pending webhook storage + getUpdates)
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
	if msgs[0].ChatID != "chat_group_123" {
		t.Errorf("msg 0: expected ChatID 'chat_group_123', got '%s'", msgs[0].ChatID)
	}
	if msgs[0].MessageID != "webhook_msg_999" {
		t.Errorf("msg 0: expected MessageID 'webhook_msg_999', got '%s'", msgs[0].MessageID)
	}

	// Check message 2 (from getUpdates polling)
	if msgs[1].TargetAgent != "support" {
		t.Errorf("msg 1: expected target agent 'support', got '%s'", msgs[1].TargetAgent)
	}
	if msgs[1].SenderName != "Ted Nguyen" {
		t.Errorf("msg 1: expected sender name 'Ted Nguyen', got '%s'", msgs[1].SenderName)
	}
}
