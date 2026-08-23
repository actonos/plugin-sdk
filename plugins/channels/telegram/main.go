package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/actonos/plugin-sdk/sdk"
)

// TelegramConfig defines the root configuration model for the Telegram channel plugin.
type TelegramConfig struct {
	PollIntervalSeconds int               `json:"poll_interval_seconds"`
	Accounts            []TelegramAccount `json:"accounts"`
}

// TelegramAccount represents an individual configured Telegram bot instance.
type TelegramAccount struct {
	AccountID    string `json:"account_id"`
	DisplayName  string `json:"display_name"`
	BotToken     string `json:"bot_token,omitempty"`
	DefaultAgent string `json:"default_agent"`
}

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

type TelegramAPIResponse struct {
	OK          bool             `json:"ok"`
	ErrorCode   int              `json:"error_code,omitempty"`
	Description string           `json:"description,omitempty"`
	Result      []TelegramUpdate `json:"result,omitempty"`
}

func (t *TelegramChannel) SendMessage(ctx sdk.Context, msg sdk.OutboundMessage) error {
	accountID := msg.AccountID
	if accountID == "" {
		accountID = msg.Metadata["account_id"]
	}
	if accountID == "" {
		accountID = "default"
	}

	token, err := getTelegramBotToken(ctx, TelegramAccount{AccountID: accountID})
	if err != nil {
		ctx.Log().Error("Telegram SendMessage token error", "account_id", accountID, "err", err)
		return err
	}

	// Prioritize chat_id from metadata (group/channel chat), fallback to recipient
	chatID := msg.Metadata["chat_id"]
	if chatID == "" {
		chatID = msg.Recipient
	}
	if chatID == "" {
		return fmt.Errorf("recipient or chat_id is required")
	}

	reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload := map[string]any{
		"chat_id": chatID,
		"text":    msg.Content,
	}

	if parseMode, ok := msg.Metadata["parse_mode"]; ok && parseMode != "" {
		payload["parse_mode"] = parseMode
	}
	if replyTo, ok := msg.Metadata["reply_to_msg_id"]; ok && replyTo != "" {
		if id, err := strconv.Atoi(replyTo); err == nil {
			payload["reply_to_message_id"] = id
		}
	}

	resp, err := ctx.HTTP().PostJSON(reqURL, payload)
	if err != nil {
		ctx.Log().Error("Telegram sendMessage network error", "chat_id", chatID, "err", err)
		return fmt.Errorf("telegram sendMessage API failed: %w", err)
	}

	// If Markdown parse error (400 Bad Request), retry as plain text
	if resp.Status == 400 && payload["parse_mode"] != nil {
		ctx.Log().Warn("Telegram markdown parse failed, retrying plain text", "body", resp.Body)
		delete(payload, "parse_mode")
		resp, err = ctx.HTTP().PostJSON(reqURL, payload)
		if err != nil {
			return fmt.Errorf("telegram sendMessage plain retry failed: %w", err)
		}
	}

	if resp.Status != 200 {
		ctx.Log().Error("Telegram API returned error status", "status", resp.Status, "body", resp.Body)
		return fmt.Errorf("telegram API returned HTTP status %d: %s", resp.Status, resp.Body)
	}

	_ = ctx.EventBus().Emit("channel.telegram.sent", map[string]string{
		"account_id": accountID,
		"chat_id":    chatID,
		"status":     "sent",
	})
	return nil
}

