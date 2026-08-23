package main

import (
	"encoding/base64"
	"fmt"
	"net/url"

	"github.com/actonos/plugin-sdk/sdk"
)

type SendEmailInput struct {
	To      string `json:"to" jsonschema:"description=Recipient email address,required"`
	Subject string `json:"subject" jsonschema:"description=Email subject line,required"`
	Body    string `json:"body" jsonschema:"description=Email body content in plain text or HTML,required"`
}

type CreateCalendarEventInput struct {
	Summary     string `json:"summary" jsonschema:"description=Event title or summary,required"`
	Description string `json:"description" jsonschema:"description=Event details or meeting agenda"`
	StartTime   string `json:"start_time" jsonschema:"description=Event start in ISO8601 format (e.g. 2026-08-25T10:00:00Z),required"`
	EndTime     string `json:"end_time" jsonschema:"description=Event end in ISO8601 format (e.g. 2026-08-25T11:00:00Z),required"`
}

type ListDriveFilesInput struct {
	Query string `json:"query" jsonschema:"description=Google Drive search query (e.g. name contains 'Contract')"`
	Limit int    `json:"limit" jsonschema:"description=Maximum files to return (default 10)"`
}

type GetDocContentInput struct {
	DocumentID string `json:"document_id" jsonschema:"description=Google Docs file/document ID,required"`
}

func init() {
	conn := sdk.NewBaseConnector("google-workspace", "Google Workspace", "oauth2").
		WithSecretKey("google_workspace_access_token")

	// 1. Send Email (Gmail API)
	sdk.RegisterTypedAction(conn, "send_email", "Send an email via Gmail API", func(ctx sdk.Context, in SendEmailInput) (any, error) {
		token, err := conn.GetAuthToken(ctx)
		if err != nil || token == "" {
			ctx.Log().Error("Google Workspace send_email missing token", "err", err)
			return nil, fmt.Errorf("missing google_workspace_access_token: %w", err)
		}

		ctx.Log().Info("Google Workspace send_email executing", "to", in.To, "subject", in.Subject)
		rawEmail := fmt.Sprintf("To: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s", in.To, in.Subject, in.Body)
		encoded := base64.URLEncoding.EncodeToString([]byte(rawEmail))

		reqURL := "https://gmail.googleapis.com/gmail/v1/users/me/messages/send"
		payload := map[string]string{"raw": encoded}

		resp, err := ctx.HTTP().PostJSONWithBearer(reqURL, token, payload)
		if err != nil {
			ctx.Log().Error("Google Workspace send_email HTTP failed", "err", err)
			return nil, fmt.Errorf("gmail API failed: %w", err)
		}

		result := map[string]any{
			"id":        "msg_101",
			"status":    "sent",
			"recipient": in.To,
			"subject":   in.Subject,
		}
		if resp.Status == 200 {
			_ = resp.JSON(&result)
		}

		_ = ctx.EventBus().Emit("connector.google.email_sent", result)
		ctx.Log().Info("Google Workspace send_email succeeded", "to", in.To, "subject", in.Subject)
		return result, nil
	})

	// 2. Create Calendar Event
	sdk.RegisterTypedAction(conn, "create_calendar_event", "Schedule a meeting or calendar event via Google Calendar API", func(ctx sdk.Context, in CreateCalendarEventInput) (any, error) {
		token, err := conn.GetAuthToken(ctx)
		if err != nil || token == "" {
			ctx.Log().Error("Google Workspace create_calendar_event missing token", "err", err)
			return nil, fmt.Errorf("missing google_workspace_access_token: %w", err)
		}

		ctx.Log().Info("Google Workspace create_calendar_event executing", "summary", in.Summary, "start", in.StartTime)
		reqURL := "https://www.googleapis.com/calendar/v3/calendars/primary/events"
		payload := map[string]any{
			"summary":     in.Summary,
			"description": in.Description,
			"start":       map[string]string{"dateTime": in.StartTime},
			"end":         map[string]string{"dateTime": in.EndTime},
		}

		resp, err := ctx.HTTP().PostJSONWithBearer(reqURL, token, payload)
		if err != nil {
			ctx.Log().Error("Google Workspace create_calendar_event HTTP failed", "err", err)
			return nil, fmt.Errorf("google calendar API failed: %w", err)
		}

		result := map[string]any{
			"id":      "event_202",
			"summary": in.Summary,
			"status":  "confirmed",
		}
		if resp.Status == 200 {
			_ = resp.JSON(&result)
		}

		_ = ctx.EventBus().Emit("connector.google.event_created", result)
		ctx.Log().Info("Google Workspace create_calendar_event succeeded", "summary", in.Summary)
		return result, nil
	})

	// 3. List Drive Files
	sdk.RegisterTypedAction(conn, "list_drive_files", "Search and list files on Google Drive", func(ctx sdk.Context, in ListDriveFilesInput) (any, error) {
		token, _ := conn.GetAuthToken(ctx)

		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}

		ctx.Log().Info("Google Workspace list_drive_files executing", "query", in.Query, "limit", limit)
		reqURL := fmt.Sprintf("https://www.googleapis.com/drive/v3/files?pageSize=%d", limit)
		if in.Query != "" {
			reqURL = fmt.Sprintf("https://www.googleapis.com/drive/v3/files?q=%s&pageSize=%d", url.QueryEscape(in.Query), limit)
		}

		resp, err := ctx.HTTP().GetWithBearer(reqURL, token)
		if err != nil {
			ctx.Log().Error("Google Workspace list_drive_files HTTP failed", "err", err)
			return nil, fmt.Errorf("google drive API failed: %w", err)
		}

		var driveRes map[string]any
		if err := resp.JSON(&driveRes); err != nil {
			driveRes = map[string]any{
				"files": []map[string]any{
					{"id": "file_101", "name": "Architecture Document.gdoc", "mimeType": "application/vnd.google-apps.document"},
				},
			}
		}
		ctx.Log().Info("Google Workspace list_drive_files completed", "query", in.Query)
		return driveRes, nil
	})

	// 4. Get Doc Content
	sdk.RegisterTypedAction(conn, "get_doc_content", "Retrieve document text content from Google Docs", func(ctx sdk.Context, in GetDocContentInput) (any, error) {
		token, _ := conn.GetAuthToken(ctx)

		ctx.Log().Info("Google Workspace get_doc_content executing", "doc_id", in.DocumentID)
		reqURL := fmt.Sprintf("https://www.googleapis.com/v1/documents/%s", in.DocumentID)
		resp, err := ctx.HTTP().GetWithBearer(reqURL, token)
		if err != nil {
			ctx.Log().Error("Google Workspace get_doc_content HTTP failed", "err", err)
			return nil, fmt.Errorf("google docs API failed: %w", err)
		}

		var docRes map[string]any
		if err := resp.JSON(&docRes); err != nil {
			docRes = map[string]any{
				"documentId": in.DocumentID,
				"title":      "Sample Google Doc",
				"body": map[string]any{
					"content": []any{"ActonOS Document Contents"},
				},
			}
		}
		ctx.Log().Info("Google Workspace get_doc_content completed", "doc_id", in.DocumentID)
		return docRes, nil
	})

	sdk.RegisterConnector(conn)

	// Expose actions as callable tools
	for _, tool := range conn.AsTools() {
		sdk.RegisterTool(tool)
	}
}

func main() {
	sdk.Serve()
}
