package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/actonos/plugin-sdk/sdk"
)

const (
	defaultZaloBotAPIBase = "https://bot-api.zaloplatforms.com"
	maxZaloTextLength     = 2000
)

// ZaloConfig defines the root configuration model for the Zalo Bot Platform channel plugin.
type ZaloConfig struct {
	PollIntervalSeconds int           `json:"poll_interval_seconds"`
	Accounts            []ZaloAccount `json:"accounts"`
	// Legacy root-level fields kept for backward-compatible config binding.
	ZaloBotToken          string `json:"zalo_bot_token"`
	ParseMode             string `json:"parse_mode"`
	DefaultAgent          string `json:"default_agent"`
	EnableTypingIndicator *bool  `json:"enable_typing_indicator"`
	EnableAckReaction     *bool  `json:"enable_ack_reaction"`
	EnableReplyQuote      *bool  `json:"enable_reply_quote"`
}

// ZaloAccount represents an individual configured Zalo Bot account instance.
type ZaloAccount struct {
	sdk.ChannelAccount
	ZaloBotToken string `json:"zalo_bot_token,omitempty"`
	ParseMode    string `json:"parse_mode,omitempty"`
}

type ZaloChannel struct {
	sdk.BaseChannel
}

// --- Zalo Bot Platform Models ---

type ZaloUser struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	IsBot       bool   `json:"is_bot"`
}

type ZaloChat struct {
	ID       string `json:"id"`
	ChatType string `json:"chat_type"` // "PRIVATE" or "GROUP"
}

type ZaloMessagePayload struct {
	MessageID string    `json:"message_id"`
	Date      int64     `json:"date"`
	From      *ZaloUser `json:"from,omitempty"`
	Chat      *ZaloChat `json:"chat,omitempty"`
	Text      string    `json:"text,omitempty"`
	Photo     string    `json:"photo,omitempty"`
	Document  string    `json:"document,omitempty"`
	Caption   string    `json:"caption,omitempty"`
	Sticker   string    `json:"sticker,omitempty"`
	URL       string    `json:"url,omitempty"`
	VoiceURL  string    `json:"voice_url,omitempty"`
}

type ZaloEventResult struct {
	EventID   int64               `json:"event_id,omitempty"`
	UpdateID  int64               `json:"update_id,omitempty"`
	EventName string              `json:"event_name"`
	Message   *ZaloMessagePayload `json:"message,omitempty"`
}

type ZaloWebhookEvent struct {
	OK     bool             `json:"ok"`
	Result *ZaloEventResult `json:"result,omitempty"`

	// Compatibility for legacy OpenAPI webhook events
	AppID     string `json:"app_id,omitempty"`
	EventName string `json:"event_name,omitempty"`
	Sender    struct {
		ID string `json:"id"`
	} `json:"sender,omitempty"`
	Recipient struct {
		ID string `json:"id"`
	} `json:"recipient,omitempty"`
	LegacyMessage struct {
		MsgID string `json:"msg_id"`
		Text  string `json:"text"`
	} `json:"message,omitempty"`
}

type ZaloUpdatesResponse struct {
	OK          bool            `json:"ok"`
	ErrorCode   int             `json:"error_code,omitempty"`
	Description string          `json:"description,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
}

type ZaloGetMeResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code,omitempty"`
	Description string `json:"description,omitempty"`
	Result      *struct {
		ID            string `json:"id"`
		AccountName   string `json:"account_name"`
		AccountType   string `json:"account_type"`
		CanJoinGroups bool   `json:"can_join_groups"`
	} `json:"result,omitempty"`
}

type ZaloSendMessagePayload struct {
	ChatID           string `json:"chat_id"`
	Text             string `json:"text"`
	ParseMode        string `json:"parse_mode,omitempty"`
	ReplyToMessageID string `json:"reply_to_message_id,omitempty"`
	TextStyles       []any  `json:"text_styles,omitempty"`
}

type ZaloSendPhotoPayload struct {
	ChatID           string `json:"chat_id"`
	Photo            string `json:"photo"`
	Caption          string `json:"caption,omitempty"`
	ParseMode        string `json:"parse_mode,omitempty"`
	ReplyToMessageID string `json:"reply_to_message_id,omitempty"`
}

