package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/actonos/plugin-sdk/sdk"
)

type TelegramChannel struct {
	sdk.BaseChannel
}

type TelegramUpdateResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code,omitempty"`
	Description string `json:"description,omitempty"`
	Result      []struct {
		UpdateID int `json:"update_id"`
		Message  struct {
			MessageID int `json:"message_id"`
			From      struct {
				ID        int64  `json:"id"`
				FirstName string `json:"first_name"`
				Username  string `json:"username"`
			} `json:"from"`
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
			Text string `json:"text"`
		} `json:"message"`
	} `json:"result"`
}

func (c *TelegramChannel) SendMessage(ctx sdk.Context, msg sdk.OutboundMessage) error {
	token, err := ctx.Vault().GetSecret("telegram_bot_token")
	if err != nil || token == "" {
		return fmt.Errorf("retrieving telegram_bot_token from vault: %w", err)
	}

	chatID := msg.Metadata["chat_id"]
	if chatID == "" {
		chatID = msg.Recipient
	}
	if chatID == "" {
		return fmt.Errorf("recipient or chat_id is required")
	}

	ctx.Log().Info("Dispatching outbound message via Telegram Bot", "chat_id", chatID)

	reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload := map[string]any{
		"chat_id":    chatID,
		"text":       msg.Content,
		"parse_mode": "Markdown",
	}

	resp, err := ctx.HTTP().PostJSON(reqURL, payload)
	if err != nil {
		return fmt.Errorf("calling telegram API sendMessage: %w", err)
	}

	// Retry without parse_mode if Markdown parsing failed
	if resp.Status == 400 {
		ctx.Log().Warn("Telegram markdown failed, retrying plain text", "body", resp.Body)
		delete(payload, "parse_mode")
		resp, err = ctx.HTTP().PostJSON(reqURL, payload)
		if err != nil {
			return fmt.Errorf("calling telegram API retry: %w", err)
		}
	}

	if resp.Status != 200 {
		return fmt.Errorf("telegram API returned HTTP status %d: %s", resp.Status, resp.Body)
	}

	return nil
}

func (c *TelegramChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
	token, err := ctx.Vault().GetSecret("telegram_bot_token")
	if err != nil || token == "" {
		return nil, fmt.Errorf("retrieving telegram_bot_token: %w", err)
	}

	// Read last offset from KV storage
	offsetStr, ok, _ := ctx.Storage().Get("telegram_last_update_id")
	offset := 0
	if ok && offsetStr != "" {
		offset, _ = strconv.Atoi(offsetStr)
	}

	reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?limit=10&timeout=2", token)
	if offset > 0 {
		reqURL = fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&limit=10&timeout=2", token, offset)
	}

	resp, err := ctx.HTTP().Get(reqURL)
	if err != nil {
		ctx.Log().Warn("Telegram getUpdates network error", "err", err)
		return nil, err
	}

	// If webhook conflict, delete webhook
	if resp.Status == 409 || strings.Contains(resp.Body, "webhook is active") {
		ctx.Log().Info("Deleting old telegram webhook to allow polling...")
		delURL := fmt.Sprintf("https://api.telegram.org/bot%s/deleteWebhook?drop_pending_updates=false", token)
		_, _ = ctx.HTTP().Get(delURL)
		return nil, nil
	}

	if resp.Status != 200 {
		ctx.Log().Warn("Telegram getUpdates non-200 status", "status", resp.Status, "body", resp.Body)
		return nil, nil
	}

	var tgResp TelegramUpdateResponse
	if err := resp.JSON(&tgResp); err != nil || !tgResp.OK {
		ctx.Log().Warn("Telegram getUpdates parse error or not ok", "body", resp.Body)
		return nil, nil
	}

	var messages []sdk.InboundMessage
	maxID := offset

	for _, item := range tgResp.Result {
		if item.UpdateID >= maxID {
			maxID = item.UpdateID + 1
		}
		if item.Message.Text == "" {
			continue
		}

		senderName := item.Message.From.FirstName
		if item.Message.From.Username != "" {
			senderName = "@" + item.Message.From.Username
		}

		chatIDStr := strconv.FormatInt(item.Message.Chat.ID, 10)
		messages = append(messages, sdk.InboundMessage{
			ChannelID:  "telegram",
			AccountID:  "default",
			SenderID:   strconv.FormatInt(item.Message.From.ID, 10),
			SenderName: senderName,
			Content:    item.Message.Text,
			Metadata: map[string]string{
				"update_id":  strconv.Itoa(item.UpdateID),
				"message_id": strconv.Itoa(item.Message.MessageID),
				"chat_id":    chatIDStr,
			},
		})
		ctx.Log().Info("Telegram message received", "sender", senderName, "chat_id", chatIDStr)
	}

	if maxID > offset {
		_ = ctx.Storage().Set("telegram_last_update_id", strconv.Itoa(maxID))
	}

	return messages, nil
}

func init() {
	ch := &TelegramChannel{
		BaseChannel: sdk.BaseChannel{
			ChannelName:        "telegram",
			ChannelDisplayName: "Telegram Bot",
			PairingRequired:    true,
		},
	}

	sdk.RegisterChannel(ch)
}

func main() {
	sdk.Serve()
}
