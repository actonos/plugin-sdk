package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/actonos/plugin-sdk/sdk"
)

const (
	defaultZaloBotAPIBase = "https://bot-api.zaloplatforms.com"
	maxZaloTextLength     = 2000
)

// ZaloConfig defines the root configuration model for the Zalo Bot Platform channel plugin.
type ZaloConfig struct {
	BotToken     string        `json:"bot_token"`
	ParseMode    string        `json:"parse_mode"` // "markdown" or "html"
	DefaultAgent string        `json:"default_agent"`
	Accounts     []ZaloAccount `json:"accounts"`
}

// ZaloAccount represents an individual configured Zalo Bot account instance.
type ZaloAccount struct {
	AccountID    string `json:"account_id"`
	DisplayName  string `json:"display_name"`
	BotToken     string `json:"bot_token,omitempty"`
	DefaultAgent string `json:"default_agent"`
	ParseMode    string `json:"parse_mode"`
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
	Caption   string    `json:"caption,omitempty"`
	Sticker   string    `json:"sticker,omitempty"`
	URL       string    `json:"url,omitempty"`
	VoiceURL  string    `json:"voice_url,omitempty"`
}

type ZaloEventResult struct {
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
	OK          bool              `json:"ok"`
	ErrorCode   int               `json:"error_code,omitempty"`
	Description string            `json:"description,omitempty"`
	Result      []ZaloEventResult `json:"result,omitempty"`
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
	ChatID     string `json:"chat_id"`
	Text       string `json:"text"`
	ParseMode  string `json:"parse_mode,omitempty"`
	TextStyles []any  `json:"text_styles,omitempty"`
}

type ZaloSendPhotoPayload struct {
	ChatID    string `json:"chat_id"`
	Photo     string `json:"photo"`
	Caption   string `json:"caption,omitempty"`
	ParseMode string `json:"parse_mode,omitempty"`
}

type ZaloSendChatActionPayload struct {
	ChatID string `json:"chat_id"`
	Action string `json:"action"` // "typing"
}

// --- Channel Implementation ---

func (z *ZaloChannel) SendMessage(ctx sdk.Context, msg sdk.OutboundMessage) error {
	accountID := msg.AccountID
	if accountID == "" {
		accountID = msg.Metadata["account_id"]
	}
	if accountID == "" {
		accountID = "default"
	}

	token, parseMode, err := getZaloBotToken(ctx, accountID)
	if err != nil {
		ctx.Log().Error("Zalo SendMessage token error", "account_id", accountID, "err", err)
		return err
	}

	chatID := msg.Metadata["chat_id"]
	if chatID == "" {
		chatID = msg.Recipient
	}
	if chatID == "" {
		return fmt.Errorf("recipient or chat_id is required")
	}

	// 1. Handle typing indicator or chat actions
	if msg.Metadata["typing"] == "true" || msg.Metadata["action"] != "" || (msg.Content == "" && msg.Metadata["photo"] == "" && msg.Metadata["image_url"] == "") {
		action := msg.Metadata["action"]
		if action == "" {
			action = "typing"
		}
		return sendZaloChatAction(ctx, token, chatID, action)
	}

	// 2. Handle image/photo attachments
	photoURL := msg.Metadata["photo"]
	if photoURL == "" {
		photoURL = msg.Metadata["image_url"]
	}
	if photoURL != "" {
		return sendZaloPhoto(ctx, token, chatID, photoURL, msg.Content, parseMode)
	}

	// 3. Handle standard text messages with Markdown formatting
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

		ctx.Log().Info("Sending Zalo Bot message", "account_id", accountID, "chat_id", chatID, "chunk", i+1, "total", len(chunks))
		resp, err := ctx.HTTP().DoWithAuth("POST", reqURL, "", headers, payload)
		if err != nil {
			return fmt.Errorf("zalo sendMessage network error (chunk %d): %w", i+1, err)
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
			// Try parsing as single event wrapper
			var singleEvent ZaloWebhookEvent
			if err2 := json.Unmarshal([]byte(rawQueue), &singleEvent); err2 == nil {
				events = []ZaloWebhookEvent{singleEvent}
			} else {
				ctx.Log().Error("Failed parsing pending_zalo_webhook JSON", "err", err)
			}
		}

		for _, ev := range events {
			msg, ok := parseZaloEventToInbound(ev, cfg, "default")
			if ok {
				inboundMsgs = append(inboundMsgs, msg)
				_ = ctx.EventBus().Emit("channel.zalo.received", msg)
			}
		}
	}

	// 2. Poll via getUpdates API for all active bot accounts
	accounts := getActiveZaloAccounts(cfg)
	for _, acc := range accounts {
		token, _, err := getZaloBotToken(ctx, acc.AccountID)
		if err != nil || token == "" {
			continue
		}

		updatesURL := fmt.Sprintf("%s/bot%s/getUpdates", defaultZaloBotAPIBase, token)
		payload := map[string]any{
			"timeout": "1",
		}
		headers := map[string]string{
			"Content-Type": "application/json",
		}

		resp, err := ctx.HTTP().DoWithAuth("POST", updatesURL, "", headers, payload)
		if err != nil || resp.Status != 200 {
			continue
		}

		var updatesResp ZaloUpdatesResponse
		if err := resp.JSON(&updatesResp); err != nil || !updatesResp.OK {
			continue
		}

		for _, res := range updatesResp.Result {
			ev := ZaloWebhookEvent{
				OK:     true,
				Result: &res,
			}
			msg, ok := parseZaloEventToInbound(ev, cfg, acc.AccountID)
			if ok {
				inboundMsgs = append(inboundMsgs, msg)
				_ = ctx.EventBus().Emit("channel.zalo.received", msg)
			}
		}
	}

	return inboundMsgs, nil
}