type ZaloSendDocumentPayload struct {
	ChatID           string `json:"chat_id"`
	Document         string `json:"document"`
	Caption          string `json:"caption,omitempty"`
	FileName         string `json:"file_name,omitempty"`
	ParseMode        string `json:"parse_mode,omitempty"`
	ReplyToMessageID string `json:"reply_to_message_id,omitempty"`
}

type ZaloSendVoicePayload struct {
	ChatID           string `json:"chat_id"`
	Voice            string `json:"voice"`
	Caption          string `json:"caption,omitempty"`
	ReplyToMessageID string `json:"reply_to_message_id,omitempty"`
}

type ZaloSendChatActionPayload struct {
	ChatID string `json:"chat_id"`
	Action string `json:"action"` // "typing"
}

// --- Channel Implementation ---

func (z *ZaloChannel) SendMessage(ctx sdk.Context, msg sdk.OutboundMessage) error {
	acc, token, parseMode, err := resolveZaloAccount(ctx, msg.AccountID)
	if err != nil {
		ctx.Log().Error("Zalo SendMessage token error", "account_id", msg.AccountID, "err", err)
		return err
	}
	accountID := acc.AccountID

	chatID := sdk.FirstNonEmpty(msg.ChatID, msg.Recipient)
	if chatID == "" {
		return fmt.Errorf("recipient or chat_id is required")
	}

	if msg.WantsTyping() && acc.TypingEnabled() {
		_ = sendZaloChatAction(ctx, token, chatID, msg.ChatAction())
		if msg.IsTypingOnly() {
			return nil
		}
	} else if msg.IsTypingOnly() {
		return nil
	}

	if msg.Reaction != "" && acc.AckReactionEnabled() {
		setZaloReaction(ctx, token, chatID, msg.ReplyToID, sdk.MapReactionForPlatform("zalo", msg.Reaction))
		if msg.Kind == sdk.MessageKindReaction && msg.IsControlOnly() {
			return nil
		}
	} else if msg.Kind == sdk.MessageKindReaction && msg.IsControlOnly() {
		return nil
	}

	replyToID := ""
	if acc.ReplyQuoteEnabled() {
		replyToID = msg.ReplyToID
	}

	// 2. Handle image/photo attachments
	photoURL := msg.Metadata["photo"]
	if photoURL == "" {
		photoURL = msg.Metadata["image_url"]
	}
	if photoURL != "" {
		return sendZaloPhoto(ctx, token, chatID, photoURL, msg.Content, parseMode, replyToID)
	}

	// 3. Handle document/file attachments
	docURL := msg.Metadata["document"]
	if docURL == "" {
		docURL = msg.Metadata["file_url"]
	}
	if docURL != "" {
		fileName := msg.Metadata["file_name"]
		return sendZaloDocument(ctx, token, chatID, docURL, msg.Content, fileName, parseMode, replyToID)
	}

	// 4. Handle voice/audio attachments
	voiceURL := msg.Metadata["voice"]
	if voiceURL == "" {
		voiceURL = msg.Metadata["voice_url"]
	}
	if voiceURL == "" {
		voiceURL = msg.Metadata["audio_url"]
	}
	if voiceURL != "" {
		return sendZaloVoice(ctx, token, chatID, voiceURL, msg.Content, replyToID)
	}

	// 5. Handle standard text messages with Markdown formatting & auto-resilience
	chunks := sdk.SplitMessage(msg.Content, maxZaloTextLength)
	reqURL := fmt.Sprintf("%s/bot%s/sendMessage", defaultZaloBotAPIBase, token)
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	for i, chunk := range chunks {
		payload := ZaloSendMessagePayload{
			ChatID:    chatID,
			Text:      chunk,
			ParseMode: parseMode,
		}
		if i == 0 && replyToID != "" {
			payload.ReplyToMessageID = replyToID
		}

		ctx.Log().Info("Sending Zalo Bot message", "account_id", accountID, "chat_id", chatID, "chunk", i+1, "total", len(chunks))
		resp, err := ctx.HTTP().DoWithAuth("POST", reqURL, "", headers, payload)
		if err != nil {
			return fmt.Errorf("zalo sendMessage network error (chunk %d): %w", i+1, err)
		}

		// Graceful Markdown syntax error fallback (like Telegram & Discord)
		if resp.Status == 400 || (resp.Status == 200 && strings.Contains(strings.ToLower(resp.Body), "parse")) {
			var checkResp struct {
				OK          bool   `json:"ok"`
				ErrorCode   int    `json:"error_code"`
				Description string `json:"description"`
			}
			_ = resp.JSON(&checkResp)
			if !checkResp.OK || resp.Status == 400 {
				ctx.Log().Warn("Zalo markdown parse issue, retrying as clean plain text", "account_id", accountID, "status", resp.Status)
				payload.ParseMode = ""
				payload.Text = stripMarkdown(chunk)
				resp, err = ctx.HTTP().DoWithAuth("POST", reqURL, "", headers, payload)
				if err != nil {
					return fmt.Errorf("zalo sendMessage retry error (chunk %d): %w", i+1, err)
				}
			}
		}

		if resp.Status != 200 {
			return fmt.Errorf("zalo API returned status %d: %s", resp.Status, resp.Body)
		}

		var apiResp struct {
			OK          bool   `json:"ok"`
			ErrorCode   int    `json:"error_code,omitempty"`
			Description string `json:"description,omitempty"`
		}
		if err := resp.JSON(&apiResp); err == nil && !apiResp.OK {
			return fmt.Errorf("zalo API error (%d): %s", apiResp.ErrorCode, apiResp.Description)
		}
	}

	_ = ctx.EventBus().Emit("channel.zalo.sent", map[string]string{
		"account_id": accountID,
		"recipient":  chatID,
		"status":     "sent",
		"chunks":     strconv.Itoa(len(chunks)),
	})
	return nil
}

