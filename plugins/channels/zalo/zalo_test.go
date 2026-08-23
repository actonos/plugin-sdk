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

func TestZaloPluginWasm(t *testing.T) {
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

	mockHost.SetAllowedDomains([]string{"openapi.zalo.me"})
	mockHost.SetVaultSecret("zalo_oa_access_token", "zalo_oa_secret_123")

	mockHost.MockHTTPRoute("https://openapi.zalo.me/v3.0/oa/message/cs", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"error": 0, "message": "Success", "data": {"message_id": "zalo_msg_100"}}`,
	})

	webhookJSON := `[
		{
			"app_id": "123456",
			"event_name": "user_send_text",
			"sender": {"id": "zalo_user_888"},
			"recipient": {"id": "oa_999"},
			"message": {
				"msg_id": "zalo_incoming_1",
				"text": "@coder xin chào! Giúp tôi viết code ActonOS"
			}
		}
	]`

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	defer runner.Close()

	// 1. Send outbound message
	outMsg := sdk.NewOutboundMessage("zalo", "zalo_user_888", "Xin chào bạn từ ActonOS AI!")
	if err := runner.SendChannelMessage(outMsg); err != nil {
		t.Fatalf("send message failed: %v", err)
	}

	// 2. Poll webhook queue
	mockHost.SetStorage("pending_zalo_webhook", webhookJSON)
	msgs, err := runner.PollChannelMessages()
	if err != nil {
		t.Fatalf("poll channel failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message from queue, got %d", len(msgs))
	}
	if msgs[0].TargetAgent != "coder" {
		t.Fatalf("expected target agent 'coder', got '%s'", msgs[0].TargetAgent)
	}
	if msgs[0].Content != "xin chào! Giúp tôi viết code ActonOS" {
		t.Fatalf("expected clean content, got '%s'", msgs[0].Content)
	}
}
