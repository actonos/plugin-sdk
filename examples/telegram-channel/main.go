package main

import (
	"fmt"
	"strconv"

	"github.com/actonos/acton-plugin-sdk/sdk"
)

type TelegramChannel struct {
	sdk.BaseChannel
}

type TelegramUpdateResponse struct {
	OK     bool `json:"ok"`
	Result []struct {
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
	if err != nil {
		return fmt.Errorf("retrieving telegram_bot_token from vault: %w", err)
	}

	ctx.Log().Info("Dispatching outbound message via Telegram Bot", "recipient", msg.Recipient)

	reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload := map[string]any{
		"chat_id":    msg.Recipient,
		"text":       msg.Content,
		"parse_mode": "Markdown",
	}

	resp, err := ctx.HTTP().PostJSON(reqURL, payload)
	if err != nil {
		return fmt.Errorf("calling telegram API sendMessage: %w", err)
	}

	if resp.Status != 200 {
		return fmt.Errorf("telegram API returned HTTP status %d: %s", resp.Status, resp.Body)
	}

	return nil
}

func (c *TelegramChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
	token, err := ctx.Vault().GetSecret("telegram_bot_token")
	if err != nil {
		return nil, fmt.Errorf("retrieving telegram_bot_token: %w", err)
	}

	// Read last offset from KV storage
	offsetStr, ok, _ := ctx.Storage().Get("telegram_last_update_id")
	offset := 0
	if ok && offsetStr != "" {
		offset, _ = strconv.Atoi(offsetStr)
	}

	reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&limit=10&timeout=2", token, offset)
	resp, err := ctx.HTTP().Get(reqURL)
	if err != nil {
		return nil, err
	}

	var tgResp TelegramUpdateResponse
	if err := resp.JSON(&tgResp); err != nil || !tgResp.OK {
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

		messages = append(messages, sdk.InboundMessage{
			ChannelID:  "telegram",
			AccountID:  "bot_primary",
			SenderID:   strconv.FormatInt(item.Message.Chat.ID, 10),
			SenderName: senderName,
			Content:    item.Message.Text,
			Metadata: map[string]string{
				"update_id":  strconv.Itoa(item.UpdateID),
				"message_id": strconv.Itoa(item.Message.MessageID),
			},
		})
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