func (z *ZaloChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
	var cfg ZaloConfig
	_ = ctx.Config().Bind(&cfg)

	var inboundMsgs []sdk.InboundMessage

	// 1. Process pending webhook queue from storage (if any webhook events are stored)
	rawQueue, ok, _ := ctx.Storage().Get("pending_zalo_webhook")
	if ok && rawQueue != "" {
		_ = ctx.Storage().Delete("pending_zalo_webhook")

		var events []ZaloWebhookEvent
		if err := json.Unmarshal([]byte(rawQueue), &events); err != nil {
			var singleEvent ZaloWebhookEvent
			if err2 := json.Unmarshal([]byte(rawQueue), &singleEvent); err2 == nil {
				events = []ZaloWebhookEvent{singleEvent}
			} else {
				ctx.Log().Error("Failed parsing pending_zalo_webhook JSON", "err", err)
			}
		}

		webhookAcc := getActiveZaloAccounts(cfg)[0]
		for _, ev := range events {
			msg, ok := parseZaloEventToInbound(ev, webhookAcc)
			if ok {
				inboundMsgs = append(inboundMsgs, msg)
				ctx.Log().Info("Received Zalo message from Webhook queue", "sender_id", msg.SenderID, "sender_name", msg.SenderName, "agent", msg.TargetAgent)
				_ = ctx.EventBus().Emit("channel.zalo.received", msg)
			}
		}
	}

	// 2. Poll via getUpdates API for all active bot accounts
	accounts := getActiveZaloAccounts(cfg)
	for _, acc := range accounts {
		token, _, err := getZaloBotToken(ctx, acc)
		if err != nil || token == "" {
			ctx.Log().Warn("Zalo polling skipped: bot token missing", "account_id", acc.AccountID, "err", err)
			continue
		}

		// Ensure bot info is verified & logged once per token
		botInfoKey := "zalo_bot_info_" + acc.AccountID
		if info, ok, _ := ctx.Storage().Get(botInfoKey); !ok || info == "" {
			verifyZaloBotAccount(ctx, token, acc.AccountID, botInfoKey)
		}

		// Ensure any active Webhook on Zalo server is cleared once for this token so long-polling receives updates
		whCleanedKey := "zalo_wh_cleared_" + acc.AccountID
		lastTokenPrefix, _, _ := ctx.Storage().Get(whCleanedKey)
		tokenPrefix := token
		if len(tokenPrefix) > 12 {
			tokenPrefix = tokenPrefix[:12]
		}
		if lastTokenPrefix != tokenPrefix {
			delWHURL := fmt.Sprintf("%s/bot%s/deleteWebhook", defaultZaloBotAPIBase, token)
			_, _ = ctx.HTTP().DoWithAuth("POST", delWHURL, "", map[string]string{"Content-Type": "application/json"}, map[string]any{})
			_ = ctx.Storage().Set(whCleanedKey, tokenPrefix)
			ctx.Log().Info("Ensured Zalo long-polling mode (deleteWebhook executed)", "account_id", acc.AccountID)
		}

		offsetKey := "zalo_offset_" + acc.AccountID
		offsetStr, _, _ := ctx.Storage().Get(offsetKey)
		offset := int64(0)
		if offsetStr != "" {
			_, _ = fmt.Sscanf(offsetStr, "%d", &offset)
		}

		pollTimeout := cfg.PollIntervalSeconds
		if pollTimeout <= 0 {
			pollTimeout = 2
		}
		if pollTimeout > 30 {
			pollTimeout = 30
		}

		updatesURL := fmt.Sprintf("%s/bot%s/getUpdates", defaultZaloBotAPIBase, token)
		payload := map[string]any{
			"timeout": pollTimeout,
			"limit":   20,
		}
		if offset > 0 {
			payload["offset"] = offset
		}
		headers := map[string]string{
			"Content-Type": "application/json",
		}

		resp, err := ctx.HTTP().DoWithAuth("POST", updatesURL, "", headers, payload)
		if err != nil {
			ctx.Log().Error("Zalo getUpdates HTTP request failed", "account_id", acc.AccountID, "err", err)
			continue
		}
		if resp.Status != 200 {
			if resp.Status == 408 {
				continue // Standard HTTP 408 Request Timeout when no new messages arrive during long-poll window
			}
			if resp.Status == 409 || strings.Contains(strings.ToLower(resp.Body), "webhook is active") {
				ctx.Log().Info("Active webhook conflict detected (409), clearing webhook to restore long-polling...", "account_id", acc.AccountID)
				delWHURL := fmt.Sprintf("%s/bot%s/deleteWebhook", defaultZaloBotAPIBase, token)
				_, _ = ctx.HTTP().DoWithAuth("POST", delWHURL, "", map[string]string{"Content-Type": "application/json"}, map[string]any{})
				continue
			}
			ctx.Log().Warn("Zalo getUpdates non-200 status", "account_id", acc.AccountID, "status", resp.Status, "body", resp.Body)
			continue
		}

		var updatesResp ZaloUpdatesResponse
		if err := resp.JSON(&updatesResp); err != nil {
			ctx.Log().Warn("Failed to parse Zalo getUpdates response", "account_id", acc.AccountID, "body", resp.Body, "err", err)
			continue
		}

		if !updatesResp.OK {
			// 408 Request Timeout is standard Zalo long-polling idle response when no message arrives
			if updatesResp.ErrorCode == 408 || strings.Contains(strings.ToLower(updatesResp.Description), "timeout") {
				continue
			}
			if updatesResp.ErrorCode == 409 {
				ctx.Log().Info("Zalo getUpdates conflict (409): Auto-clearing webhook...", "account_id", acc.AccountID)
				delWHURL := fmt.Sprintf("%s/bot%s/deleteWebhook", defaultZaloBotAPIBase, token)
				_, _ = ctx.HTTP().DoWithAuth("POST", delWHURL, "", map[string]string{"Content-Type": "application/json"}, map[string]any{})
				continue
			}
			ctx.Log().Warn("Zalo getUpdates returned ok=false", "account_id", acc.AccountID, "error_code", updatesResp.ErrorCode, "description", updatesResp.Description)
			continue
		}

		// Handle both single object result (Zalo standard) and array result (Telegram compatible)
		var results []ZaloEventResult
		if len(updatesResp.Result) > 0 && string(updatesResp.Result) != "null" {
			var single ZaloEventResult
			if err := json.Unmarshal(updatesResp.Result, &single); err == nil && (single.EventName != "" || single.Message != nil || single.EventID > 0 || single.UpdateID > 0) {
				results = []ZaloEventResult{single}
			} else {
				_ = json.Unmarshal(updatesResp.Result, &results)
			}
		}

		maxID := offset
		for _, res := range results {
			currID := res.UpdateID
			if currID == 0 {
				currID = res.EventID
			}
			if currID >= maxID {
				maxID = currID + 1
			}

			ev := ZaloWebhookEvent{
				OK:     true,
				Result: &res,
			}
			msg, ok := parseZaloEventToInbound(ev, acc)
			if ok {
				inboundMsgs = append(inboundMsgs, msg)
				ctx.Log().Info("Received Zalo message via getUpdates", "sender_id", msg.SenderID, "sender_name", msg.SenderName, "agent", msg.TargetAgent, "text", msg.Content)
				_ = ctx.EventBus().Emit("channel.zalo.received", msg)

				chatID := sdk.FirstNonEmpty(msg.ChatID, msg.SenderID)
				if acc.TypingEnabled() {
					_ = sendZaloChatAction(ctx, token, chatID, "typing")
				}
				if acc.AckReactionEnabled() {
					setZaloReaction(ctx, token, chatID, msg.MessageID, sdk.MapReactionForPlatform("zalo", acc.ReactionEmoji()))
				}
			}
		}

		if maxID > offset {
			_ = ctx.Storage().Set(offsetKey, fmt.Sprintf("%d", maxID))
		}
	}

	return inboundMsgs, nil
}

