package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/actonos/plugin-sdk/sdk"
)

type TelegramConfig struct {
	PollIntervalSeconds int               `json:"poll_interval_seconds"`
	Accounts            []TelegramAccount `json:"accounts"`
	// Legacy root-level fields kept for backward-compatible config binding.
	TelegramBotToken string `json:"telegram_bot_token"`
	BotToken         string `json:"bot_token"`
	DefaultAgent     string `json:"default_agent"`
	ParseMode        string `json:"parse_mode"`
}

type TelegramAccount struct {
	sdk.ChannelAccount
	ParseMode string `json:"parse_mode,omitempty"`
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
		Date int64  `json:"date"`
	} `json:"message"`
}

type TelegramAPIResponse struct {
	OK          bool             `json:"ok"`
	ErrorCode   int              `json:"error_code,omitempty"`
	Description string           `json:"description,omitempty"`
	Result      []TelegramUpdate `json:"result,omitempty"`
}

func (t *TelegramChannel) SendMessage(ctx sdk.Context, msg sdk.OutboundMessage) error {
	acc, token, err := resolveTelegramAccount(ctx, msg.AccountID)
	if err != nil {
		ctx.Log().Error("Telegram SendMessage token error", "account_id", msg.AccountID, "err", err)
		return err
	}

	chatID := sdk.FirstNonEmpty(msg.ChatID, msg.Recipient)
	if chatID == "" {
		return fmt.Errorf("recipient or chat_id is required")
	}

	if msg.WantsTyping() && acc.TypingEnabled() {
		sendTelegramChatAction(ctx, token, chatID, msg.ChatAction())
		if msg.IsTypingOnly() {
			return nil
		}
	} else if msg.IsTypingOnly() {
		return nil
	}

	if msg.Reaction != "" && acc.AckReactionEnabled() {
		if msgID, convErr := strconv.Atoi(msg.ReplyToID); convErr == nil {
			setTelegramReaction(ctx, token, chatID, msgID, sdk.MapReactionForPlatform("telegram", msg.Reaction))
		}
		if msg.Kind == sdk.MessageKindReaction && msg.IsControlOnly() {
			return nil
		}
	} else if msg.Kind == sdk.MessageKindReaction && msg.IsControlOnly() {
		return nil
	}

	reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	chunks := sdk.SplitMessage(msg.Content, 3900)

	parseMode := sdk.FirstNonEmpty(msg.Metadata["parse_mode"], acc.ParseMode, "Markdown")

	for i, chunk := range chunks {
		payload := map[string]any{
			"chat_id":    chatID,
			"text":       chunk,
			"parse_mode": parseMode,
		}

		if acc.ReplyQuoteEnabled() && i == 0 && msg.ReplyToID != "" {
			if id, convErr := strconv.Atoi(msg.ReplyToID); convErr == nil {
				payload["reply_to_message_id"] = id
			}
		}

		resp, err := ctx.HTTP().PostJSON(reqURL, payload)
		if err != nil {
			ctx.Log().Error("Telegram sendMessage network error", "chat_id", chatID, "chunk", i+1, "err", err)
			return fmt.Errorf("telegram sendMessage API failed: %w", err)
		}

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
	}

	_ = ctx.EventBus().Emit("channel.telegram.sent", map[string]string{
		"account_id": acc.AccountID,
		"chat_id":    chatID,
		"status":     "sent",
		"chunks":     strconv.Itoa(len(chunks)),
	})
	return nil
}