// --- Helper Functions ---

func getZaloBotToken(ctx sdk.Context, accountID string) (token string, parseMode string, err error) {
	var cfg ZaloConfig
	_ = ctx.Config().Bind(&cfg)

	parseMode = cfg.ParseMode
	if parseMode == "" {
		parseMode = "markdown"
	}

	// 1. Check account-specific Vault secret
	if accountID != "" && accountID != "default" {
		if secret, err := ctx.Vault().GetSecret("zalo_tokens." + accountID); err == nil && secret != "" {
			return secret, parseMode, nil
		}
	}

	// 2. Check accounts list from config
	for _, acc := range cfg.Accounts {
		if acc.AccountID == accountID {
			if acc.BotToken != "" {
				if acc.ParseMode != "" {
					parseMode = acc.ParseMode
				}
				return acc.BotToken, parseMode, nil
			}
		}
	}

	// 3. Fallback to default Vault secret
	if secret, err := ctx.Vault().GetSecret("zalo_bot_token"); err == nil && secret != "" {
		return secret, parseMode, nil
	}

	// 4. Fallback to root config bot_token
	if cfg.BotToken != "" {
		return cfg.BotToken, parseMode, nil
	}

	return "", "", fmt.Errorf("no Zalo Bot Token found for account '%s' (check vault 'zalo_bot_token' or config)", accountID)
}

func getActiveZaloAccounts(cfg ZaloConfig) []ZaloAccount {
	if len(cfg.Accounts) > 0 {
		return cfg.Accounts
	}
	return []ZaloAccount{
		{
			AccountID:    "default",
			DisplayName:  "Default Zalo Bot",
			BotToken:     cfg.BotToken,
			DefaultAgent: cfg.DefaultAgent,
			ParseMode:    cfg.ParseMode,
		},
	}
}

func parseZaloEventToInbound(ev ZaloWebhookEvent, cfg ZaloConfig, accountID string) (sdk.InboundMessage, bool) {
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
		if targetAgent == "" && cfg.DefaultAgent != "" {
			targetAgent = cfg.DefaultAgent
		}

		inbound := sdk.NewInboundMessage(
			"zalo",
			accountID,
			senderID,
			senderName,
			cleanText,
		)
		inbound.TargetAgent = targetAgent
		inbound.Metadata["msg_id"] = m.MessageID
		inbound.Metadata["event_name"] = ev.Result.EventName
		if m.Chat != nil {
			inbound.Metadata["chat_id"] = m.Chat.ID
			inbound.Metadata["chat_type"] = m.Chat.ChatType
		}
		if m.Photo != "" {
			inbound.Metadata["photo"] = m.Photo
		}
		if m.VoiceURL != "" {
			inbound.Metadata["voice_url"] = m.VoiceURL
		}
		if m.Date > 0 {
			inbound.Metadata["date"] = fmt.Sprintf("%d", m.Date)
		}

		return inbound, true
	}

	// B. Handle legacy Zalo OA OpenAPI webhook event format
	if ev.EventName == "user_send_text" && ev.LegacyMessage.Text != "" {
		targetAgent, cleanText := sdk.ExtractAgentMention(ev.LegacyMessage.Text)
		if targetAgent == "" && cfg.DefaultAgent != "" {
			targetAgent = cfg.DefaultAgent
		}

		inbound := sdk.NewInboundMessage(
			"zalo",
			accountID,
			ev.Sender.ID,
			"ZaloUser_"+ev.Sender.ID,
			cleanText,
		)
		inbound.TargetAgent = targetAgent
		inbound.Metadata["msg_id"] = ev.LegacyMessage.MsgID
		inbound.Metadata["oa_id"] = ev.Recipient.ID

		return inbound, true
	}

	return sdk.InboundMessage{}, false
}

func sendZaloChatAction(ctx sdk.Context, token, chatID, action string) error {
	reqURL := fmt.Sprintf("%s/bot%s/sendChatAction", defaultZaloBotAPIBase, token)
	payload := ZaloSendChatActionPayload{
		ChatID: chatID,
		Action: strings.ToLower(action),
	}
	headers := map[string]string{"Content-Type": "application/json"}
	_, err := ctx.HTTP().DoWithAuth("POST", reqURL, "", headers, payload)
	return err
}

func sendZaloPhoto(ctx sdk.Context, token, chatID, photoURL, caption, parseMode string) error {
	reqURL := fmt.Sprintf("%s/bot%s/sendPhoto", defaultZaloBotAPIBase, token)
	payload := ZaloSendPhotoPayload{
		ChatID:    chatID,
		Photo:     photoURL,
		Caption:   caption,
		ParseMode: parseMode,
	}
	headers := map[string]string{"Content-Type": "application/json"}
	resp, err := ctx.HTTP().DoWithAuth("POST", reqURL, "", headers, payload)
	if err != nil {
		return fmt.Errorf("zalo sendPhoto error: %w", err)
	}
	if resp.Status != 200 {
		return fmt.Errorf("zalo sendPhoto returned status %d: %s", resp.Status, resp.Body)
	}
	return nil
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