// --- Helper Functions ---

func verifyZaloBotAccount(ctx sdk.Context, token, accountID, botInfoKey string) {
	reqURL := fmt.Sprintf("%s/bot%s/getMe", defaultZaloBotAPIBase, token)
	resp, err := ctx.HTTP().DoWithAuth("POST", reqURL, "", map[string]string{"Content-Type": "application/json"}, map[string]any{})
	if err != nil {
		ctx.Log().Warn("Zalo getMe check failed", "account_id", accountID, "err", err)
		return
	}
	var getMeResp ZaloGetMeResponse
	if err := resp.JSON(&getMeResp); err == nil && getMeResp.OK && getMeResp.Result != nil {
		infoStr := fmt.Sprintf("%s (%s)", getMeResp.Result.AccountName, getMeResp.Result.ID)
		_ = ctx.Storage().Set(botInfoKey, infoStr)
		ctx.Log().Info("Connected to Zalo Bot Platform", "account_id", accountID, "bot_name", getMeResp.Result.AccountName, "bot_id", getMeResp.Result.ID)
	}
}

func getZaloBotToken(ctx sdk.Context, acc ZaloAccount) (token string, parseMode string, err error) {
	var cfg ZaloConfig
	_ = ctx.Config().Bind(&cfg)

	parseMode = sdk.FirstNonEmpty(acc.ParseMode, cfg.ParseMode, "markdown")
	accountID := acc.AccountID
	inline := sdk.FirstNonEmpty(acc.BotToken, acc.ZaloBotToken)

	if inline != "" {
		return inline, parseMode, nil
	}

	// Account-specific Vault secrets — ONLY Zalo-specific keys (never generic "bot_token")
	keys := []string{}
	if accountID != "" && accountID != "default" {
		keys = append(keys, "zalo_bot_tokens."+accountID, "zalo_tokens."+accountID)
	}
	keys = append(keys, "zalo_bot_token")
	if secret := sdk.ResolveSecret(ctx, "", keys...); secret != "" {
		return secret, parseMode, nil
	}

	if cfg.ZaloBotToken != "" {
		return cfg.ZaloBotToken, parseMode, nil
	}

	if val, ok, _ := ctx.Storage().Get("zalo_bot_token"); ok && val != "" {
		return val, parseMode, nil
	}

	return "", "", fmt.Errorf("no Zalo Bot Token found for account '%s': please set 'bot_token' / 'zalo_bot_token' in plugin config or vault", accountID)
}

