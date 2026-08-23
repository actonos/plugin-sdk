package host_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/plugin-sdk/host"
	"github.com/actonos/plugin-sdk/sdk"
)

func TestWasmCompilationAndExecution(t *testing.T) {
	wasmPath := filepath.Join(t.TempDir(), "plugin.wasm")
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", wasmPath, "../examples/weather-tool")
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compilation error: %v (output: %s)", err, string(out))
	}

	ctx := context.Background()
	mockHost, err := host.NewMockHost(ctx)
	if err != nil {
		t.Fatalf("mock host init error: %v", err)
	}
	defer mockHost.Close()

	// Configure mock HTTP response for weather API
	mockHost.MockHTTPRoute("https://api.open-meteo.com/v1/forecast", host.HTTPMockResponse{
		Status:  200,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `{"current_weather":{"temperature":24.5,"windspeed":10.2,"weathercode":1,"is_day":1,"time":"2026-08-24T00:00"}}`,
	})

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		t.Fatalf("loading plugin wasm: %v", err)
	}
	defer runner.Close()

	// Execute tool
	res, err := runner.ExecuteTool("get_weather", []byte(`{"city":"Tokyo"}`))
	if err != nil {
		t.Fatalf("execute tool error: %v", err)
	}

	t.Logf("Tool Result: Content='%s', Data=%+v, Error='%s'", res.Content, res.Data, res.Error)
	if res.Error != "" {
		t.Errorf("tool returned error: %s", res.Error)
	}

	if res.Content == "" {
		t.Errorf("expected non-empty content in tool result")
	}

	temp, ok := res.Data["temperature"].(float64)
	if !ok || temp != 24.5 {
		t.Errorf("expected temperature 24.5, got %v", res.Data["temperature"])
	}
}

func TestMultiBotDiscordChannelExecution(t *testing.T) {
	wasmPath := filepath.Join(t.TempDir(), "channel-discord.wasm")
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", wasmPath, "../plugins/channels/discord")
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("discord plugin compilation error: %v (output: %s)", err, string(out))
	}

	ctx := context.Background()
	mockHost, err := host.NewMockHost(ctx)
	if err != nil {
		t.Fatalf("mock host init error: %v", err)
	}
	defer mockHost.Close()

	// 1. Set Hardware Vault secrets for both bots
	mockHost.SetVaultSecret("discord_bot_tokens.bot_cskh", "secret_token_cskh_123")
	mockHost.SetVaultSecret("discord_bot_tokens.bot_devops", "secret_token_devops_456")

	// 2. Mock Discord API routes for each bot's listen channel
	mockHost.MockHTTPRoute("https://discord.com/api/v10/channels/1001/messages", host.HTTPMockResponse{
		Status:  200,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body: `[
			{
				"id": "msg_001",
				"channel_id": "1001",
				"content": "Tôi cần hỗ trợ kỹ thuật gấp",
				"author": {"id": "user_alice", "username": "alice", "bot": false}
			}
		]`,
	})

	mockHost.MockHTTPRoute("https://discord.com/api/v10/channels/2002/messages", host.HTTPMockResponse{
		Status:  200,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body: `[
			{
				"id": "msg_002",
				"channel_id": "2002",
				"content": "@SecurityAuditor kiểm tra lỗ hổng CVE",
				"author": {"id": "user_bob", "username": "bob", "bot": false}
			}
		]`,
	})

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		t.Fatalf("loading discord plugin wasm: %v", err)
	}
	defer runner.Close()

	// 3. Inject Multi-Bot configuration into Plugin Runner
	pluginConfig := map[string]any{
		"poll_interval_seconds": 3,
		"accounts": []map[string]any{
			{
				"account_id":        "bot_cskh",
				"display_name":      "Customer Support Bot",
				"default_agent":     "agent-support",
				"listen_channel_id": "1001",
				"enable_embeds":     true,
			},
			{
				"account_id":        "bot_devops",
				"display_name":      "Infrastructure Bot",
				"default_agent":     "agent-devops",
				"listen_channel_id": "2002",
				"enable_embeds":     false,
			},
		},
	}

	if err := runner.SetPluginConfig(pluginConfig); err != nil {
		t.Fatalf("failed setting plugin config: %v", err)
	}

	// 4. Poll incoming messages from both bots
	inboundMsgs, err := runner.PollChannelMessages()
	if err != nil {
		t.Fatalf("failed polling channel messages: %v", err)
	}

	if len(inboundMsgs) != 2 {
		t.Fatalf("expected 2 inbound messages from 2 bots, got %d", len(inboundMsgs))
	}

	// Verify Message 1 (Bot CSKH -> routed to default agent-support)
	msg1 := inboundMsgs[0]
	if msg1.AccountID != "bot_cskh" {
		t.Errorf("expected msg1 AccountID 'bot_cskh', got %q", msg1.AccountID)
	}
	if msg1.TargetAgent != "agent-support" {
		t.Errorf("expected msg1 TargetAgent 'agent-support', got %q", msg1.TargetAgent)
	}
	if msg1.Content != "Tôi cần hỗ trợ kỹ thuật gấp" {
		t.Errorf("unexpected content: %q", msg1.Content)
	}

	// Verify Message 2 (Bot DevOps -> explicit mention @SecurityAuditor override)
	msg2 := inboundMsgs[1]
	if msg2.AccountID != "bot_devops" {
		t.Errorf("expected msg2 AccountID 'bot_devops', got %q", msg2.AccountID)
	}
	if msg2.TargetAgent != "SecurityAuditor" {
		t.Errorf("expected msg2 TargetAgent 'SecurityAuditor', got %q", msg2.TargetAgent)
	}
	if msg2.Content != "kiểm tra lỗ hổng CVE" {
		t.Errorf("unexpected content: %q", msg2.Content)
	}

	// 5. Send outbound reply through specific bot account
	outboundReply := sdk.OutboundMessage{
		ChannelID: "discord",
		AccountID: "bot_cskh",
		Recipient: "1001",
		Content:   "Dạ ActonOS Support đã tiếp nhận yêu cầu!",
	}
	if err := runner.SendChannelMessage(outboundReply); err != nil {
		t.Fatalf("failed sending outbound message for bot_cskh: %v", err)
	}
}


