package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/plugin-sdk/host"
)

func TestGoogleWorkspacePluginWasm(t *testing.T) {
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

	mockHost.SetAllowedDomains([]string{"gmail.googleapis.com", "www.googleapis.com"})
	mockHost.SetVaultSecret("google_workspace_access_token", "ya29.mock_google_oauth_token")

	mockHost.MockHTTPRoute("https://gmail.googleapis.com/gmail/v1/users/me/messages/send", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"id": "msg_gmail_123", "threadId": "thread_123", "labelIds": ["SENT"]}`,
	})

	mockHost.MockHTTPRoute("https://www.googleapis.com/calendar/v3/calendars/primary/events", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"id": "event_cal_456", "summary": "AI Team Standup", "status": "confirmed"}`,
	})

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	defer runner.Close()

	// 1. Test dispatch connector action: send_email
	emailInput := SendEmailInput{
		To:      "dev@actonos.org",
		Subject: "ActonOS v1.0 Launch",
		Body:    "Team, v1.0 is ready!",
	}
	res, err := runner.DispatchConnectorAction("send_email", emailInput)
	if err != nil {
		t.Fatalf("send_email failed: %v", err)
	}
	t.Logf("send_email result: %v", res)

	// 2. Test Agent Tool invocation bridged from connector: connector_google-workspace_create_calendar_event
	calPayload := []byte(`{"summary":"AI Standup","start_time":"2026-08-25T10:00:00Z","end_time":"2026-08-25T11:00:00Z"}`)
	toolRes, err := runner.ExecuteTool("connector_google-workspace_create_calendar_event", calPayload)
	if err != nil || toolRes.Error != "" {
		t.Fatalf("tool execution failed: %v, error: %s", err, toolRes.Error)
	}
	t.Logf("Tool execution result: %s", toolRes.Content)
}