func resolveZaloAccount(ctx sdk.Context, accountID string) (ZaloAccount, string, string, error) {
	var cfg ZaloConfig
	_ = ctx.Config().Bind(&cfg)
	accounts := getActiveZaloAccounts(cfg)

	acc := ZaloAccount{ChannelAccount: sdk.ChannelAccount{AccountID: accountID}}
	for _, a := range accounts {
		if a.AccountID == accountID || (accountID == "default" && (a.AccountID == "" || a.AccountID == "default")) {
			acc = a
			break
		}
	}
	if acc.AccountID == "" {
		acc.AccountID = "default"
	}
	token, parseMode, err := getZaloBotToken(ctx, acc)
	return acc, token, parseMode, err
}

func getActiveZaloAccounts(cfg ZaloConfig) []ZaloAccount {
	if len(cfg.Accounts) > 0 {
		return cfg.Accounts
	}
	return []ZaloAccount{
		{
			ChannelAccount: sdk.ChannelAccount{
				AccountID:    "default",
				DisplayName:  "Default Zalo Bot",
				BotToken:     cfg.ZaloBotToken,
				DefaultAgent: cfg.DefaultAgent,
				ChannelAccountFeatures: sdk.ChannelAccountFeatures{
					EnableTypingIndicator: cfg.EnableTypingIndicator,
					EnableAckReaction:     cfg.EnableAckReaction,
					EnableReplyQuote:      cfg.EnableReplyQuote,
				},
			},
			ZaloBotToken: cfg.ZaloBotToken,
			ParseMode:    cfg.ParseMode,
		},
	}
}

