package main

import (
	"fmt"
	"strconv"

	"github.com/actonos/acton-plugin-sdk/sdk"
)

type TelegramChannel struct {
	sdk.BaseChannel
}

type TelegramUpdate struct {
	UpdateID int `json:"update_id"`
	Message  *struct {
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
}

func (t *TelegramChannel) SendMessage(ctx sdk.Context, msg sdk.OutboundMessage) error {
	token, err := ctx.Vault().GetSecret("telegram_bot_token")
	if err != nil || token == "" {
		return fmt.Errorf("missing telegram_bot_token in vault: %w", err)
	}

	reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload := map[string]any{
		"chat_id": msg.Recipient,
		"text":    msg.Content,
	}

	if parseMode, ok := msg.Metadata["parse_mode"]; ok {
		payload["parse_mode"] = parseMode
	}
	if replyTo, ok := msg.Metadata["reply_to_msg_id"]; ok {
		if id, err := strconv.Atoi(replyTo); err == nil {
			payload["reply_to_message_id"] = id
		}
	}

	resp, err := ctx.HTTP().PostJSON(reqURL, payload)
	if err != nil {
		return fmt.Errorf("telegram sendMessage API failed: %w", err)
	}
	if resp.Status != 200 {
		return fmt.Errorf("telegram API returned HTTP status %d: %s", resp.Status, resp.Body)
	}

	_ = ctx.EventBus().Emit("channel.telegram.sent", map[string]string{
		"recipient": msg.Recipient,
		"status":    "sent",
	})
	return nil
}

func (t *TelegramChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
	token, err := ctx.Vault().GetSecret("telegram_bot_token")
	if err != nil || token == "" {
		return nil, fmt.Errorf("missing telegram_bot_token in vault: %w", err)
	}

	offset := 0
	if rawOffset, ok, _ := ctx.Storage().Get("last_update_id"); ok && rawOffset != "" {
		offset, _ = strconv.Atoi(rawOffset)
	}

	reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&limit=10", token, offset)
	resp, err := ctx.HTTP().Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("telegram getUpdates failed: %w", err)
	}

	var updateResponse struct {
		OK     bool             `json:"ok"`
		Result []TelegramUpdate `json:"result"`
	}

	if err := resp.JSON(&updateResponse); err != nil {
		return nil, fmt.Errorf("parsing getUpdates JSON: %w", err)
	}

	var inboundMsgs []sdk.InboundMessage
	maxID := offset

	for _, update := range updateResponse.Result {
		if update.UpdateID >= maxID {
			maxID = update.UpdateID + 1
		}
		if update.Message != nil && update.Message.Text != "" {
			senderID := strconv.FormatInt(update.Message.From.ID, 10)
			senderName := update.Message.From.Username
			if senderName == "" {
				senderName = update.Message.From.FirstName
			}

			inbound := sdk.NewInboundMessage(
				"telegram",
				"default",
				senderID,
				senderName,
				update.Message.Text,
			)
			inbound.Metadata["chat_id"] = strconv.FormatInt(update.Message.Chat.ID, 10)
			inbound.Metadata["message_id"] = strconv.Itoa(update.Message.MessageID)

			inboundMsgs = append(inboundMsgs, inbound)
			_ = ctx.EventBus().Emit("channel.telegram.received", inbound)
		}
	}

	if maxID > offset {
		_ = ctx.Storage().Set("last_update_id", strconv.Itoa(maxID))
	}

	return inboundMsgs, nil
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
