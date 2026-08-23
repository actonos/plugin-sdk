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

func TestTelegramChannelWasm(t *testing.T) {
	wasmPath := filepath.Join(t.TempDir(), "plugin.wasm")

	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", wasmPath, ".")
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compilation error: %v (out: %s)", err, string(out))
	}

	ctx := context.Background()
	mockHost, err := host.NewMockHost(ctx)
	if err != nil {
		t.Fatalf("mock host init: %v", err)
	}
	defer mockHost.Close()

	mockHost.SetVaultSecret("telegram_bot_token", "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11")
	mockHost.MockHTTPRoute("https://api.telegram.org/bot", host.HTTPMockResponse{
		Status:  200,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `{"ok":true,"result":[{"update_id":100,"message":{"message_id":1,"from":{"id":123,"first_name":"Alice","username":"alice_dev"},"chat":{"id":123},"text":"Hello ActonOS!"}}]}`,
	})

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	defer runner.Close()

	// Test 1: Send Message
	err = runner.SendChannelMessage(sdk.OutboundMessage{
		ChannelID: "telegram",
		Recipient: "12345678",
		Content:   "Testing from ActonOS Agent",
	})
	if err != nil {
		t.Fatalf("send message error: %v", err)
	}

	// Test 2: Poll Messages
	msgs, err := runner.PollChannelMessages()
	if err != nil {
		t.Fatalf("poll messages error: %v", err)
	}

	if len(msgs) != 1 {
		t.Fatalf("expected 1 inbound message, got %d", len(msgs))
	}

	if msgs[0].Content != "Hello ActonOS!" {
		t.Errorf("unexpected message content: %s", msgs[0].Content)
	}

	t.Logf("Polled message: from=%s, content=%s", msgs[0].SenderName, msgs[0].Content)
}