func parseZaloEventToInbound(ev ZaloWebhookEvent, acc ZaloAccount) (sdk.InboundMessage, bool) {
	accountID := acc.AccountID
	if accountID == "" {
		accountID = "default"
	}

	// A. Handle new Zalo Bot Platform event format
	if ev.Result != nil && ev.Result.Message != nil {
		m := ev.Result.Message
		senderID := ""
		senderName := "Zalo User"
		if m.From != nil {
			senderID = m.From.ID
			if m.From.DisplayName != "" {
				senderName = m.From.DisplayName
			}
		}
		if senderID == "" && m.Chat != nil {
			senderID = m.Chat.ID
		}

		rawText := m.Text
		if rawText == "" && m.Caption != "" {
			rawText = m.Caption
		}

		targetAgent, cleanText := sdk.ExtractAgentMention(rawText)
		if targetAgent == "" && acc.DefaultAgent != "" {
			targetAgent = acc.DefaultAgent
		}

		inbound := sdk.NewInboundMessage(
			"zalo",
			accountID,
			senderID,
			senderName,
			cleanText,
		)
		inbound.TargetAgent = targetAgent
		inbound.Metadata["reply_to_msg_id"] = m.MessageID
		if ev.Result.EventName != "" {
			inbound.Metadata["event_name"] = ev.Result.EventName
		}
		chatID := senderID
		if m.Chat != nil {
			chatID = m.Chat.ID
			inbound.Metadata["chat_type"] = m.Chat.ChatType
		}
		if m.Photo != "" {
			inbound.Metadata["photo"] = m.Photo
		}
		if m.Document != "" {
			inbound.Metadata["document"] = m.Document
		}
		if m.VoiceURL != "" {
			inbound.Metadata["voice_url"] = m.VoiceURL
		}
		ts := ""
		if m.Date > 0 {
			ts = fmt.Sprintf("%d", m.Date)
			inbound.Metadata["date"] = ts
		}
		sdk.ApplyInboundEnvelope(&inbound, chatID, m.MessageID, "", ts)
		return inbound, true
	}

	// B. Handle legacy Zalo OA OpenAPI webhook event format
	if ev.EventName == "user_send_text" && ev.LegacyMessage.Text != "" {
		targetAgent, cleanText := sdk.ExtractAgentMention(ev.LegacyMessage.Text)
		if targetAgent == "" && acc.DefaultAgent != "" {
			targetAgent = acc.DefaultAgent
		}

		inbound := sdk.NewInboundMessage(
			"zalo",
			accountID,
			ev.Sender.ID,
			"ZaloUser_"+ev.Sender.ID,
			cleanText,
		)
		inbound.TargetAgent = targetAgent
		inbound.Metadata["oa_id"] = ev.Recipient.ID
		sdk.ApplyInboundEnvelope(&inbound, ev.Sender.ID, ev.LegacyMessage.MsgID, "", "")
		return inbound, true
	}

	return sdk.InboundMessage{}, false
}

func setZaloReaction(ctx sdk.Context, token, chatID, messageID, emoji string) {
	if token == "" || chatID == "" || messageID == "" || emoji == "" {
		return
	}
	reqURL := fmt.Sprintf("%s/bot%s/setMessageReaction", defaultZaloBotAPIBase, token)
	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"reaction": []map[string]string{
			{"type": "emoji", "emoji": emoji},
		},
	}
	_, _ = ctx.HTTP().DoWithAuth("POST", reqURL, "", map[string]string{"Content-Type": "application/json"}, payload)
}

func sendZaloChatAction(ctx sdk.Context, token, chatID, action string) error {
	if token == "" || chatID == "" {
		return nil
	}
	if action == "" {
		action = "typing"
	}
	reqURL := fmt.Sprintf("%s/bot%s/sendChatAction", defaultZaloBotAPIBase, token)
	payload := ZaloSendChatActionPayload{
		ChatID: chatID,
		Action: strings.ToLower(action),
	}
	headers := map[string]string{"Content-Type": "application/json"}
	_, err := ctx.HTTP().DoWithAuth("POST", reqURL, "", headers, payload)
	return err
}