func (t *TelegramChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
	var cfg TelegramConfig
	_ = ctx.Config().Bind(&cfg)

	accounts := cfg.Accounts
	if len(accounts) == 0 {
		accounts = []TelegramAccount{
			{
				AccountID:   "default",
				DisplayName: "Default Telegram Bot",
			},
		}
	}

	var allInbound []sdk.InboundMessage

	for _, acc := range accounts {
		token, err := getTelegramBotToken(ctx, acc)
		if err != nil || token == "" {
			ctx.Log().Debug("Telegram bot token not found for account", "account_id", acc.AccountID)
			continue
		}

		offsetKey := fmt.Sprintf("last_update_id_%s", acc.AccountID)
		offset := 0
		if rawOffset, ok, _ := ctx.Storage().Get(offsetKey); ok && rawOffset != "" {
			offset, _ = strconv.Atoi(rawOffset)
		} else if rawOffset, ok, _ := ctx.Storage().Get("last_update_id"); ok && rawOffset != "" {
			offset, _ = strconv.Atoi(rawOffset)
		}

		reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?limit=10", token)
		if offset > 0 {
			reqURL = fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&limit=10", token, offset)
		}

		resp, err := ctx.HTTP().Get(reqURL)
		if err != nil {
			ctx.Log().Warn("Telegram getUpdates request failed", "account_id", acc.AccountID, "err", err)
			continue
		}

		// Handle Webhook Conflict (409) -> automatically delete existing webhook so getUpdates works
		if resp.Status == 409 || (resp.Body != "" && strings.Contains(resp.Body, "can't use getUpdates method while webhook is active")) {
			ctx.Log().Info("Telegram active webhook detected, clearing webhook for polling mode...", "account_id", acc.AccountID)
			delURL := fmt.Sprintf("https://api.telegram.org/bot%s/deleteWebhook?drop_pending_updates=false", token)
			_, _ = ctx.HTTP().Get(delURL)
			continue
		}

		if resp.Status != 200 {
			ctx.Log().Warn("Telegram getUpdates returned non-200", "account_id", acc.AccountID, "status", resp.Status, "body", resp.Body)
			continue
		}

		var apiResp TelegramAPIResponse
		if err := resp.JSON(&apiResp); err != nil {
			ctx.Log().Warn("Telegram getUpdates JSON parse failed", "err", err, "raw", resp.Body)
			continue
		}

		if !apiResp.OK {
			ctx.Log().Warn("Telegram getUpdates returned ok=false", "error_code", apiResp.ErrorCode, "desc", apiResp.Description)
			continue
		}

		maxID := offset
		for _, update := range apiResp.Result {
			if update.UpdateID >= maxID {
				maxID = update.UpdateID + 1
			}
			if update.Message != nil && update.Message.Text != "" {
				senderID := strconv.FormatInt(update.Message.From.ID, 10)
				senderName := update.Message.From.Username
				if senderName == "" {
					senderName = update.Message.From.FirstName
				}

				targetAgent, cleanContent := sdk.ExtractAgentMention(update.Message.Text)
				if targetAgent == "" && acc.DefaultAgent != "" {
					targetAgent = acc.DefaultAgent
				}

				inbound := sdk.NewInboundMessage(
					"telegram",
					acc.AccountID,
					senderID,
					senderName,
					cleanContent,
				)
				inbound.TargetAgent = targetAgent
				inbound.Metadata["chat_id"] = strconv.FormatInt(update.Message.Chat.ID, 10)
				inbound.Metadata["message_id"] = strconv.Itoa(update.Message.MessageID)
				inbound.Metadata["account_id"] = acc.AccountID

				allInbound = append(allInbound, inbound)
				_ = ctx.EventBus().Emit("channel.telegram.received", inbound)
				ctx.Log().Info("Telegram message received", "from", senderName, "chat_id", inbound.Metadata["chat_id"], "target_agent", targetAgent)
			}
		}

		if maxID > offset {
			_ = ctx.Storage().Set(offsetKey, strconv.Itoa(maxID))
		}
	}

	return allInbound, nil
}

func getTelegramBotToken(ctx sdk.Context, acc TelegramAccount) (string, error) {
	if acc.BotToken != "" {
		return acc.BotToken, nil
	}

	accountID := acc.AccountID
	if accountID != "" && accountID != "default" {
		if token, err := ctx.Vault().GetSecret("telegram_bot_tokens." + accountID); err == nil && token != "" {
			return token, nil
		}
		if token, err := ctx.Vault().GetSecret("telegram_bot_token_" + accountID); err == nil && token != "" {
			return token, nil
		}
		if token, err := ctx.Vault().GetSecret("accounts." + accountID + ".bot_token"); err == nil && token != "" {
			return token, nil
		}
	}

	if token, err := ctx.Vault().GetSecret("telegram_bot_token"); err == nil && token != "" {
		return token, nil
	}
	if token, err := ctx.Vault().GetSecret("bot_token"); err == nil && token != "" {
		return token, nil
	}

	return "", fmt.Errorf("missing telegram bot token for account %q in vault or config", accountID)
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