func (t *TelegramChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
	var cfg TelegramConfig
	_ = ctx.Config().Bind(&cfg)

	accounts := activeTelegramAccounts(cfg)
	var allInbound []sdk.InboundMessage

	for _, acc := range accounts {
		token := getTelegramBotToken(ctx, acc)
		if token == "" {
			continue
		}

		offsetKey := fmt.Sprintf("telegram_offset_%s", acc.AccountID)
		lastOffsetStr, ok, _ := ctx.Storage().Get(offsetKey)
		offset := 0
		if ok && lastOffsetStr != "" {
			offset, _ = strconv.Atoi(lastOffsetStr)
		}

		pollInterval := cfg.PollIntervalSeconds
		if pollInterval <= 0 {
			pollInterval = 2
		}

		reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?limit=20&timeout=%d", token, pollInterval)
		if offset > 0 {
			reqURL = fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&limit=20&timeout=%d", token, offset, pollInterval)
		}

		resp, err := ctx.HTTP().Get(reqURL)
		if err != nil {
			ctx.Log().Warn("Telegram getUpdates network error", "account_id", acc.AccountID, "err", err)
			continue
		}

		if resp.Status == 409 || strings.Contains(resp.Body, "webhook is active") {
			ctx.Log().Info("Active webhook detected, deleting webhook to enable long-polling...", "account_id", acc.AccountID)
			delURL := fmt.Sprintf("https://api.telegram.org/bot%s/deleteWebhook?drop_pending_updates=false", token)
			_, _ = ctx.HTTP().Get(delURL)
			continue
		}

		if resp.Status != 200 {
			ctx.Log().Warn("Telegram getUpdates non-200 status", "status", resp.Status, "body", resp.Body)
			continue
		}

		var apiResp TelegramAPIResponse
		if err := resp.JSON(&apiResp); err != nil {
			ctx.Log().Warn("Failed to parse Telegram updates response", "account_id", acc.AccountID, "body", resp.Body)
			continue
		}

		if !apiResp.OK {
			ctx.Log().Warn("Telegram getUpdates returned ok=false", "error_code", apiResp.ErrorCode, "desc", apiResp.Description)
			continue
		}

		listenTarget := acc.ResolveListenTarget()
		maxID := offset
		for _, update := range apiResp.Result {
			if update.UpdateID >= maxID {
				maxID = update.UpdateID + 1
			}
			if update.Message == nil || update.Message.Text == "" {
				continue
			}

			chatID := strconv.FormatInt(update.Message.Chat.ID, 10)
			if listenTarget != "" && listenTarget != chatID {
				continue
			}

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
			ts := ""
			if update.Message.Date > 0 {
				ts = strconv.FormatInt(update.Message.Date, 10)
			}
			sdk.ApplyInboundEnvelope(&inbound, chatID, strconv.Itoa(update.Message.MessageID), "", ts)

			allInbound = append(allInbound, inbound)
			_ = ctx.EventBus().Emit("channel.telegram.received", inbound)
			ctx.Log().Info("Telegram message received", "from", senderName, "chat_id", chatID, "target_agent", targetAgent)

			if acc.TypingEnabled() {
				sendTelegramChatAction(ctx, token, chatID, "typing")
			}
			if acc.AckReactionEnabled() {
				setTelegramReaction(ctx, token, chatID, update.Message.MessageID, sdk.MapReactionForPlatform("telegram", acc.ReactionEmoji()))
			}
		}

		if maxID > offset {
			_ = ctx.Storage().Set(offsetKey, strconv.Itoa(maxID))
		}
	}

	return allInbound, nil
}

func sendTelegramChatAction(ctx sdk.Context, token, chatID, action string) {
	if token == "" || chatID == "" {
		return
	}
	if action == "" {
		action = "typing"
	}
	reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendChatAction", token)
	_, _ = ctx.HTTP().PostJSON(reqURL, map[string]string{
		"chat_id": chatID,
		"action":  action,
	})
}

func setTelegramReaction(ctx sdk.Context, token, chatID string, messageID int, emoji string) {
	if token == "" || chatID == "" || messageID == 0 || emoji == "" {
		return
	}
	reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/setMessageReaction", token)
	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"reaction": []map[string]string{
			{
				"type":  "emoji",
				"emoji": emoji,
			},
		},
	}
	_, _ = ctx.HTTP().PostJSON(reqURL, payload)
}

func activeTelegramAccounts(cfg TelegramConfig) []TelegramAccount {
	if len(cfg.Accounts) > 0 {
		return cfg.Accounts
	}
	return []TelegramAccount{
		{
			ChannelAccount: sdk.ChannelAccount{
				AccountID:    "default",
				DisplayName:  "Default Telegram Bot",
				BotToken:     sdk.FirstNonEmpty(cfg.TelegramBotToken, cfg.BotToken),
				DefaultAgent: cfg.DefaultAgent,
			},
			ParseMode: cfg.ParseMode,
		},
	}
}

func getTelegramBotToken(ctx sdk.Context, acc TelegramAccount) string {
	return sdk.ResolveSecret(ctx, acc.BotToken, sdk.AccountVaultKeys(acc.AccountID, "telegram_bot_tokens", "telegram_bot_token", "bot_token", "token")...)
}

func resolveTelegramAccount(ctx sdk.Context, accountID string) (TelegramAccount, string, error) {
	var cfg TelegramConfig
	_ = ctx.Config().Bind(&cfg)
	accounts := activeTelegramAccounts(cfg)

	acc := TelegramAccount{ChannelAccount: sdk.ChannelAccount{AccountID: accountID}}
	for _, a := range accounts {
		if a.AccountID == accountID || (accountID == "default" && (a.AccountID == "" || a.AccountID == "default")) {
			acc = a
			break
		}
	}
	if acc.AccountID == "" {
		acc.AccountID = "default"
	}
	token := getTelegramBotToken(ctx, acc)
	if token == "" {
		return acc, "", fmt.Errorf("missing telegram bot token for account %q in vault or config", acc.AccountID)
	}
	return acc, token, nil
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