func sendZaloPhoto(ctx sdk.Context, token, chatID, photoURL, caption, parseMode, replyToID string) error {
	reqURL := fmt.Sprintf("%s/bot%s/sendPhoto", defaultZaloBotAPIBase, token)
	payload := ZaloSendPhotoPayload{
		ChatID:           chatID,
		Photo:            photoURL,
		Caption:          caption,
		ParseMode:        parseMode,
		ReplyToMessageID: replyToID,
	}
	headers := map[string]string{"Content-Type": "application/json"}
	resp, err := ctx.HTTP().DoWithAuth("POST", reqURL, "", headers, payload)
	if err != nil {
		return fmt.Errorf("zalo sendPhoto network error: %w", err)
	}

	// Retry without markdown if syntax parse error
	if resp.Status == 400 && payload.ParseMode != "" {
		payload.ParseMode = ""
		payload.Caption = stripMarkdown(caption)
		resp, err = ctx.HTTP().DoWithAuth("POST", reqURL, "", headers, payload)
		if err != nil {
			return fmt.Errorf("zalo sendPhoto retry error: %w", err)
		}
	}

	if resp.Status != 200 {
		return fmt.Errorf("zalo sendPhoto returned status %d: %s", resp.Status, resp.Body)
	}
	return nil
}

func sendZaloDocument(ctx sdk.Context, token, chatID, documentURL, caption, fileName, parseMode, replyToID string) error {
	reqURL := fmt.Sprintf("%s/bot%s/sendDocument", defaultZaloBotAPIBase, token)
	payload := ZaloSendDocumentPayload{
		ChatID:           chatID,
		Document:         documentURL,
		Caption:          caption,
		FileName:         fileName,
		ParseMode:        parseMode,
		ReplyToMessageID: replyToID,
	}
	headers := map[string]string{"Content-Type": "application/json"}
	resp, err := ctx.HTTP().DoWithAuth("POST", reqURL, "", headers, payload)
	if err != nil {
		return fmt.Errorf("zalo sendDocument network error: %w", err)
	}

	// Retry without markdown if syntax parse error
	if resp.Status == 400 && payload.ParseMode != "" {
		payload.ParseMode = ""
		payload.Caption = stripMarkdown(caption)
		resp, err = ctx.HTTP().DoWithAuth("POST", reqURL, "", headers, payload)
		if err != nil {
			return fmt.Errorf("zalo sendDocument retry error: %w", err)
		}
	}

	if resp.Status != 200 {
		return fmt.Errorf("zalo sendDocument returned status %d: %s", resp.Status, resp.Body)
	}
	return nil
}

func sendZaloVoice(ctx sdk.Context, token, chatID, voiceURL, caption, replyToID string) error {
	reqURL := fmt.Sprintf("%s/bot%s/sendVoice", defaultZaloBotAPIBase, token)
	payload := ZaloSendVoicePayload{
		ChatID:           chatID,
		Voice:            voiceURL,
		Caption:          caption,
		ReplyToMessageID: replyToID,
	}
	headers := map[string]string{"Content-Type": "application/json"}
	resp, err := ctx.HTTP().DoWithAuth("POST", reqURL, "", headers, payload)
	if err != nil {
		return fmt.Errorf("zalo sendVoice network error: %w", err)
	}
	if resp.Status != 200 {
		return fmt.Errorf("zalo sendVoice returned status %d: %s", resp.Status, resp.Body)
	}
	return nil
}

func stripMarkdown(input string) string {
	r := strings.NewReplacer(
		"**", "",
		"__", "",
		"*", "",
		"_", "",
		"`", "",
		"~~", "",
		"# ", "",
		"## ", "",
		"### ", "",
	)
	return r.Replace(input)
}

func init() {
	ch := &ZaloChannel{
		BaseChannel: sdk.BaseChannel{
			ChannelName:        "zalo",
			ChannelDisplayName: "Zalo Bot Platform",
			PairingRequired:    true,
		},
	}
	sdk.RegisterChannel(ch)
}

func main() {
	sdk.Serve()
}
